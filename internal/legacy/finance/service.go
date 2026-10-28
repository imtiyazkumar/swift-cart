package finance

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"time"
)

type Service struct {
	DB      *pgxpool.Pool
	Gateway *Razorpay
}

func New(db *pgxpool.Pool) *Service { return &Service{DB: db, Gateway: &Razorpay{}} }
func audit(ctx context.Context, tx pgx.Tx, actor, action, target, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO public.admin_audit_logs(admin_id,action,target_type,target_id,details) VALUES($1,$2,'finance',$3,jsonb_build_object('reason',$4::text))`, actor, action, target, reason)
	return err
}
func notify(ctx context.Context, tx pgx.Tx, user, subject, body string) error {
	_, err := tx.Exec(ctx, `INSERT INTO notifications.email_outbox(recipient,subject,body) SELECT email,$2,$3 FROM public.users WHERE id=$1 AND email IS NOT NULL`, user, subject, body)
	return err
}
func (s *Service) SaveBank(ctx context.Context, user string, b BankInput) error {
	if err := b.Validate(); err != nil {
		return err
	}
	id := uuid.NewString()
	raw, key, err := encryptBank(user, id, b)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM public.users WHERE id=$1 FOR UPDATE`, user); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE finance.bank_accounts SET status='REPLACED' WHERE user_id=$1 AND status IN ('PENDING','VERIFIED')`, user); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO finance.bank_accounts(id,user_id,ciphertext,key_id,last_four) VALUES($1,$2,$3,$4,$5)`, id, user, raw, key, b.Number[len(b.Number)-4:]); err != nil {
		return err
	}
	if err = notify(ctx, tx, user, "Yember bank account changed", "Your payout account was changed and requires verification. Contact support immediately if you did not make this change."); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) VerifyBank(ctx context.Context, actor, id, reference string) error {
	if len(reference) < 8 || len(reference) > 500 {
		return errors.New("bank verification evidence reference required")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE finance.bank_accounts SET status='VERIFIED',verified_by=$2,verification_reference=$3 WHERE id=$1 AND status='PENDING' AND user_id<>$2`, id, actor, reference)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("pending account requires an independent verifier")
	}
	if err = audit(ctx, tx, actor, "BANK_VERIFIED", id, reference); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) Commission(ctx context.Context, actor, merchant, reason string, bps int) error {
	if bps < 0 || bps > 10000 || len(reason) < 5 || len(reason) > 2000 {
		return errors.New("commission 0–10000 basis points and reason required")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE public.restaurants SET commission_bps=$2 WHERE id=$1 AND deleted_at IS NULL`, merchant, bps)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("restaurant not found")
	}
	if err = audit(ctx, tx, actor, "COMMISSION_CHANGED", merchant, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) Generate(ctx context.Context, actor string, now time.Time) (string, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(894213)`); err != nil {
		return "", err
	}
	var days int
	var anchor time.Time
	if err = tx.QueryRow(ctx, `SELECT interval_days,anchor FROM finance.policy WHERE id=true`).Scan(&days, &anchor); err != nil {
		return "", err
	}
	cutoff := Cutoff(now, anchor, days)
	var batch string
	err = tx.QueryRow(ctx, `INSERT INTO finance.batches(cutoff,prepared_by) VALUES($1,NULLIF($2,'')::uuid) ON CONFLICT(cutoff) DO NOTHING RETURNING id`, cutoff, actor).Scan(&batch)
	if err == pgx.ErrNoRows {
		err = tx.QueryRow(ctx, `SELECT id FROM finance.batches WHERE cutoff=$1`, cutoff).Scan(&batch)
		return batch, err
	}
	if err != nil {
		return "", err
	}
	rows, err := tx.Query(ctx, `SELECT e.user_id,-sum(e.amount),b.id FROM finance.entries e JOIN finance.bank_accounts b ON b.user_id=e.user_id AND b.status='VERIFIED' JOIN public.users u ON u.id=e.user_id AND u.disabled_at IS NULL AND u.deleted_at IS NULL WHERE e.account='PARTNER_PAYABLE' AND NOT EXISTS(SELECT 1 FROM finance.entries ce JOIN finance.journals cj ON cj.id=ce.journal_id JOIN finance.cod_custody cc ON cc.order_id=cj.order_id WHERE ce.user_id=e.user_id AND NOT EXISTS(SELECT 1 FROM finance.cash_handovers ch WHERE ch.order_id=cc.order_id AND ch.status='PAID')) AND e.created_at<$1 AND NOT EXISTS(SELECT 1 FROM finance.settlement_entries se WHERE se.entry_id=e.id AND NOT EXISTS(SELECT 1 FROM finance.settlement_releases sr WHERE sr.entry_id=se.entry_id AND sr.payout_id=se.payout_id)) AND NOT EXISTS(SELECT 1 FROM finance.discrepancies d WHERE d.resolved_at IS NULL AND (d.order_id IS NULL OR EXISTS(SELECT 1 FROM finance.entries de JOIN finance.journals dj ON dj.id=de.journal_id WHERE dj.order_id=d.order_id AND de.user_id=e.user_id))) GROUP BY e.user_id,b.id HAVING sum(e.amount)<0`, cutoff)
	if err != nil {
		return "", err
	}
	type candidate struct {
		user, bank string
		amount     int64
	}
	items := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.user, &c.amount, &c.bank); err != nil {
			rows.Close()
			return "", err
		}
		items = append(items, c)
	}
	rows.Close()
	if rows.Err() != nil {
		return "", rows.Err()
	}
	for _, c := range items {
		var payout string
		if err = tx.QueryRow(ctx, `INSERT INTO finance.payouts(batch_id,user_id,bank_account_id,amount) VALUES($1,$2,$3,$4) RETURNING id`, batch, c.user, c.bank, c.amount).Scan(&payout); err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO finance.settlement_entries(entry_id,payout_id) SELECT e.id,$1 FROM finance.entries e WHERE e.user_id=$2 AND e.account='PARTNER_PAYABLE' AND e.created_at<$3 AND NOT EXISTS(SELECT 1 FROM finance.settlement_entries se WHERE se.entry_id=e.id AND NOT EXISTS(SELECT 1 FROM finance.settlement_releases sr WHERE sr.entry_id=se.entry_id AND sr.payout_id=se.payout_id))`, payout, c.user, cutoff); err != nil {
			return "", err
		}
		if err = notify(ctx, tx, c.user, "Yember settlement prepared", "Your eligible earnings are included in a settlement batch awaiting finance approval. Funds have not yet been paid."); err != nil {
			return "", err
		}
	}
	if actor != "" {
		if err = audit(ctx, tx, actor, "SETTLEMENT_PREPARED", batch, "Scheduled eligible earnings reserved"); err != nil {
			return "", err
		}
	}
	return batch, tx.Commit(ctx)
}
func (s *Service) Approve(ctx context.Context, actor, id, reason string) error {
	if len(reason) < 5 {
		return errors.New("approval reason required")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE finance.batches SET status='APPROVED',approved_by=$2,approved_at=now() WHERE id=$1 AND status='REVIEW' AND prepared_by IS DISTINCT FROM $2::uuid`, id, actor)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("batch requires independent approval")
	}
	if err = audit(ctx, tx, actor, "SETTLEMENT_APPROVED", id, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Initiate reserves the payout for a banking operator. It does not assert money moved.
// The stable payout UUID is the idempotency reference for the external bank instruction.
func (s *Service) Initiate(ctx context.Context, actor, id, reason string) error {
	if len(reason) < 5 {
		return errors.New("payout instruction reason required")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	provider := "BANK_RECONCILIATION"
	if os.Getenv("FINANCE_PAYOUT_PROVIDER") == "RAZORPAYX" {
		provider = "RAZORPAYX"
	}
	tag, err := tx.Exec(ctx, `UPDATE finance.payouts p SET status='PROCESSING',provider=$3,initiated_by=$2,updated_at=now(),attempt_no=attempt_no+1,provider_id=NULL FROM finance.batches b,finance.bank_accounts a,public.users u WHERE p.id=$1 AND b.id=p.batch_id AND b.status='APPROVED' AND b.approved_by<>$2 AND a.id=p.bank_account_id AND a.status='VERIFIED' AND u.id=p.user_id AND u.disabled_at IS NULL AND u.deleted_at IS NULL AND p.status IN ('PENDING','FAILED') AND NOT EXISTS(SELECT 1 FROM finance.discrepancies d WHERE d.resolved_at IS NULL AND (d.order_id IS NULL OR EXISTS(SELECT 1 FROM finance.entries de JOIN finance.journals dj ON dj.id=de.journal_id WHERE dj.order_id=d.order_id AND de.user_id=p.user_id)))`, id, actor, provider)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("approved payout requires independent operator and verified current bank account")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO finance.payout_history(payout_id,status,reference,actor) VALUES($1,'PROCESSING',$2,$3)`, id, reason, actor); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO finance.payout_attempts(payout_id,attempt_no,idempotency_key) SELECT id,attempt_no,gen_random_uuid() FROM finance.payouts WHERE id=$1`, id); err != nil {
		return err
	}
	if err = audit(ctx, tx, actor, "PAYOUT_INITIATED", id, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Confirm requires independent bank-statement reconciliation; browser success is never payment proof.
func (s *Service) Confirm(ctx context.Context, actor, id, status, reference string) error {
	return s.confirm(ctx, actor, id, status, reference, false)
}
func (s *Service) confirm(ctx context.Context, actor, id, status, reference string, providerVerified bool) error {
	if len(reference) < 8 || len(reference) > 500 {
		return errors.New("unique bank statement/UTR evidence reference required")
	}
	if status != "SUCCEEDED" && status != "FAILED" && status != "UNKNOWN" {
		return errors.New("unsupported reconciliation status")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(894213)`); err != nil {
		return err
	}
	var user, current, initiator, provider string
	var amount int64
	if err = tx.QueryRow(ctx, `SELECT user_id,status,COALESCE(initiated_by::text,''),amount,provider FROM finance.payouts WHERE id=$1 FOR UPDATE`, id).Scan(&user, &current, &initiator, &amount, &provider); err != nil {
		return err
	}
	if provider == "RAZORPAYX" && !providerVerified {
		return errors.New("RazorpayX outcomes require authoritative provider reconciliation")
	}
	if current == status {
		return nil
	}
	if current != "PROCESSING" && current != "UNKNOWN" {
		return errors.New("only processing or unknown payouts can be reconciled")
	}
	if initiator == actor {
		return errors.New("independent reconciliation required")
	}
	if status == "SUCCEEDED" {
		if _, err = tx.Exec(ctx, `INSERT INTO finance.reconciliations(actor,reference,details) VALUES($1,$2,jsonb_build_object('payout_id',$3::text,'amount',$4::bigint))`, actor, reference, id, amount); err != nil {
			return err
		}
		// Debit liability; credit central cash. Entries are consumed here, not at preparation.
		if err = post(ctx, tx, "payout/"+id, "PAYOUT", "", reference, "Bank statement confirmed payout", actor, Line{"PARTNER_PAYABLE", user, "payout", amount}, Line{"CASH", "", "payout", -amount}); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO finance.settlement_entries(entry_id,payout_id) SELECT e.id,$1 FROM finance.entries e JOIN finance.journals j ON j.id=e.journal_id WHERE j.event_key=$2 AND e.account='PARTNER_PAYABLE'`, id, "payout/"+id); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE finance.payouts SET status=$2,confirmed_by=$3,evidence=$4,updated_at=now() WHERE id=$1`, id, status, actor, reference); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO finance.payout_history(payout_id,status,reference,actor) VALUES($1,$2,$3,$4)`, id, status, reference, actor); err != nil {
		return err
	}
	if err = audit(ctx, tx, actor, "PAYOUT_RECONCILED", id, reference); err != nil {
		return err
	}
	if err = notify(ctx, tx, user, "Yember settlement update", "Your settlement status is "+status+". Reference: "+reference); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) Summary(ctx context.Context, user string) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.DB.QueryRow(ctx, `SELECT jsonb_build_object('currency','INR','gross',COALESCE(-sum(e.amount) FILTER(WHERE e.component='restaurant_gross'),0),'delivery_earnings',COALESCE(-sum(e.amount) FILTER(WHERE e.component='delivery'),0),'commission',COALESCE(sum(e.amount) FILTER(WHERE e.component='commission'),0),'adjustments',COALESCE(-sum(e.amount) FILTER(WHERE e.component IN ('bonus','incentive','adjustment','refund')),0),'paid',COALESCE(sum(e.amount) FILTER(WHERE e.component IN ('payout','payout_reversal')),0),'pending',COALESCE(-sum(e.amount),0),'next_settlement_at',(SELECT anchor+((floor(extract(epoch FROM (now()-anchor))/(interval_days*86400))+1)*interval_days)*interval '1 day' FROM finance.policy)) FROM finance.entries e WHERE e.user_id=$1 AND account='PARTNER_PAYABLE'`, user).Scan(&raw)
	return raw, err
}

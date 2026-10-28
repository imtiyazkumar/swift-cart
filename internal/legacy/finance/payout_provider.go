package finance

import (
	"context"
	"errors"
	"net/url"
	"os"
	"time"
)

// SubmitRazorpayX uses a different approved banking product, not a collection split.
// A transport failure stays UNKNOWN and can only be recovered under the same payout id.
func (s *Service) SubmitRazorpayX(ctx context.Context, id string) error {
	lock, err := s.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	var locked bool
	if err = lock.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,894216))`, id).Scan(&locked); err != nil {
		lock.Release()
		return err
	}
	if !locked {
		lock.Release()
		return errors.New("payout instruction is already being processed")
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, unlockErr := lock.Exec(unlockCtx, `SELECT pg_advisory_unlock(hashtextextended($1,894216))`, id); unlockErr != nil {
			_ = lock.Conn().Close(unlockCtx)
		}
		lock.Release()
	}()
	if os.Getenv("FINANCE_PAYOUT_PROVIDER") != "RAZORPAYX" {
		return nil
	}
	if os.Getenv("FINANCE_LIVE_APPROVED") != "true" || os.Getenv("RAZORPAYX_ACCOUNT_NUMBER") == "" {
		return errors.New("RazorpayX approval and source account required")
	}
	var user, bank, key string
	var raw []byte
	var amount int64
	var fund *string
	err = s.DB.QueryRow(ctx, `SELECT p.user_id,p.bank_account_id,b.key_id,b.ciphertext,p.amount,b.provider_fund_account FROM finance.payouts p JOIN finance.bank_accounts b ON b.id=p.bank_account_id JOIN public.users u ON u.id=p.user_id WHERE p.id=$1 AND p.provider='RAZORPAYX' AND p.status IN ('PROCESSING','UNKNOWN') AND b.status='VERIFIED' AND u.disabled_at IS NULL AND u.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM finance.discrepancies d WHERE d.resolved_at IS NULL AND (d.order_id IS NULL OR EXISTS(SELECT 1 FROM finance.entries de JOIN finance.journals dj ON dj.id=de.journal_id WHERE dj.order_id=d.order_id AND de.user_id=p.user_id)))`, id).Scan(&user, &bank, &key, &raw, &amount, &fund)
	if err != nil {
		return err
	}
	if fund == nil {
		details, err := decryptBank(user, bank, key, raw)
		if err != nil {
			return err
		}
		var contact struct {
			ID string `json:"id"`
		}
		if err = s.Gateway.request(ctx, "POST", "/contacts", "", map[string]any{"name": details.Holder, "type": "vendor", "reference_id": user}, &contact); err != nil {
			return err
		}
		if contact.ID == "" {
			return errors.New("beneficiary contact reference missing")
		}
		var account struct {
			ID string `json:"id"`
		}
		if err = s.Gateway.request(ctx, "POST", "/fund_accounts", "", map[string]any{"contact_id": contact.ID, "account_type": "bank_account", "bank_account": map[string]string{"name": details.Holder, "ifsc": details.Routing, "account_number": details.Number}}, &account); err != nil {
			return err
		}
		if account.ID == "" {
			return errors.New("beneficiary fund account missing")
		}
		if _, err = s.DB.Exec(ctx, `UPDATE finance.bank_accounts SET provider_fund_account=$2 WHERE id=$1 AND status='VERIFIED'`, bank, account.ID); err != nil {
			return err
		}
		fund = &account.ID
	}
	var payoutKey string
	if err = s.DB.QueryRow(ctx, `SELECT a.idempotency_key FROM finance.payout_attempts a JOIN finance.payouts p ON p.id=a.payout_id AND p.attempt_no=a.attempt_no WHERE p.id=$1`, id).Scan(&payoutKey); err != nil {
		return err
	}
	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Amount int64  `json:"amount"`
	}
	body := map[string]any{"account_number": os.Getenv("RAZORPAYX_ACCOUNT_NUMBER"), "fund_account_id": *fund, "amount": amount, "currency": "INR", "mode": "IMPS", "purpose": "payout", "queue_if_low_balance": true, "reference_id": id, "narration": "Yember settlement"}
	if err = s.Gateway.request(ctx, "POST", "/payouts", payoutKey, body, &result); err != nil {
		_, _ = s.DB.Exec(ctx, `UPDATE finance.payouts SET status='UNKNOWN',provider='RAZORPAYX',updated_at=now() WHERE id=$1 AND status='PROCESSING'`, id)
		return err
	}
	if result.ID == "" || result.Amount != amount {
		return errors.New("payout provider response mismatch")
	}
	if _, err = s.DB.Exec(ctx, `UPDATE finance.payout_attempts a SET provider_id=$2 FROM finance.payouts p WHERE a.payout_id=p.id AND a.attempt_no=p.attempt_no AND p.id=$1`, id, result.ID); err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `UPDATE finance.payouts SET provider='RAZORPAYX',provider_id=$2,updated_at=now() WHERE id=$1 AND status IN ('PROCESSING','UNKNOWN')`, id, result.ID)
	return err
}

func (s *Service) ReconcileRazorpayX(ctx context.Context, actor, id string) error {
	var providerID string
	var amount int64
	if err := s.DB.QueryRow(ctx, `SELECT provider_id,amount FROM finance.payouts WHERE id=$1 AND provider='RAZORPAYX' AND status IN ('PROCESSING','UNKNOWN','SUCCEEDED')`, id).Scan(&providerID, &amount); err != nil {
		return errors.New("provider payout reference required")
	}
	var result struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		Amount    int64  `json:"amount"`
		Currency  string `json:"currency"`
		UTR       string `json:"utr"`
		Reference string `json:"reference_id"`
	}
	if err := s.Gateway.request(ctx, "GET", "/payouts/"+url.PathEscape(providerID), "", nil, &result); err != nil {
		return err
	}
	if result.Amount != amount || result.Currency != "INR" || result.ID != providerID || result.Reference != id {
		return errors.New("payout reconciliation mismatch")
	}
	status := "UNKNOWN"
	if result.Status == "processed" {
		status = "SUCCEEDED"
	}
	if result.Status == "failed" || result.Status == "rejected" || result.Status == "cancelled" {
		status = "FAILED"
	}
	if result.Status == "reversed" {
		return s.flagPayoutReversal(ctx, id, providerID)
	}
	reference := providerID + "/" + result.UTR
	return s.confirm(ctx, actor, id, status, reference, true)
}
func (s *Service) flagPayoutReversal(ctx context.Context, id, reference string) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO finance.discrepancies(event_key,kind,details) VALUES($1,'PAYOUT_REVERSAL',jsonb_build_object('payout_id',$2::text,'provider_id',$3::text)) ON CONFLICT DO NOTHING`, "payout-reversal/"+reference, id, reference)
	return err
}

func (s *Service) RecoverLink(ctx context.Context, actor, id, providerID string) error {
	var link PaymentLink
	if err := s.Gateway.request(ctx, "GET", "/payment_links/"+url.PathEscape(providerID), "", nil, &link); err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if link.ID != providerID || link.Reference != id || link.URL == "" {
		return errors.New("verified link reference mismatch")
	}
	tag, err := tx.Exec(ctx, `UPDATE finance.payment_attempts SET provider_id=$2,checkout_url=$3,status='PENDING' WHERE id=$1 AND provider='RAZORPAY' AND provider_id IS NULL AND amount=$4 AND currency=$5 AND id::text=$6`, id, link.ID, link.URL, link.Amount, link.Currency, link.Reference)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		tag, err = tx.Exec(ctx, `UPDATE finance.onboarding_attempts SET provider_id=$2,checkout_url=$3,status='PENDING',expires_at=to_timestamp($7) WHERE id=$1 AND status='CREATING' AND provider_id IS NULL AND amount=$4 AND currency=$5 AND id::text=$6`, id, link.ID, link.URL, link.Amount, link.Currency, link.Reference, link.Expires)
		if err != nil {
			return err
		}
	}
	if tag.RowsAffected() != 1 {
		return errors.New("verified link reference mismatch")
	}
	if err = audit(ctx, tx, actor, "PAYMENT_LINK_RECOVERED", id, providerID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

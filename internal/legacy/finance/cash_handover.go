package finance

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

// RecordCashCollection belongs to the same transaction as the agent's delivery proof.
// Physical cash is an asset in transit, never a receipt into Yember's bank account.
func RecordCashCollection(ctx context.Context, tx pgx.Tx, order string) error {
	var amount int64
	var attempt string
	if err := tx.QueryRow(ctx, `SELECT c.amount,p.id FROM finance.cod_custody c JOIN finance.payment_attempts p ON p.order_id=c.order_id AND p.provider='COD' WHERE c.order_id=$1`, order).Scan(&amount, &attempt); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE finance.payment_attempts SET status='PAID',payment_reference='cash/'||order_id::text WHERE id=$1 AND status IN ('PENDING','CREATING','PAID')`, attempt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("cash payment state requires review")
	}
	return post(ctx, tx, "capture/"+order, "CASH_COLLECTION", order, "cash/"+order, "Agent confirmed cash receipt", "", Line{"CASH_IN_TRANSIT", "", "cash_collection", amount}, Line{"UNEARNED", "", "order_receipt", -amount})
}

func (s *Service) SubmitCashHandover(ctx context.Context, user, order string, amount int64) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var expected int64
	if err = tx.QueryRow(ctx, `SELECT c.amount FROM finance.cod_custody c JOIN public.riders p ON p.id=c.partner_id JOIN public.orders o ON o.id=c.order_id WHERE c.order_id=$1 AND p.user_id=$2 AND o.status='DELIVERED' FOR UPDATE OF o`, order, user).Scan(&expected); err != nil {
		return errors.New("delivered cash order assigned to this agent required")
	}
	if amount <= 0 {
		return errors.New("positive cash handover amount required")
	}
	status := "PENDING"
	if amount != expected {
		status = "DISPUTED"
	}
	var prior int64
	err = tx.QueryRow(ctx, `SELECT amount_paise FROM finance.cash_handovers WHERE order_id=$1`, order).Scan(&prior)
	if err == nil {
		if prior != amount {
			return errors.New("handover already submitted with another amount")
		}
		return nil
	}
	if err != pgx.ErrNoRows {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO finance.cash_handovers(order_id,amount_paise,status,submitted_by) VALUES($1,$2,$3,$4)`, order, amount, status, user); err != nil {
		return err
	}
	if status == "DISPUTED" {
		if _, err = tx.Exec(ctx, `INSERT INTO finance.discrepancies(event_key,kind,order_id,details) VALUES($1,'CASH_HANDOVER',$2,jsonb_build_object('expected',$3::bigint,'received',$4::bigint)) ON CONFLICT DO NOTHING`, "handover/"+order, order, expected, amount); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) ConfirmCashHandover(ctx context.Context, user, order string, amount int64, accept bool) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(894213)`); err != nil {
		return err
	}
	// The order lock serializes recognition and payout reservation with this handover.
	var restaurant string
	var total, net int64
	if err = tx.QueryRow(ctx, `SELECT f.restaurant_user,f.total,f.gross-f.commission FROM finance.order_financials f JOIN public.orders o ON o.id=f.order_id WHERE f.order_id=$1 AND f.restaurant_user=$2 AND o.status='DELIVERED' FOR UPDATE OF o`, order, user).Scan(&restaurant, &total, &net); err != nil {
		return errors.New("cash order owned by this restaurant required")
	}
	var recorded int64
	var status string
	if err = tx.QueryRow(ctx, `SELECT amount_paise,status FROM finance.cash_handovers WHERE order_id=$1 FOR UPDATE`, order).Scan(&recorded, &status); err != nil {
		return errors.New("agent must submit cash handover first")
	}
	if amount != recorded {
		return errors.New("acknowledged amount must match agent handover")
	}
	if status == "PAID" {
		if accept {
			return nil
		}
		return errors.New("paid handover cannot be disputed through this operation")
	}
	if !accept {
		if _, err = tx.Exec(ctx, `UPDATE finance.cash_handovers SET status='DISPUTED',updated_at=now() WHERE order_id=$1`, order); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO finance.discrepancies(event_key,kind,order_id,details) VALUES($1,'CASH_HANDOVER',$2,'{"restaurant_disputed":true}') ON CONFLICT DO NOTHING`, "handover/"+order, order); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if status != "PENDING" || amount != total {
		return errors.New("disputed or mismatched cash requires reconciliation")
	}
	// Do not pay earnings already reserved for a bank payout.
	var reserved bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM finance.settlement_entries se JOIN finance.entries e ON e.id=se.entry_id JOIN finance.journals j ON j.id=e.journal_id WHERE j.order_id=$1 AND e.user_id=$2 AND NOT EXISTS(SELECT 1 FROM finance.settlement_releases r WHERE r.entry_id=se.entry_id AND r.payout_id=se.payout_id))`, order, restaurant).Scan(&reserved); err != nil {
		return err
	}
	if reserved {
		return errors.New("cash handover conflicts with reserved bank settlement")
	}
	if err = Recognize(ctx, tx, order); err != nil {
		return err
	}
	eligible := min(amount, net)
	if err = post(ctx, tx, "cash-handover/"+order, "CASH_HANDOVER", order, order, "Restaurant acknowledged gross cash handover", user, Line{"CASH_IN_TRANSIT", "", "cash_handover", -amount}, Line{"PARTNER_PAYABLE", restaurant, "cash_handover", eligible}, Line{"RESTAURANT_FEE_RECEIVABLE", "", "cash_fees", amount - eligible}); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE finance.cash_handovers SET status='PAID',confirmed_by=$2,confirmed_at=now(),updated_at=now() WHERE order_id=$1`, order, user); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

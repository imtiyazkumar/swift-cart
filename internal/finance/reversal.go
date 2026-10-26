package finance

import (
	"context"
	"errors"
	"net/url"
)

func (s *Service) ReconcileReversal(ctx context.Context, actor, discrepancy, reference string) error {
	if len(reference) < 10 {
		return errors.New("bank reconciliation evidence required")
	}
	var payout, providerID, user, initiator string
	var amount int64
	err := s.DB.QueryRow(ctx, `SELECT p.id,p.provider_id,p.user_id,COALESCE(p.initiated_by::text,''),p.amount FROM finance.discrepancies d JOIN finance.payouts p ON p.id::text=d.details->>'payout_id' WHERE d.id=$1 AND d.kind='PAYOUT_REVERSAL' AND d.resolved_at IS NULL`, discrepancy).Scan(&payout, &providerID, &user, &initiator, &amount)
	if err != nil {
		return errors.New("open payout reversal required")
	}
	if actor == initiator {
		return errors.New("independent reconciliation required")
	}
	var result struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
	}
	if err = s.Gateway.request(ctx, "GET", "/payouts/"+url.PathEscape(providerID), "", nil, &result); err != nil {
		return err
	}
	if result.ID != providerID || result.Status != "reversed" || result.Amount != amount || result.Currency != "INR" {
		return errors.New("provider has not confirmed the expected reversal")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(894213)`); err != nil {
		return err
	}
	var paid bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM finance.journals WHERE event_key=$1)`, "payout/"+payout).Scan(&paid); err != nil {
		return err
	}
	if paid {
		if err = post(ctx, tx, "payout-reversal/"+payout, "PAYOUT_REVERSAL", "", providerID, "Provider-confirmed bank payout reversal", actor, Line{"CASH", "", "payout_reversal", amount}, Line{"PARTNER_PAYABLE", user, "payout_reversal", -amount}); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE finance.discrepancies SET resolved_by=$2,resolution=$3,resolved_at=now() WHERE id=$1 AND resolved_at IS NULL`, discrepancy, actor, reference); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO finance.payout_history(payout_id,status,reference,actor) VALUES($1,'REVERSED',$2,$3)`, payout, reference, actor); err != nil {
		return err
	}
	if err = audit(ctx, tx, actor, "PAYOUT_REVERSAL_RECONCILED", payout, reference); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

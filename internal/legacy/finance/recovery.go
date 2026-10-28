package finance

import (
	"context"
	"errors"
)

func (s *Service) CancelPayout(ctx context.Context, actor, id, reason string) error {
	if len(reason) < 10 || len(reason) > 2000 {
		return errors.New("cancellation explanation required")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(894213)`); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE finance.payouts SET status='CANCELLED',updated_at=now() WHERE id=$1 AND status IN ('PENDING','FAILED')`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("only unsent or confirmed-failed payouts may be cancelled")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO finance.settlement_releases(entry_id,payout_id,actor,reason) SELECT entry_id,payout_id,$2,$3 FROM finance.settlement_entries WHERE payout_id=$1`, id, actor, reason); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO finance.payout_history(payout_id,status,reference,actor) VALUES($1,'CANCELLED',$2,$3)`, id, reason, actor); err != nil {
		return err
	}
	if err = audit(ctx, tx, actor, "PAYOUT_CANCELLED", id, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) ResolveDiscrepancy(ctx context.Context, actor, id, reason string) error {
	if len(reason) < 20 || len(reason) > 2000 {
		return errors.New("detailed resolution and reconciliation evidence required")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var kind string
	if err = tx.QueryRow(ctx, `SELECT kind FROM finance.discrepancies WHERE id=$1 AND resolved_at IS NULL FOR UPDATE`, id).Scan(&kind); err != nil {
		return errors.New("open discrepancy not found")
	}
	// Recognized refunds already debit platform refund expense. This policy does not claw back partner earnings.
	if kind != "REFUND_REQUIRES_ALLOCATION" {
		return errors.New("chargeback/payout reversal requires verified bank movement before liability adjustment; keep this discrepancy open")
	}
	if _, err = tx.Exec(ctx, `UPDATE finance.discrepancies SET resolved_by=$2,resolution=$3,resolved_at=now() WHERE id=$1`, id, actor, "Platform absorbs refund under central policy. "+reason); err != nil {
		return err
	}
	if err = audit(ctx, tx, actor, "REFUND_ALLOCATION_REVIEWED", id, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

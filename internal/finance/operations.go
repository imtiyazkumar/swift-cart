package finance

import (
	"context"
	"errors"
	"go.uber.org/zap"
	"os"
	"time"
)

func (s *Service) COD(ctx context.Context, actor, order, reference string) error {
	if len(reference) < 8 || len(reference) > 200 {
		return errors.New("central bank remittance reference required")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var custody bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM finance.cod_custody WHERE order_id=$1)`, order).Scan(&custody); err != nil {
		return err
	}
	if custody {
		return errors.New("agent cash must be handed to and acknowledged by the restaurant")
	}
	var total int64
	var currency, status string
	if err = tx.QueryRow(ctx, `SELECT f.total,f.currency,o.status FROM finance.order_financials f JOIN public.orders o ON o.id=f.order_id WHERE f.order_id=$1 FOR UPDATE OF o`, order).Scan(&total, &currency, &status); err != nil {
		return err
	}
	if status != "DELIVERED" {
		return errors.New("delivered COD order required")
	}
	var id string
	if err = tx.QueryRow(ctx, `SELECT id FROM finance.payment_attempts WHERE order_id=$1 AND provider='COD'`, order).Scan(&id); err != nil {
		return errors.New("COD payment record required")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO finance.reconciliations(actor,reference,details) VALUES($1,$2,jsonb_build_object('order_id',$3::text,'amount',$4::bigint)) ON CONFLICT(reference) DO NOTHING`, actor, reference, order, total); err != nil {
		return err
	}
	if err = capture(ctx, tx, order, id, reference, total); err != nil {
		return err
	}
	if err = audit(ctx, tx, actor, "COD_REMITTED", order, reference); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func Run(ctx context.Context, s *Service, log *zap.Logger) {
	if os.Getenv("FINANCE_SCHEDULER_ENABLED") != "true" {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.ProcessRefundJob(ctx); err != nil {
				log.Warn("cancellation refund requires reconciliation", zap.Error(err))
			}
			var enabled bool
			if err := s.DB.QueryRow(ctx, `SELECT enabled FROM finance.policy`).Scan(&enabled); err != nil {
				log.Warn("finance scheduler unavailable", zap.Error(err))
				continue
			}
			if !enabled {
				continue
			}
			if _, err := s.Generate(ctx, "", time.Now()); err != nil {
				log.Warn("settlement preparation failed", zap.Error(err))
			}
		}
	}
}

// Adjustment is an explicit journal, never an edit to an order snapshot or an old payout.
func (s *Service) Adjustment(ctx context.Context, actor, user, order, key, component, reason string, amount int64) error {
	if amount == 0 || amount > 100000000 || amount < -100000000 || len(key) < 8 || len(key) > 120 || len(reason) < 10 {
		return errors.New("bounded nonzero adjustment, stable key and detailed reason required")
	}
	switch component {
	case "bonus", "incentive", "adjustment", "refund":
	default:
		return errors.New("unsupported adjustment component")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(894213)`); err != nil {
		return err
	}
	var role string
	if err = tx.QueryRow(ctx, `SELECT role FROM public.users WHERE id=$1 AND role IN ('RESTAURANT_OWNER','DELIVERY_PARTNER') AND disabled_at IS NULL AND deleted_at IS NULL FOR UPDATE`, user).Scan(&role); err != nil {
		return errors.New("eligible partner required")
	}
	if amount < 0 {
		return errors.New("negative partner adjustments require separately verified contractual authorization; automatic clawback is disabled")
	}
	if err = post(ctx, tx, "adjustment/"+key, "ADJUSTMENT", order, key, reason, actor, Line{"ADJUSTMENT", "", component, amount}, Line{"PARTNER_PAYABLE", user, component, -amount}); err != nil {
		return err
	}
	if err = audit(ctx, tx, actor, "PARTNER_ADJUSTMENT", user, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

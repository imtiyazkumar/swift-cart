package finance

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"os"
)

// Cancellation refunds have a stable order-scoped reference. An uncertain provider
// response remains in REVIEW and is never resubmitted with a fresh reference.
func (s *Service) ProcessRefundJob(ctx context.Context) error {
	actor := os.Getenv("FINANCE_AUTOMATION_ADMIN_ID")
	if actor == "" {
		return nil
	}
	var authorized bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM finance.permissions p JOIN public.users u ON u.id=p.user_id WHERE p.user_id=$1 AND p.permission='ADJUST' AND u.role='ADMIN' AND u.disabled_at IS NULL AND u.deleted_at IS NULL)`, actor).Scan(&authorized); err != nil {
		return err
	}
	if !authorized {
		return errors.New("refund automation permission unavailable")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var order string
	var amount int64
	err = tx.QueryRow(ctx, `SELECT j.order_id,f.total FROM finance.refund_jobs j JOIN finance.order_financials f ON f.order_id=j.order_id JOIN public.orders o ON o.id=j.order_id CROSS JOIN finance.refund_policy p WHERE j.status='PENDING' AND o.status IN ('CANCELLED','REJECTED') AND p.unfulfilled_full_refund AND NOT EXISTS(SELECT 1 FROM finance.journals e WHERE e.order_id=j.order_id AND e.kind='EARN') ORDER BY j.created_at LIMIT 1 FOR UPDATE OF j SKIP LOCKED`).Scan(&order, &amount)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE finance.refund_jobs SET status='REVIEW' WHERE order_id=$1`, order); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	err = s.Refund(ctx, actor, order, "cancellation/"+order, "Central policy: unfulfilled cancelled/rejected order", amount)
	if err != nil {
		_, _ = s.DB.Exec(ctx, `UPDATE finance.refund_jobs SET last_error='Provider response requires reconciliation' WHERE order_id=$1`, order)
		return err
	}
	_, err = s.DB.Exec(ctx, `UPDATE finance.refund_jobs SET status='SUBMITTED',last_error=NULL WHERE order_id=$1`, order)
	return err
}

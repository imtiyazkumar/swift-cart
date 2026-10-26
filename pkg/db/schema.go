package db

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CheckSchema validates required runtime relations and columns without changing
// data. Migrations remain a separate, explicitly run deployment step.
func CheckSchema(ctx context.Context, pool *pgxpool.Pool) error {
	const checks = `
 SELECT scope,client,window_start,count FROM auth.rate_windows LIMIT 0;
 SELECT delivery_charge,rider_base FROM finance.policy LIMIT 0;
 SELECT commission_bps,commission,total FROM finance.order_financials LIMIT 0;
 SELECT permission FROM finance.permissions LIMIT 0;
 SELECT idempotency_key FROM finance.payout_attempts LIMIT 0;
 SELECT provider_id,payment_reference FROM finance.onboarding_attempts LIMIT 0;
 SELECT family_id,family_expires_at,parent_hash FROM auth.refresh_sessions LIMIT 0;
 SELECT provider_reference,status FROM notifications.sms_outbox LIMIT 0;
 SELECT sent_at,resend_count FROM delivery.completion_challenges LIMIT 0;`
	if _, err := pool.Exec(ctx, checks); err != nil {
		return fmt.Errorf("database schema is not ready: run 'go run ./cmd/migrate' from yember-core (required through 053_delivery_code_recovery.sql): %w", err)
	}
	return nil
}

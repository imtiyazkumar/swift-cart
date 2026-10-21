package payments

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type pgRepo struct {
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewPostgresRepo(db *pgxpool.Pool, log *zap.Logger) Repository {
	return &pgRepo{db: db, log: log}
}

func (r *pgRepo) CreateIntent(ctx context.Context, customerID string, intent *Intent) error {
	return r.db.QueryRow(ctx, `INSERT INTO payments.payments (order_id, provider, method, amount, currency, status, idempotency_key)
		SELECT o.id, $3,$4,$5,$6,$7,$8 FROM public.orders o
		WHERE o.id=$1 AND o.customer_id=$2 AND ROUND(o.total_price * 100)::BIGINT=$5
		RETURNING id, created_at`,
		intent.OrderID, customerID, intent.Provider, intent.Method, intent.Amount, intent.Currency, intent.Status, intent.IdempotencyKey).Scan(&intent.ID, &intent.CreatedAt)
}

func (r *pgRepo) MarkWebhookProcessed(ctx context.Context, provider, eventID string) error {
	_, err := r.db.Exec(ctx, `INSERT INTO payments.transactions (provider, provider_event_id, status) VALUES ($1,$2,'PROCESSED') ON CONFLICT (provider, provider_event_id) DO NOTHING`, provider, eventID)
	return err
}

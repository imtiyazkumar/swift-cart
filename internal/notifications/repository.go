package notifications

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

func (r *pgRepo) Log(ctx context.Context, n *Notification) error {
	return r.db.QueryRow(ctx, `INSERT INTO notifications.notification_logs (user_id, channel, template_key, payload, status)
		VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at`, n.UserID, n.Channel, n.TemplateKey, n.Payload, n.Status).Scan(&n.ID, &n.CreatedAt)
}

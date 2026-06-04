package delivery

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

func (r *pgRepo) UpsertAvailability(ctx context.Context, partnerID, status string) error {
	_, err := r.db.Exec(ctx, `UPDATE delivery.delivery_partners SET status=$1, updated_at=now() WHERE id=$2`, status, partnerID)
	return err
}

func (r *pgRepo) SaveLocation(ctx context.Context, u LocationUpdate) error {
	_, err := r.db.Exec(ctx, `INSERT INTO delivery.live_locations (partner_id, latitude, longitude, recorded_at) VALUES ($1,$2,$3,$4)`, u.PartnerID, u.Latitude, u.Longitude, u.RecordedAt)
	return err
}

func (r *pgRepo) CreateAssignment(ctx context.Context, a *Assignment) error {
	return r.db.QueryRow(ctx, `INSERT INTO delivery.delivery_assignments (order_id, partner_id, status) VALUES ($1,$2,$3) RETURNING id, created_at, updated_at`, a.OrderID, a.PartnerID, a.Status).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

func (r *pgRepo) UpdateAssignmentStatus(ctx context.Context, orderID, partnerID, status string) error {
	_, err := r.db.Exec(ctx, `UPDATE delivery.delivery_assignments SET status=$1, updated_at=now() WHERE order_id=$2 AND partner_id=$3`, status, orderID, partnerID)
	return err
}

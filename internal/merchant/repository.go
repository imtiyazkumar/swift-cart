package merchant

import (
	"context"
	"github.com/imtiyazkumar/yember-core/internal/stations"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Repository interface {
	CreateMerchant(ctx context.Context, m *Merchant) (string, error)
	GetMerchantByID(ctx context.Context, id string) (*Merchant, error)
	ListMerchants(ctx context.Context) ([]*Merchant, error)
	UpdateMerchant(ctx context.Context, m *Merchant) error
	DeleteMerchant(ctx context.Context, id string) error
}

type pgRepo struct {
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewPostgresRepo(db *pgxpool.Pool, log *zap.Logger) Repository {
	return &pgRepo{db: db, log: log}
}

func (r *pgRepo) CreateMerchant(ctx context.Context, m *Merchant) (string, error) {
	var id string
	query := `INSERT INTO merchants (name, email, phone, merchant_type, onboarding_status, approval_status,latitude,longitude) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,station_id`
	err := r.db.QueryRow(ctx, query, m.Name, m.Email, m.Phone, m.MerchantType, m.OnboardingStatus, m.ApprovalStatus, m.Latitude, m.Longitude).Scan(&id, &m.StationID)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (r *pgRepo) GetMerchantByID(ctx context.Context, id string) (*Merchant, error) {
	var m Merchant
	query := `SELECT id, name, email, phone, merchant_type, onboarding_status, approval_status,COALESCE(station_id::text,''),latitude,longitude, created_at, updated_at FROM merchants WHERE id=$1`
	row := r.db.QueryRow(ctx, query, id)
	err := row.Scan(&m.ID, &m.Name, &m.Email, &m.Phone, &m.MerchantType, &m.OnboardingStatus, &m.ApprovalStatus, &m.StationID, &m.Latitude, &m.Longitude, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *pgRepo) ListMerchants(ctx context.Context) ([]*Merchant, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name, email, phone, merchant_type, onboarding_status, approval_status,COALESCE(station_id::text,''),latitude,longitude, created_at, updated_at FROM merchants`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Merchant
	for rows.Next() {
		var m Merchant
		if err := rows.Scan(&m.ID, &m.Name, &m.Email, &m.Phone, &m.MerchantType, &m.OnboardingStatus, &m.ApprovalStatus, &m.StationID, &m.Latitude, &m.Longitude, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &m)
	}
	return list, rows.Err()
}

func (r *pgRepo) UpdateMerchant(ctx context.Context, m *Merchant) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = stations.LockCoverage(ctx, tx); err != nil {
		return err
	}
	query := `UPDATE merchants SET name=$1, email=$2, phone=$3, merchant_type=$4, onboarding_status=$5, approval_status=$6, updated_at=NOW() WHERE id=$7`
	_, err = tx.Exec(ctx, query, m.Name, m.Email, m.Phone, m.MerchantType, m.OnboardingStatus, m.ApprovalStatus, m.ID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *pgRepo) DeleteMerchant(ctx context.Context, id string) error {
	query := `DELETE FROM merchants WHERE id=$1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

package order

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"time"
)

type Repository interface {
	Create(ctx context.Context, o *Order) error
	GetByID(ctx context.Context, id string) (*Order, error)
	List(ctx context.Context) ([]*Order, error)
	UpdateStatus(ctx context.Context, id string, status string) error
	Delete(ctx context.Context, id string) error
}

type pgRepo struct {
	pool *pgxpool.Pool
	log  *zap.Logger
}

func NewPostgresRepo(pool *pgxpool.Pool, log *zap.Logger) Repository {
	return &pgRepo{pool: pool, log: log}
}

func (r *pgRepo) Create(ctx context.Context, o *Order) error {
	if o.ID == "" {
		o.ID = uuid.New().String()
	}
	query := `INSERT INTO orders (id, merchant_id, customer_id, status, total_price, created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7)`
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, query, o.ID, o.MerchantID, o.CustomerID, o.Status, o.TotalPrice, now, now)
	return err
}

func (r *pgRepo) GetByID(ctx context.Context, id string) (*Order, error) {
	o := &Order{}
	query := `SELECT id, merchant_id, customer_id, status, total_price, created_at, updated_at FROM orders WHERE id=$1`
	err := r.pool.QueryRow(ctx, query, id).Scan(&o.ID, &o.MerchantID, &o.CustomerID, &o.Status, &o.TotalPrice, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (r *pgRepo) List(ctx context.Context) ([]*Order, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, merchant_id, customer_id, status, total_price, created_at, updated_at FROM orders`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var orders []*Order
	for rows.Next() {
		o := &Order{}
		if err := rows.Scan(&o.ID, &o.MerchantID, &o.CustomerID, &o.Status, &o.TotalPrice, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

func (r *pgRepo) UpdateStatus(ctx context.Context, id string, status string) error {
	query := `UPDATE orders SET status=$1, updated_at=$2 WHERE id=$3`
	_, err := r.pool.Exec(ctx, query, status, time.Now().UTC(), id)
	return err
}

func (r *pgRepo) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM orders WHERE id=$1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

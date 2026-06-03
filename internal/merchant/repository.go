package merchant

import (
    "context"
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
    query := `INSERT INTO merchants (name, email, phone) VALUES ($1,$2,$3) RETURNING id`
    err := r.db.QueryRow(ctx, query, m.Name, m.Email, m.Phone).Scan(&id)
    if err != nil {
        return "", err
    }
    return id, nil
}

func (r *pgRepo) GetMerchantByID(ctx context.Context, id string) (*Merchant, error) {
    var m Merchant
    query := `SELECT id, name, email, phone, created_at, updated_at FROM merchants WHERE id=$1`
    row := r.db.QueryRow(ctx, query, id)
    err := row.Scan(&m.ID, &m.Name, &m.Email, &m.Phone, &m.CreatedAt, &m.UpdatedAt)
    if err != nil {
        return nil, err
    }
    return &m, nil
}

func (r *pgRepo) ListMerchants(ctx context.Context) ([]*Merchant, error) {
    rows, err := r.db.Query(ctx, `SELECT id, name, email, phone, created_at, updated_at FROM merchants`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var list []*Merchant
    for rows.Next() {
        var m Merchant
        if err := rows.Scan(&m.ID, &m.Name, &m.Email, &m.Phone, &m.CreatedAt, &m.UpdatedAt); err != nil {
            return nil, err
        }
        list = append(list, &m)
    }
    return list, rows.Err()
}

func (r *pgRepo) UpdateMerchant(ctx context.Context, m *Merchant) error {
    query := `UPDATE merchants SET name=$1, email=$2, phone=$3, updated_at=NOW() WHERE id=$4`
    _, err := r.db.Exec(ctx, query, m.Name, m.Email, m.Phone, m.ID)
    return err
}

func (r *pgRepo) DeleteMerchant(ctx context.Context, id string) error {
    query := `DELETE FROM merchants WHERE id=$1`
    _, err := r.db.Exec(ctx, query, id)
    return err
}

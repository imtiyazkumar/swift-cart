package customer

import (
    "context"
    "github.com/jackc/pgx/v5/pgxpool"
    "go.uber.org/zap"
)

type Repository interface {
    CreateCustomer(ctx context.Context, c *Customer) (string, error)
    GetCustomerByID(ctx context.Context, id string) (*Customer, error)
    ListCustomers(ctx context.Context) ([]*Customer, error)
    UpdateCustomer(ctx context.Context, c *Customer) error
    DeleteCustomer(ctx context.Context, id string) error
}

type pgRepo struct {
    db  *pgxpool.Pool
    log *zap.Logger
}

func NewPostgresRepo(db *pgxpool.Pool, log *zap.Logger) Repository {
    return &pgRepo{db: db, log: log}
}

func (r *pgRepo) CreateCustomer(ctx context.Context, c *Customer) (string, error) {
    var id string
    query := `INSERT INTO customers (name, email, phone, address) VALUES ($1,$2,$3,$4) RETURNING id`
    err := r.db.QueryRow(ctx, query, c.Name, c.Email, c.Phone, c.Address).Scan(&id)
    if err != nil {
        return "", err
    }
    return id, nil
}

func (r *pgRepo) GetCustomerByID(ctx context.Context, id string) (*Customer, error) {
    var c Customer
    query := `SELECT id, name, email, phone, address, created_at, updated_at FROM customers WHERE id=$1`
    row := r.db.QueryRow(ctx, query, id)
    err := row.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Address, &c.CreatedAt, &c.UpdatedAt)
    if err != nil {
        return nil, err
    }
    return &c, nil
}

func (r *pgRepo) ListCustomers(ctx context.Context) ([]*Customer, error) {
    rows, err := r.db.Query(ctx, `SELECT id, name, email, phone, address, created_at, updated_at FROM customers`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var list []*Customer
    for rows.Next() {
        var c Customer
        if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Address, &c.CreatedAt, &c.UpdatedAt); err != nil {
            return nil, err
        }
        list = append(list, &c)
    }
    return list, rows.Err()
}

func (r *pgRepo) UpdateCustomer(ctx context.Context, c *Customer) error {
    query := `UPDATE customers SET name=$1, email=$2, phone=$3, address=$4, updated_at=NOW() WHERE id=$5`
    _, err := r.db.Exec(ctx, query, c.Name, c.Email, c.Phone, c.Address, c.ID)
    return err
}

func (r *pgRepo) DeleteCustomer(ctx context.Context, id string) error {
    query := `DELETE FROM customers WHERE id=$1`
    _, err := r.db.Exec(ctx, query, id)
    return err
}

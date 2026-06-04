package catalog

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Repository interface {
	CreateCategory(ctx context.Context, category *Category) error
	CreateItem(ctx context.Context, item *Item) error
	GetItem(ctx context.Context, id string) (*Item, error)
	ListMerchantItems(ctx context.Context, merchantID string) ([]*Item, error)
	SearchItems(ctx context.Context, query string, limit int) ([]*Item, error)
	UpdateItem(ctx context.Context, item *Item) error
}

type pgRepo struct {
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewPostgresRepo(db *pgxpool.Pool, log *zap.Logger) Repository {
	return &pgRepo{db: db, log: log}
}

func (r *pgRepo) CreateCategory(ctx context.Context, c *Category) error {
	query := `INSERT INTO catalog.categories (merchant_id, name, sort_order, is_active)
		VALUES ($1,$2,$3,$4) RETURNING id, created_at, updated_at`
	return r.db.QueryRow(ctx, query, c.MerchantID, c.Name, c.SortOrder, c.IsActive).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (r *pgRepo) CreateItem(ctx context.Context, i *Item) error {
	query := `INSERT INTO catalog.items
		(merchant_id, category_id, name, description, item_type, is_vegetarian, base_price, currency, is_available, inventory_tracked, substitution_policy)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, created_at, updated_at`
	return r.db.QueryRow(ctx, query, i.MerchantID, i.CategoryID, i.Name, i.Description, i.ItemType, i.IsVegetarian, i.BasePrice, i.Currency, i.IsAvailable, i.InventoryTracked, i.SubstitutionPolicy).Scan(&i.ID, &i.CreatedAt, &i.UpdatedAt)
}

func (r *pgRepo) GetItem(ctx context.Context, id string) (*Item, error) {
	item := &Item{}
	err := r.db.QueryRow(ctx, itemSelect()+` WHERE id=$1 AND deleted_at IS NULL`, id).Scan(
		&item.ID, &item.MerchantID, &item.CategoryID, &item.Name, &item.Description, &item.ItemType, &item.IsVegetarian,
		&item.BasePrice, &item.Currency, &item.IsAvailable, &item.InventoryTracked, &item.SubstitutionPolicy, &item.CreatedAt, &item.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (r *pgRepo) ListMerchantItems(ctx context.Context, merchantID string) ([]*Item, error) {
	rows, err := r.db.Query(ctx, itemSelect()+` WHERE merchant_id=$1 AND deleted_at IS NULL ORDER BY name`, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows)
}

func (r *pgRepo) SearchItems(ctx context.Context, query string, limit int) ([]*Item, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, itemSelect()+` WHERE deleted_at IS NULL AND name ILIKE '%' || $1 || '%' ORDER BY name LIMIT $2`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows)
}

func (r *pgRepo) UpdateItem(ctx context.Context, i *Item) error {
	query := `UPDATE catalog.items SET category_id=$1, name=$2, description=$3, item_type=$4, is_vegetarian=$5,
		base_price=$6, currency=$7, is_available=$8, inventory_tracked=$9, substitution_policy=$10, updated_at=now()
		WHERE id=$11 AND deleted_at IS NULL`
	_, err := r.db.Exec(ctx, query, i.CategoryID, i.Name, i.Description, i.ItemType, i.IsVegetarian, i.BasePrice, i.Currency, i.IsAvailable, i.InventoryTracked, i.SubstitutionPolicy, i.ID)
	return err
}

func itemSelect() string {
	return `SELECT id, merchant_id, category_id, name, description, item_type, is_vegetarian,
		base_price, currency, is_available, inventory_tracked, substitution_policy, created_at, updated_at FROM catalog.items`
}

func scanItems(rows pgx.Rows) ([]*Item, error) {
	var items []*Item
	for rows.Next() {
		item := &Item{}
		if err := rows.Scan(&item.ID, &item.MerchantID, &item.CategoryID, &item.Name, &item.Description, &item.ItemType, &item.IsVegetarian, &item.BasePrice, &item.Currency, &item.IsAvailable, &item.InventoryTracked, &item.SubstitutionPolicy, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

package restaurant

import (
    "context"
    "fmt"
    "github.com/jackc/pgx/v5/pgxpool"
    "go.uber.org/zap"
    "math"
)

type Repository interface {
    GetByOwnerID(ctx context.Context, ownerID string) (*RestaurantProfile, error)
    UpdateStatus(ctx context.Context, merchantID string, isActive bool) error
    ListCategories(ctx context.Context, merchantID string) ([]*MenuCategory, error)
    CreateCategory(ctx context.Context, cat *MenuCategory) error
    SaveMenuItem(ctx context.Context, item *MenuItemInput) (string, error)
    ToggleItemAvailability(ctx context.Context, itemID string, isAvailable bool) error
    GetIncomingOrders(ctx context.Context, merchantID string) ([]*RestaurantOrder, error)
    UpdateOrderStatus(ctx context.Context, merchantID, orderID, status string) error
    GetDailySummary(ctx context.Context, merchantID string) (*DailySalesSummary, error)
}

type merchantContextKey struct{}
type orderReasonContextKey struct{}

type pgRepo struct {
    db  *pgxpool.Pool
    log *zap.Logger
}

func NewPostgresRepo(db *pgxpool.Pool, log *zap.Logger) Repository {
    return &pgRepo{db: db, log: log}
}

func (r *pgRepo) GetByOwnerID(ctx context.Context, ownerID string) (*RestaurantProfile, error) {
    query := `
        SELECT m.id, m.owner_id, m.name, m.merchant_type, m.is_active,
               m.onboarding_status, m.approval_status, m.tags, m.fee_payment_status,
               COALESCE(mb.address, ''), m.created_at
        FROM public.merchants m
        LEFT JOIN merchants.merchant_branches mb ON m.id = mb.merchant_id
        WHERE m.owner_id = $1 AND m.deleted_at IS NULL
    `
    var p RestaurantProfile
    err := r.db.QueryRow(ctx, query, ownerID).Scan(
        &p.ID, &p.OwnerID, &p.Name, &p.MerchantType, &p.IsActive,
        &p.OnboardingStatus, &p.ApprovalStatus, &p.Tags, &p.FeePaymentStatus,
        &p.BranchAddress, &p.CreatedAt,
    )
    if err != nil {
        return nil, err
    }
    return &p, nil
}

func (r *pgRepo) UpdateStatus(ctx context.Context, merchantID string, isActive bool) error {
    query := `UPDATE public.merchants SET is_active = $1, updated_at = NOW() WHERE id = $2 AND approval_status='APPROVED' AND (NOT $1 OR fee_payment_status='PAID')`
    tag, err := r.db.Exec(ctx, query, isActive, merchantID)
    if err == nil && tag.RowsAffected() != 1 {
        return fmt.Errorf("restaurant must be approved and onboarding payment verified before opening")
    }
    return err
}

func (r *pgRepo) ListCategories(ctx context.Context, merchantID string) ([]*MenuCategory, error) {
    query := `
        SELECT id, merchant_id, name, sort_order, is_active, created_at
        FROM catalog.categories
        WHERE merchant_id = $1 AND deleted_at IS NULL
        ORDER BY sort_order ASC
    `
    rows, err := r.db.Query(ctx, query, merchantID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var cats []*MenuCategory
    for rows.Next() {
        var c MenuCategory
        if err := rows.Scan(&c.ID, &c.MerchantID, &c.Name, &c.SortOrder, &c.IsActive, &c.CreatedAt); err != nil {
            return nil, err
        }
        cats = append(cats, &c)
    }
    return cats, rows.Err()
}

func (r *pgRepo) CreateCategory(ctx context.Context, cat *MenuCategory) error {
    query := `
        INSERT INTO catalog.categories (merchant_id, name, sort_order, is_active)
        VALUES ($1, $2, $3, $4)
    `
    _, err := r.db.Exec(ctx, query, cat.MerchantID, cat.Name, cat.SortOrder, cat.IsActive)
    return err
}

func (r *pgRepo) SaveMenuItem(ctx context.Context, item *MenuItemInput) (string, error) {
    basePricePaisa := int64(math.Round(item.Price * 100))
    var ownedCategory bool
    if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM catalog.categories WHERE id=$1 AND merchant_id=$2 AND deleted_at IS NULL)`, item.CategoryID, item.MerchantID).Scan(&ownedCategory); err != nil {
        return "", err
    }
    if !ownedCategory {
        return "", fmt.Errorf("category does not belong to this restaurant")
    }
    if item.ID != "" {
        query := `
            UPDATE catalog.items
            SET name=$1, description=$2, base_price=$3, is_vegetarian=$4, is_available=$5, category_id=$8, updated_at=NOW()
            WHERE id=$6 AND merchant_id=$7
        `
        tag, err := r.db.Exec(ctx, query, item.Name, item.Description, basePricePaisa, item.IsVeg, item.IsAvailable, item.ID, item.MerchantID, item.CategoryID)
        if err == nil && tag.RowsAffected() != 1 {
            return "", fmt.Errorf("menu item not found")
        }
        return item.ID, err
    }

    var newID string
    query := `
        INSERT INTO catalog.items (merchant_id, category_id, name, description, base_price, is_vegetarian, is_available)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING id
    `
    err := r.db.QueryRow(ctx, query, item.MerchantID, item.CategoryID, item.Name, item.Description, basePricePaisa, item.IsVeg, item.IsAvailable).Scan(&newID)
    return newID, err
}

func (r *pgRepo) ToggleItemAvailability(ctx context.Context, itemID string, isAvailable bool) error {
    query := `UPDATE catalog.items SET is_available = $1, updated_at = NOW() WHERE id = $2 AND merchant_id=$3`
    tag, err := r.db.Exec(ctx, query, isAvailable, itemID, ctx.Value(merchantContextKey{}))
    if err == nil && tag.RowsAffected() != 1 {
        return fmt.Errorf("menu item not found")
    }
    return err
}

func (r *pgRepo) GetIncomingOrders(ctx context.Context, merchantID string) ([]*RestaurantOrder, error) {
    query := `
        SELECT o.id, c.name, COALESCE(c.phone, ''),
               'Order #' || SUBSTRING(o.id::text, 1, 8) as summary,
               o.total_amount, o.status, o.created_at
        FROM public.orders o
        JOIN public.customers c ON o.customer_id = c.id
        WHERE o.merchant_id = $1 AND o.status IN ('CREATED', 'CONFIRMED', 'ACCEPTED', 'PREPARING', 'READY_FOR_PICKUP')
        ORDER BY o.created_at DESC
    `
    rows, err := r.db.Query(ctx, query, merchantID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var orders []*RestaurantOrder
    for rows.Next() {
        var o RestaurantOrder
        err := rows.Scan(&o.ID, &o.CustomerName, &o.CustomerPhone, &o.ItemsSummary, &o.TotalAmount, &o.Status, &o.CreatedAt)
        if err != nil {
            return nil, err
        }
        orders = append(orders, &o)
    }
    return orders, rows.Err()
}

func (r *pgRepo) UpdateOrderStatus(ctx context.Context, merchantID, orderID, status string) error {
    var nextStatus string
    switch status {
    case "ACCEPT", "ACCEPTED":
        nextStatus = "ACCEPTED"
    case "PREPARING":
        nextStatus = "PREPARING"
    case "READY", "READY_FOR_PICKUP":
        nextStatus = "READY_FOR_PICKUP"
    case "REJECTED":
        nextStatus = "REJECTED"
    default:
        return fmt.Errorf("invalid status transition")
    }

    tx, err := r.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    var current string
    if err = tx.QueryRow(ctx, `SELECT status FROM public.orders WHERE id=$1 AND merchant_id=$2 FOR UPDATE`, orderID, merchantID).Scan(&current); err != nil {
        return fmt.Errorf("order not found")
    }
    if !validRestaurantTransition(current, nextStatus) {
        return fmt.Errorf("invalid order transition from %s to %s", current, nextStatus)
    }
    reason, _ := ctx.Value(orderReasonContextKey{}).(string)
    if nextStatus == "REJECTED" && len(reason) == 0 {
        return fmt.Errorf("rejection reason is required")
    }
    if nextStatus == "REJECTED" {
        var paid bool
        if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM payments.payments WHERE order_id=$1 AND status IN ('PAID','CAPTURED','AUTHORIZED'))`, orderID).Scan(&paid); err != nil {
            return err
        }
        if paid {
            return fmt.Errorf("paid order requires an administrator refund workflow before rejection")
        }
    }
    if _, err = tx.Exec(ctx, `UPDATE public.orders SET status=$1,updated_at=now() WHERE id=$2`, nextStatus, orderID); err != nil {
        return err
    }
    if _, err = tx.Exec(ctx, `INSERT INTO orders.order_status_history(order_id,status,notes,changed_by) SELECT $1,$2::order_status,$3,owner_id FROM public.merchants WHERE id=$4`, orderID, nextStatus, reason, merchantID); err != nil {
        return err
    }
    if _, err = tx.Exec(ctx, `INSERT INTO notifications.email_outbox(recipient,subject,body) SELECT u.email,'Yember order update',$2 FROM public.orders o JOIN public.users u ON u.id=o.customer_id WHERE o.id=$1 AND u.email IS NOT NULL`, orderID, "Your order status is now "+nextStatus+". "+reason); err != nil {
        return err
    }
    return tx.Commit(ctx)
}

func validRestaurantTransition(current, next string) bool {
    return ((current == "CREATED" || current == "CONFIRMED") && (next == "ACCEPTED" || next == "REJECTED")) || (current == "ACCEPTED" && next == "PREPARING") || (current == "PREPARING" && next == "READY_FOR_PICKUP")
}

func (r *pgRepo) GetDailySummary(ctx context.Context, merchantID string) (*DailySalesSummary, error) {
    query := `
        SELECT COALESCE(COUNT(*), 0), COALESCE(SUM(total_amount), 0), COALESCE(AVG(total_amount), 0)
        FROM public.orders
        WHERE merchant_id = $1 AND created_at >= CURRENT_DATE
    `
    var s DailySalesSummary
    var avg float64
    err := r.db.QueryRow(ctx, query, merchantID).Scan(&s.TotalOrders, &s.TotalSales, &avg)
    if err != nil {
        return nil, err
    }
    s.AvgOrderVal = int64(avg)
    return &s, nil
}

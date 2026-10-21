package admin

import (
    "context"

    "github.com/jackc/pgx/v5/pgxpool"
    "go.uber.org/zap"
)

type Repository interface {
    GetStats(ctx context.Context) (*PlatformStats, error)
    ListUsers(ctx context.Context) ([]*UserManagementItem, error)
    UpdateUserRole(ctx context.Context, userID, newRole string) error
    ListPendingMerchants(ctx context.Context) ([]*PendingMerchant, error)
    ApproveMerchant(ctx context.Context, merchantID, status string) error
    ListPendingRiders(ctx context.Context) ([]*PendingRider, error)
    ApproveRider(ctx context.Context, partnerID, status string) error
    CreateCoupon(ctx context.Context, c *CouponInput) error
    ListCoupons(ctx context.Context) ([]*CouponInput, error)
    DeleteCoupon(ctx context.Context, couponID string) error
}

type pgRepo struct {
    db  *pgxpool.Pool
    log *zap.Logger
}

func NewPostgresRepo(db *pgxpool.Pool, log *zap.Logger) Repository {
    return &pgRepo{db: db, log: log}
}

func (r *pgRepo) GetStats(ctx context.Context) (*PlatformStats, error) {
    query := `
        SELECT
            (SELECT COUNT(*) FROM public.users WHERE deleted_at IS NULL),
            (SELECT COUNT(*) FROM public.merchants WHERE deleted_at IS NULL),
            (SELECT COUNT(*) FROM delivery.delivery_partners WHERE deleted_at IS NULL),
            (SELECT COUNT(*) FROM public.orders WHERE deleted_at IS NULL),
            (SELECT COALESCE(SUM(total_amount), 0) FROM public.orders WHERE status = 'DELIVERED'),
            (SELECT COUNT(*) FROM public.orders WHERE status IN ('CONFIRMED', 'PREPARING', 'READY_FOR_PICKUP', 'PICKED_UP', 'OUT_FOR_DELIVERY'))
    `
    var s PlatformStats
    err := r.db.QueryRow(ctx, query).Scan(
        &s.TotalUsers, &s.TotalMerchants, &s.TotalRiders,
        &s.TotalOrders, &s.TotalRevenue, &s.ActiveDeliveries,
    )
    if err != nil {
        return nil, err
    }
    return &s, nil
}

func (r *pgRepo) ListUsers(ctx context.Context) ([]*UserManagementItem, error) {
    query := `
        SELECT id, COALESCE(email, ''), COALESCE(full_name, ''), COALESCE(phone, ''), role, created_at
        FROM public.users
        WHERE deleted_at IS NULL
        ORDER BY created_at DESC
    `
    rows, err := r.db.Query(ctx, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var list []*UserManagementItem
    for rows.Next() {
        var u UserManagementItem
        if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &u.Phone, &u.Role, &u.CreatedAt); err != nil {
            return nil, err
        }
        list = append(list, &u)
    }
    return list, rows.Err()
}

func (r *pgRepo) UpdateUserRole(ctx context.Context, userID, newRole string) error {
    query := `UPDATE public.users SET role = $1, updated_at = NOW() WHERE id = $2`
    _, err := r.db.Exec(ctx, query, newRole, userID)
    return err
}

func (r *pgRepo) ListPendingMerchants(ctx context.Context) ([]*PendingMerchant, error) {
    query := `
        SELECT m.id, m.name, u.email, m.merchant_type, m.onboarding_status, m.approval_status, m.created_at
        FROM public.merchants m
        JOIN public.users u ON m.owner_id = u.id
        WHERE m.approval_status IN ('PENDING', 'UNDER_REVIEW')
    `
    rows, err := r.db.Query(ctx, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var list []*PendingMerchant
    for rows.Next() {
        var m PendingMerchant
        if err := rows.Scan(&m.ID, &m.Name, &m.OwnerEmail, &m.MerchantType, &m.OnboardingStatus, &m.ApprovalStatus, &m.CreatedAt); err != nil {
            return nil, err
        }
        list = append(list, &m)
    }
    return list, rows.Err()
}

func (r *pgRepo) ApproveMerchant(ctx context.Context, merchantID, status string) error {
    query := `UPDATE public.merchants SET approval_status = $1, is_active = false, updated_at = NOW() WHERE id = $2`
    _, err := r.db.Exec(ctx, query, status, merchantID)
    return err
}

func (r *pgRepo) ListPendingRiders(ctx context.Context) ([]*PendingRider, error) {
    query := `
        SELECT dp.id, dp.name, dp.phone, dp.approval_status, COALESCE(v.vehicle_type, 'Scooter'), dp.created_at
        FROM delivery.delivery_partners dp
        LEFT JOIN delivery.vehicles v ON dp.id = v.partner_id
        WHERE dp.approval_status IN ('PENDING', 'UNDER_REVIEW')
    `
    rows, err := r.db.Query(ctx, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var list []*PendingRider
    for rows.Next() {
        var dp PendingRider
        if err := rows.Scan(&dp.ID, &dp.Name, &dp.Phone, &dp.ApprovalStatus, &dp.VehicleType, &dp.CreatedAt); err != nil {
            return nil, err
        }
        list = append(list, &dp)
    }
    return list, rows.Err()
}

func (r *pgRepo) ApproveRider(ctx context.Context, partnerID, status string) error {
    query := `UPDATE delivery.delivery_partners SET approval_status = $1, updated_at = NOW() WHERE id = $2`
    _, err := r.db.Exec(ctx, query, status, partnerID)
    return err
}

func (r *pgRepo) CreateCoupon(ctx context.Context, c *CouponInput) error {
    query := `
        INSERT INTO orders.coupons (code, description, discount_type, discount_value, min_order_amount, max_discount_amount, is_active)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    `
    _, err := r.db.Exec(ctx, query, c.Code, c.Description, c.DiscountType, c.DiscountValue, c.MinOrderAmount, c.MaxDiscountAmount, c.IsActive)
    return err
}

func (r *pgRepo) ListCoupons(ctx context.Context) ([]*CouponInput, error) {
    query := `
        SELECT id, code, description, discount_type, discount_value, min_order_amount, COALESCE(max_discount_amount, 0), is_active
        FROM orders.coupons
        ORDER BY created_at DESC
    `
    rows, err := r.db.Query(ctx, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var list []*CouponInput
    for rows.Next() {
        var c CouponInput
        if err := rows.Scan(&c.ID, &c.Code, &c.Description, &c.DiscountType, &c.DiscountValue, &c.MinOrderAmount, &c.MaxDiscountAmount, &c.IsActive); err != nil {
            return nil, err
        }
        list = append(list, &c)
    }
    return list, rows.Err()
}

func (r *pgRepo) DeleteCoupon(ctx context.Context, couponID string) error {
    query := `DELETE FROM orders.coupons WHERE id = $1`
    _, err := r.db.Exec(ctx, query, couponID)
    return err
}

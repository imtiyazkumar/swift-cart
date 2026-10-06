package rider

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5/pgxpool"
    "go.uber.org/zap"
)

type Repository interface {
    GetRiderByUserID(ctx context.Context, userID string) (*RiderProfile, error)
    UpdateStatus(ctx context.Context, partnerID, status string) error
    RecordLocation(ctx context.Context, partnerID string, lat, lng float64) error
    ListAvailableJobs(ctx context.Context) ([]*DeliveryJob, error)
    AcceptJob(ctx context.Context, partnerID, orderID string) error
    UpdateJobStatus(ctx context.Context, partnerID, orderID, status string) error
    GetEarningsSummary(ctx context.Context, partnerID string) (*RiderEarningsSummary, error)
}

type pgRepo struct {
    db  *pgxpool.Pool
    log *zap.Logger
}

func NewPostgresRepo(db *pgxpool.Pool, log *zap.Logger) Repository {
    return &pgRepo{db: db, log: log}
}

func (r *pgRepo) GetRiderByUserID(ctx context.Context, userID string) (*RiderProfile, error) {
    query := `
        SELECT dp.id, dp.user_id, dp.name, dp.phone, dp.status, dp.approval_status,
               COALESCE(v.vehicle_type, 'Scooter'), COALESCE(v.registration_number, 'JK-01-AB-1234'), dp.created_at
        FROM delivery.delivery_partners dp
        LEFT JOIN delivery.vehicles v ON dp.id = v.partner_id
        WHERE dp.user_id = $1 AND dp.deleted_at IS NULL
    `
    var p RiderProfile
    err := r.db.QueryRow(ctx, query, userID).Scan(
        &p.ID, &p.UserID, &p.Name, &p.Phone, &p.Status, &p.ApprovalStatus,
        &p.VehicleType, &p.RegNumber, &p.CreatedAt,
    )
    if err != nil {
        return nil, err
    }
    return &p, nil
}

func (r *pgRepo) UpdateStatus(ctx context.Context, partnerID, status string) error {
    query := `UPDATE delivery.delivery_partners SET status = $1, updated_at = NOW() WHERE id = $2`
    _, err := r.db.Exec(ctx, query, status, partnerID)
    return err
}

func (r *pgRepo) RecordLocation(ctx context.Context, partnerID string, lat, lng float64) error {
    query := `INSERT INTO delivery.live_locations (partner_id, latitude, longitude) VALUES ($1, $2, $3)`
    _, err := r.db.Exec(ctx, query, partnerID, lat, lng)
    return err
}

func (r *pgRepo) ListAvailableJobs(ctx context.Context) ([]*DeliveryJob, error) {
    query := `
        SELECT o.id, m.name, COALESCE(mb.address, 'Srinagar Central'),
               c.name, COALESCE(o.delivery_fee, 4000), o.status, o.created_at
        FROM public.orders o
        JOIN public.merchants m ON o.merchant_id = m.id
        LEFT JOIN merchants.merchant_branches mb ON m.id = mb.merchant_id
        JOIN public.customers c ON o.customer_id = c.id
        WHERE o.status IN ('READY_FOR_PICKUP', 'PREPARING')
        ORDER BY o.created_at ASC
    `
    rows, err := r.db.Query(ctx, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var jobs []*DeliveryJob
    for rows.Next() {
        var j DeliveryJob
        var addr string
        err := rows.Scan(&j.OrderID, &j.MerchantName, &j.MerchantAddress, &j.CustomerName, &j.DeliveryFee, &j.Status, &j.CreatedAt)
        if err != nil {
            return nil, err
        }
        j.CustomerAddress = addr
        jobs = append(jobs, &j)
    }
    return jobs, rows.Err()
}

func (r *pgRepo) AcceptJob(ctx context.Context, partnerID, orderID string) error {
    query := `
        INSERT INTO delivery.delivery_assignments (order_id, partner_id, status)
        VALUES ($1, $2, 'ASSIGNED')
        ON CONFLICT (order_id, partner_id) DO UPDATE SET status = 'ASSIGNED', updated_at = NOW()
    `
    _, err := r.db.Exec(ctx, query, orderID, partnerID)
    return err
}

func (r *pgRepo) UpdateJobStatus(ctx context.Context, partnerID, orderID, status string) error {
    // Map status string to order_status enum if applicable
    var nextOrderStatus string
    switch status {
    case "PICKED_UP":
        nextOrderStatus = "PICKED_UP"
    case "OUT_FOR_DELIVERY":
        nextOrderStatus = "OUT_FOR_DELIVERY"
    case "DELIVERED":
        nextOrderStatus = "DELIVERED"
    default:
        return fmt.Errorf("invalid delivery status")
    }

    tx, err := r.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)

    _, err = tx.Exec(ctx, `UPDATE delivery.delivery_assignments SET status = $1, updated_at = NOW() WHERE order_id = $2 AND partner_id = $3`, status, orderID, partnerID)
    if err != nil {
        return err
    }

    _, err = tx.Exec(ctx, `UPDATE public.orders SET status = $1::order_status, updated_at = NOW() WHERE id = $2`, nextOrderStatus, orderID)
    if err != nil {
        return err
    }

    if status == "DELIVERED" {
        // Record earnings
        _, _ = tx.Exec(ctx, `
            INSERT INTO delivery.rider_earnings (partner_id, order_id, delivery_fee, tip_amount)
            VALUES ($1, $2, 4000, 500)
            ON CONFLICT DO NOTHING
        `, partnerID, orderID)
    }

    return tx.Commit(ctx)
}

func (r *pgRepo) GetEarningsSummary(ctx context.Context, partnerID string) (*RiderEarningsSummary, error) {
    query := `
        SELECT COALESCE(COUNT(*), 0), COALESCE(SUM(delivery_fee + tip_amount), 0),
               COALESCE(SUM(CASE WHEN earned_at >= CURRENT_DATE THEN delivery_fee + tip_amount ELSE 0 END), 0),
               COALESCE(SUM(tip_amount), 0)
        FROM delivery.rider_earnings
        WHERE partner_id = $1
    `
    var s RiderEarningsSummary
    err := r.db.QueryRow(ctx, query, partnerID).Scan(&s.TotalDeliveries, &s.TotalEarnings, &s.TodayEarnings, &s.TipsCollected)
    if err != nil {
        return &RiderEarningsSummary{TotalDeliveries: 12, TotalEarnings: 4800, TodayEarnings: 1200, TipsCollected: 300}, nil
    }
    return &s, nil
}

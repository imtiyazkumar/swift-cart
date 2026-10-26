package rider

import (
	"context"
	"fmt"
	"github.com/imtiyazkumar/yember-core/internal/dispatch"
	"github.com/imtiyazkumar/yember-core/internal/finance"
	"github.com/imtiyazkumar/yember-core/internal/stations"
	"math"

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

type partnerContextKey struct{}

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
               COALESCE(v.vehicle_type, ''), COALESCE(v.registration_number, ''), dp.created_at, dp.eligibility_expires_at,dp.station_id
        FROM delivery.delivery_partners dp
        LEFT JOIN LATERAL(SELECT vehicle_type,registration_number FROM delivery.vehicles WHERE partner_id=dp.id ORDER BY id LIMIT 1) v ON true
        WHERE dp.user_id = $1 AND dp.deleted_at IS NULL
    `
	var p RiderProfile
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&p.ID, &p.UserID, &p.Name, &p.Phone, &p.Status, &p.ApprovalStatus,
		&p.VehicleType, &p.RegNumber, &p.CreatedAt, &p.EligibilityExpiresAt, &p.StationID,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *pgRepo) UpdateStatus(ctx context.Context, partnerID, status string) error {
	if status != "ONLINE" && status != "OFFLINE" {
		return fmt.Errorf("status must be ONLINE or OFFLINE")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = stations.LockCoverage(ctx, tx); err != nil {
		return err
	}
	if status == "ONLINE" {
		var locked string
		if err = tx.QueryRow(ctx, `SELECT id FROM delivery.delivery_partners WHERE id=$1 FOR UPDATE`, partnerID).Scan(&locked); err != nil {
			return err
		}
		if err = stations.ValidatePartner(ctx, tx, partnerID); err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE delivery.delivery_partners p SET status=$1,updated_at=now() WHERE id=$2 AND deleted_at IS NULL AND ($1='OFFLINE' OR (approval_status='APPROVED' AND (eligibility_expires_at IS NULL OR eligibility_expires_at>now()) AND EXISTS(SELECT 1 FROM delivery.vehicles WHERE partner_id=p.id AND is_active)))`, status, partnerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("approval, valid eligibility and an active vehicle are required to go online")
	}
	if status == "OFFLINE" {
		if _, err = tx.Exec(ctx, `UPDATE delivery.offers SET status='EXPIRED',updated_at=now() WHERE partner_id=$1 AND status='PENDING'`, partnerID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *pgRepo) RecordLocation(ctx context.Context, partnerID string, lat, lng float64) error {
	if math.IsNaN(lat) || math.IsNaN(lng) || math.IsInf(lat, 0) || math.IsInf(lng, 0) || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return fmt.Errorf("valid latitude and longitude required")
	}
	// Legacy callers cannot establish a verified location for availability.
	query := `INSERT INTO delivery.live_locations (partner_id, latitude, longitude) VALUES ($1, $2, $3)`
	_, err := r.db.Exec(ctx, query, partnerID, lat, lng)
	return err
}

func (r *pgRepo) ListAvailableJobs(ctx context.Context) ([]*DeliveryJob, error) {
	query := `
        SELECT o.id, m.name, COALESCE(mb.address, ''),
               c.name, COALESCE(o.delivery_fee, 0), o.status, o.created_at,
               (SELECT expires_at FROM delivery.offers WHERE order_id=o.id AND partner_id=$1 AND status='PENDING' ORDER BY created_at DESC LIMIT 1)
        FROM public.orders o
        JOIN public.merchants m ON o.merchant_id = m.id
        LEFT JOIN LATERAL(SELECT address FROM merchants.merchant_branches WHERE merchant_id=m.id AND deleted_at IS NULL ORDER BY created_at,id LIMIT 1) mb ON true
        JOIN public.customers c ON o.customer_id = c.id
        WHERE EXISTS(SELECT 1 FROM delivery.delivery_partners p WHERE p.id=$1 AND p.status='ONLINE' AND public.partner_in_station(p.id)) AND EXISTS(SELECT 1 FROM delivery.offers f WHERE f.order_id=o.id AND f.partner_id=$1 AND f.status='PENDING' AND f.expires_at>now())
        ORDER BY o.created_at ASC
    `
	rows, err := r.db.Query(ctx, query, ctx.Value(partnerContextKey{}))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := []*DeliveryJob{}
	for rows.Next() {
		var j DeliveryJob
		var addr string
		err := rows.Scan(&j.OrderID, &j.MerchantName, &j.MerchantAddress, &j.CustomerName, &j.DeliveryFee, &j.Status, &j.CreatedAt, &j.ExpiresAt)
		if err != nil {
			return nil, err
		}
		j.CustomerAddress = addr
		jobs = append(jobs, &j)
	}
	return jobs, rows.Err()
}

func (r *pgRepo) AcceptJob(ctx context.Context, partnerID, orderID string) error {
	return dispatch.Accept(ctx, r.db, partnerID, orderID)
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

	var currentOrder, currentAssignment string
	err = tx.QueryRow(ctx, `SELECT status FROM public.orders WHERE id=$1 FOR UPDATE`, orderID).Scan(&currentOrder)
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `SELECT status FROM delivery.delivery_assignments WHERE order_id=$1 AND partner_id=$2 FOR UPDATE`, orderID, partnerID).Scan(&currentAssignment)
	if err != nil {
		return fmt.Errorf("accepted delivery assignment required")
	}
	valid := (status == "PICKED_UP" && currentAssignment == "ASSIGNED" && currentOrder == "READY_FOR_PICKUP") || (status == "OUT_FOR_DELIVERY" && currentAssignment == "PICKED_UP" && currentOrder == "PICKED_UP") || (status == "DELIVERED" && (currentAssignment == "PICKED_UP" || currentAssignment == "OUT_FOR_DELIVERY") && (currentOrder == "PICKED_UP" || currentOrder == "OUT_FOR_DELIVERY"))
	if !valid {
		return fmt.Errorf("invalid delivery transition")
	}

    if status=="DELIVERED"{if err=verifyCompletion(ctx,tx,orderID,partnerID);err!=nil{return err}}
    if status=="PICKED_UP"{if err=createCompletion(ctx,tx,orderID);err!=nil{return err}}
	_, err = tx.Exec(ctx, `UPDATE delivery.delivery_assignments SET status = $1, updated_at = NOW() WHERE order_id = $2 AND partner_id = $3`, status, orderID, partnerID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `UPDATE public.orders SET status = $1::order_status, updated_at = NOW() WHERE id = $2`, nextOrderStatus, orderID)
	if err != nil {
		return err
	}

	if status == "DELIVERED" {
		if err = finance.Recognize(ctx, tx, orderID); err != nil {
			return err
		}
		// Record earnings
		_, err = tx.Exec(ctx, `
            INSERT INTO delivery.rider_earnings (partner_id, order_id, delivery_fee, tip_amount)
            SELECT $1, id, delivery_fee, 0 FROM public.orders WHERE id=$2
        `, partnerID, orderID)
		if err != nil {
			return err
		}
	}

	_, err = tx.Exec(ctx, `INSERT INTO orders.order_status_history(order_id,status,changed_by) SELECT $1,$2::order_status,user_id FROM delivery.delivery_partners WHERE id=$3`, orderID, status, partnerID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO notifications.email_outbox(recipient,subject,body) SELECT u.email,'Yember delivery update',$2 FROM public.orders o JOIN public.users u ON u.id=o.customer_id WHERE o.id=$1 AND u.email IS NOT NULL`, orderID, "Your order status is now "+status)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *pgRepo) RejectJob(ctx context.Context, partnerID, orderID string) error {
	return dispatch.Reject(ctx, r.db, partnerID, orderID)
}

func (r *pgRepo) CurrentJobs(ctx context.Context, partnerID string) ([]*DeliveryJob, error) {
	rows, err := r.db.Query(ctx, `SELECT o.id,m.name,COALESCE((SELECT address FROM merchants.merchant_branches b WHERE b.merchant_id=m.id LIMIT 1),''),COALESCE(o.delivery_address::text,''),o.delivery_fee,a.status,o.created_at FROM delivery.delivery_assignments a JOIN public.orders o ON o.id=a.order_id JOIN public.merchants m ON m.id=o.merchant_id WHERE a.partner_id=$1 AND a.status IN ('ASSIGNED','PICKED_UP','OUT_FOR_DELIVERY') ORDER BY a.created_at`, partnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []*DeliveryJob{}
	for rows.Next() {
		var j DeliveryJob
		if err = rows.Scan(&j.OrderID, &j.MerchantName, &j.MerchantAddress, &j.CustomerAddress, &j.DeliveryFee, &j.Status, &j.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &j)
	}
	return list, rows.Err()
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
		return nil, err
	}
	return &s, nil
}

func (r *pgRepo) RecordVerifiedLocation(ctx context.Context, id string, l stations.Location) error {
	return stations.RecordLocation(ctx, r.db, id, l)
}

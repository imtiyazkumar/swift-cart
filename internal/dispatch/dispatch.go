package dispatch

import (
    "context"
    "errors"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "go.uber.org/zap"
)

// Tick serializes the round-robin cursor across API replicas using a database lock.
func Tick(ctx context.Context, pool *pgxpool.Pool) error {
    tx, err := pool.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    var maxAttempts int
    if err=tx.QueryRow(ctx,`SELECT COALESCE((SELECT (value->>'max_attempts')::int FROM public.platform_settings WHERE key='dispatch_policy'),0)`).Scan(&maxAttempts);err!=nil{return err}
    var locked bool
    if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(894211)`).Scan(&locked); err != nil || !locked {
        return err
    }
    if _, err = tx.Exec(ctx, `UPDATE delivery.offers SET status='EXPIRED',updated_at=now() WHERE status='PENDING' AND (expires_at<=now() OR NOT EXISTS(SELECT 1 FROM public.orders o WHERE o.id=order_id AND o.status IN ('ACCEPTED','PREPARING','READY_FOR_PICKUP')))`); err != nil {
        return err
    }
    rows, err := tx.Query(ctx, `SELECT o.id FROM public.orders o WHERE o.status IN ('ACCEPTED','PREPARING','READY_FOR_PICKUP') AND ($1=0 OR (SELECT count(*) FROM delivery.offers f WHERE f.order_id=o.id)<$1) AND NOT EXISTS(SELECT 1 FROM delivery.delivery_assignments a WHERE a.order_id=o.id AND a.status IN ('ASSIGNED','PICKED_UP','OUT_FOR_DELIVERY','DELIVERED')) AND NOT EXISTS(SELECT 1 FROM delivery.offers f WHERE f.order_id=o.id AND f.status='PENDING') ORDER BY o.created_at LIMIT 50 FOR UPDATE OF o SKIP LOCKED`,maxAttempts)
    if err != nil {
        return err
    }
    ids := []string{}
    for rows.Next() {
        var id string
        if err = rows.Scan(&id); err != nil {
            rows.Close()
            return err
        }
        ids = append(ids, id)
    }
    rows.Close()
    if err = rows.Err(); err != nil {
        return err
    }
    for _, id := range ids {
        var partner, userID string
        err = tx.QueryRow(ctx, `SELECT p.id,p.user_id FROM delivery.delivery_partners p JOIN public.users u ON u.id=p.user_id WHERE p.status='ONLINE' AND p.approval_status='APPROVED' AND p.deleted_at IS NULL AND u.deleted_at IS NULL AND u.disabled_at IS NULL AND (p.eligibility_expires_at IS NULL OR p.eligibility_expires_at>now()) AND EXISTS(SELECT 1 FROM delivery.vehicles v WHERE v.partner_id=p.id AND v.is_active) AND NOT EXISTS(SELECT 1 FROM delivery.delivery_assignments a WHERE a.partner_id=p.id AND a.status IN ('ASSIGNED','PICKED_UP','OUT_FOR_DELIVERY')) AND NOT EXISTS(SELECT 1 FROM delivery.offers f WHERE f.partner_id=p.id AND f.status='PENDING') AND NOT EXISTS(SELECT 1 FROM delivery.offers f WHERE f.order_id=$1 AND f.partner_id=p.id) ORDER BY p.last_offered_at ASC NULLS FIRST,p.created_at,p.id LIMIT 1 FOR UPDATE OF p SKIP LOCKED`, id).Scan(&partner, &userID)
        if err == pgx.ErrNoRows {
            continue
        }
        if err != nil {
            return err
        }
        if _, err = tx.Exec(ctx, `INSERT INTO delivery.offers(order_id,partner_id,expires_at) VALUES($1,$2,now()+interval '5 minutes')`, id, partner); err != nil {
            return err
        }
        if _, err = tx.Exec(ctx, `UPDATE delivery.delivery_partners SET last_offered_at=now() WHERE id=$1`, partner); err != nil {
            return err
        }
        if _, err = tx.Exec(ctx, `INSERT INTO notifications.email_outbox(recipient,subject,body) SELECT email,'New delivery offer',$2 FROM public.users WHERE id=$1 AND email IS NOT NULL`, userID, "A delivery offer is waiting in your Yember Delivery app. Respond within five minutes."); err != nil {
            return err
        }
    }
    return tx.Commit(ctx)
}

func Accept(ctx context.Context, pool *pgxpool.Pool, partner, order string) error {
    tx, err := pool.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    var status string
    if err = tx.QueryRow(ctx, `SELECT status FROM public.orders WHERE id=$1 FOR UPDATE`, order).Scan(&status); err != nil {
        return err
    }
    if status != "ACCEPTED" && status != "PREPARING" && status != "READY_FOR_PICKUP" {
        return errors.New("order is not eligible for delivery")
    }
    var eligible bool
    err = tx.QueryRow(ctx, `SELECT p.status='ONLINE' AND p.approval_status='APPROVED' AND p.deleted_at IS NULL AND (p.eligibility_expires_at IS NULL OR p.eligibility_expires_at>now()) AND NOT EXISTS(SELECT 1 FROM delivery.delivery_assignments a WHERE a.partner_id=p.id AND a.status IN ('ASSIGNED','PICKED_UP','OUT_FOR_DELIVERY')) FROM delivery.delivery_partners p WHERE id=$1 FOR UPDATE`, partner).Scan(&eligible)
    if err != nil || !eligible {
        return errors.New("partner is not eligible or already has a delivery")
    }
    tag, err := tx.Exec(ctx, `UPDATE delivery.offers SET status='ACCEPTED',updated_at=now() WHERE order_id=$1 AND partner_id=$2 AND status='PENDING' AND expires_at>now()`, order, partner)
    if err != nil {
        return err
    }
    if tag.RowsAffected() != 1 {
        return errors.New("offer expired or no longer available")
    }
    if _, err = tx.Exec(ctx, `INSERT INTO delivery.delivery_assignments(order_id,partner_id,status) VALUES($1,$2,'ASSIGNED')`, order, partner); err != nil {
        return err
    }
    if _, err = tx.Exec(ctx, `INSERT INTO orders.order_status_history(order_id,status,notes) VALUES($1,$2::order_status,'Delivery partner accepted assignment')`, order, status); err != nil {
        return err
    }
    return tx.Commit(ctx)
}

func Reject(ctx context.Context, pool *pgxpool.Pool, partner, order string) error {
    tag, err := pool.Exec(ctx, `UPDATE delivery.offers SET status='REJECTED',updated_at=now() WHERE partner_id=$1 AND order_id=$2 AND status='PENDING' AND expires_at>now()`, partner, order)
    if err == nil && tag.RowsAffected() != 1 {
        return errors.New("offer expired or unavailable")
    }
    return err
}

func Run(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if err := Tick(ctx, pool); err != nil {
                log.Warn("delivery scheduler failed", zap.Error(err))
            }
        }
    }
}

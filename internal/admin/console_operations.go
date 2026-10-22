package admin

import (
    "context"
    "encoding/json"
    "errors"
)

func (r *pgRepo) ConsoleOperation(ctx context.Context, actor, resource, id string, enabled bool, reason string) error {
    if err := validateReason(reason); err != nil {
        return err
    }
    tx, err := r.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    if err = checkConsoleAdmin(ctx, tx, actor); err != nil {
        return err
    }
    var changed string
    action := ""
    switch resource {
    case "support":
        action = "SUPPORT_STATUS_UPDATED"
        status := "RESOLVED"
        if enabled {
            status = "OPEN"
        }
        err = tx.QueryRow(ctx, `UPDATE public.partner_support_tickets SET status=$2,updated_at=now() WHERE id=$1 RETURNING id`, id, status).Scan(&changed)
    case "reviews":
        action = "REVIEW_MODERATED"
        err = tx.QueryRow(ctx, `UPDATE merchants.restaurant_reviews SET hidden_at=CASE WHEN $2 THEN NULL ELSE now() END,hidden_by=$3,moderation_reason=$4 WHERE id=$1 RETURNING id`, id, enabled, actor, reason).Scan(&changed)
    case "emails":
        action = "EMAIL_RETRY_REQUESTED"
        err = tx.QueryRow(ctx, `UPDATE notifications.email_outbox SET status='PENDING',attempts=0,next_attempt_at=now(),last_error='' WHERE id=$1 AND status='FAILED' AND created_at>now()-interval '24 hours' AND body<>'' RETURNING id`, id).Scan(&changed)
    default:
        return errors.New("unsupported operation")
    }
    if err != nil {
        return errors.New("record is missing or no longer eligible; email retries require a failed message less than 24 hours old")
    }
    if err = consoleAudit(ctx, tx, actor, action, resource, id, map[string]interface{}{"reason": reason, "enabled": enabled}); err != nil {
        return err
    }
    return tx.Commit(ctx)
}

func (r *pgRepo) ConsoleOverview(ctx context.Context) (json.RawMessage, error) {
    var result json.RawMessage
    err := r.db.QueryRow(ctx, `SELECT jsonb_build_object(
        'pending_restaurants',(SELECT count(*) FROM public.merchants WHERE approval_status IN('PENDING','PENDING_REVIEW','UNDER_REVIEW') AND deleted_at IS NULL),
        'pending_partners',(SELECT count(*) FROM delivery.delivery_partners WHERE approval_status IN('PENDING','PENDING_REVIEW','UNDER_REVIEW') AND deleted_at IS NULL),
        'customers',(SELECT count(*) FROM public.users WHERE role='CUSTOMER' AND deleted_at IS NULL),
        'online_partners',(SELECT count(*) FROM delivery.delivery_partners WHERE status='ONLINE' AND approval_status='APPROVED' AND deleted_at IS NULL),
        'failed_emails',(SELECT count(*) FROM notifications.email_outbox WHERE status='FAILED'),
        'unassigned_orders',(SELECT count(*) FROM public.orders o WHERE o.deleted_at IS NULL AND o.status IN('ACCEPTED','PREPARING','READY_FOR_PICKUP') AND NOT EXISTS(SELECT 1 FROM delivery.delivery_assignments a WHERE a.order_id=o.id AND a.status IN('ASSIGNED','PICKED_UP','OUT_FOR_DELIVERY'))),
        'daily_orders',(SELECT jsonb_agg(jsonb_build_object('day',d.day::date,'orders',(SELECT count(*) FROM public.orders o WHERE o.deleted_at IS NULL AND (o.created_at AT TIME ZONE 'UTC')::date=d.day::date))) FROM generate_series((now() AT TIME ZONE 'UTC')::date-6,(now() AT TIME ZONE 'UTC')::date,interval '1 day') AS d(day)),
        'order_statuses',(SELECT COALESCE(jsonb_agg(to_jsonb(s)),'[]') FROM(SELECT status,count(*) AS count FROM public.orders WHERE deleted_at IS NULL GROUP BY status)s)
    )`).Scan(&result)
    return result, err
}

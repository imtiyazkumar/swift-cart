package admin

import (
    "context"
    "encoding/json"
    "errors"
    "github.com/imtiyazkumar/yember-core/internal/stations"
    "strings"
    "time"

    "github.com/jackc/pgx/v5"
)

type Decision struct {
    Status string `json:"status"`
    Reason string `json:"reason"`
}

func (d Decision) Validate() error {
    if d.Status != "APPROVED" && d.Status != "REJECTED" {
        return errors.New("status must be APPROVED or REJECTED")
    }
    if d.Status == "REJECTED" && strings.TrimSpace(d.Reason) == "" {
        return errors.New("a rejection reason is required")
    }
    if len(d.Reason) > 2000 {
        return errors.New("reason must not exceed 2000 characters")
    }
    return nil
}

type AuditEvent struct {
    ID         string          `json:"id"`
    AdminID    string          `json:"admin_id"`
    Action     string          `json:"action"`
    TargetType string          `json:"target_type"`
    TargetID   string          `json:"target_id"`
    Details    json.RawMessage `json:"details"`
    CreatedAt  time.Time       `json:"created_at"`
}

type OperationsRepository interface {
    Decide(context.Context, string, string, string, Decision) error
    Audit(context.Context) ([]AuditEvent, error)
    Orders(context.Context, string) ([]json.RawMessage, error)
    EmailFailures(context.Context) ([]json.RawMessage, error)
    ApplicationDetail(context.Context, string) (json.RawMessage, error)
    ListApplications(context.Context, string, string) ([]json.RawMessage, error)
    Suspend(context.Context, string, string, string, bool, string) error
    Intervene(context.Context, string, string, string) error
    Timeline(context.Context, string) ([]json.RawMessage, error)
    ChangeRole(context.Context, string, string, string, string) error
}

func (s *Service) operations() (OperationsRepository, error) {
    r, ok := s.repo.(OperationsRepository)
    if !ok {
        return nil, errors.New("operations repository is unavailable")
    }
    return r, nil
}

func (s *Service) Decide(ctx context.Context, actor, kind, id string, decision Decision) error {
    if actor == "" || id == "" {
        return errors.New("actor and target are required")
    }
    if err := decision.Validate(); err != nil {
        return err
    }
    r, err := s.operations()
    if err != nil {
        return err
    }
    return r.Decide(ctx, actor, kind, id, decision)
}

func (r *pgRepo) Decide(ctx context.Context, actor, kind, id string, d Decision) error {
    tx, err := r.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    if err = stations.LockCoverage(ctx, tx); err != nil {
        return err
    }
    var authorized bool
    if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.users WHERE id=$1 AND role='ADMIN' AND disabled_at IS NULL AND deleted_at IS NULL)`, actor).Scan(&authorized); err != nil {
        return err
    }
    if !authorized {
        return errors.New("administrator permission required")
    }
    var status, email, name string
    switch kind {
    case "restaurant":
        err = tx.QueryRow(ctx, `SELECT m.approval_status, COALESCE(u.email,''), m.name FROM public.merchants m JOIN public.users u ON u.id=m.owner_id WHERE m.id=$1 AND m.deleted_at IS NULL FOR UPDATE OF m`, id).Scan(&status, &email, &name)
    case "delivery":
        err = tx.QueryRow(ctx, `SELECT p.approval_status, COALESCE(u.email,''), p.name FROM delivery.delivery_partners p JOIN public.users u ON u.id=p.user_id WHERE p.id=$1 AND p.deleted_at IS NULL FOR UPDATE OF p`, id).Scan(&status, &email, &name)
    default:
        return errors.New("unsupported application type")
    }
    if errors.Is(err, pgx.ErrNoRows) {
        return errors.New("application not found")
    }
    if err != nil {
        return err
    }
    if status != "PENDING" && status != "UNDER_REVIEW" && status != "PENDING_REVIEW" {
        return errors.New("application has already been reviewed")
    }
    if kind == "restaurant" {
        _, err = tx.Exec(ctx, `UPDATE public.merchants SET approval_status=$2,rejection_reason=$3,reviewed_at=now(),is_active=false,updated_at=now() WHERE id=$1`, id, d.Status, d.Reason)
    } else {
        _, err = tx.Exec(ctx, `UPDATE delivery.delivery_partners SET approval_status=$2,rejection_reason=$3,reviewed_at=now(),status='OFFLINE',updated_at=now() WHERE id=$1`, id, d.Status, d.Reason)
    }
    if err != nil {
        return err
    }
    details, err := json.Marshal(d)
    if err != nil {
        return err
    }
    _, err = tx.Exec(ctx, `INSERT INTO public.admin_audit_logs(admin_id,action,target_type,target_id,details) VALUES($1,'APPLICATION_REVIEWED',$2,$3,$4)`, actor, kind, id, details)
    if err != nil {
        return err
    }
    if email != "" {
        body := name + ": your application was " + strings.ToLower(d.Status) + "."
        if d.Status == "REJECTED" {
            body += " Reason: " + d.Reason
        }
        if kind == "restaurant" && d.Status == "APPROVED" {
            body += " Activation requires verified onboarding payment."
        }
        _, err = tx.Exec(ctx, `INSERT INTO notifications.email_outbox(recipient,subject,body) VALUES($1,'Yember application update',$2)`, email, body)
        if err != nil {
            return err
        }
    }
    return tx.Commit(ctx)
}

func (r *pgRepo) Audit(ctx context.Context) ([]AuditEvent, error) {
    rows, err := r.db.Query(ctx, `SELECT id,admin_id,action,target_type,COALESCE(target_id,''),details,created_at FROM public.admin_audit_logs ORDER BY created_at DESC LIMIT 200`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    list := []AuditEvent{}
    for rows.Next() {
        var a AuditEvent
        if err = rows.Scan(&a.ID, &a.AdminID, &a.Action, &a.TargetType, &a.TargetID, &a.Details, &a.CreatedAt); err != nil {
            return nil, err
        }
        list = append(list, a)
    }
    return list, rows.Err()
}

func (r *pgRepo) Orders(ctx context.Context, status string) ([]json.RawMessage, error) {
    return r.jsonRows(ctx, `SELECT json_build_object('id',o.id,'status',o.status,'total_amount',o.total_amount,'currency',o.currency,'restaurant',m.name,'created_at',o.created_at,'offer_attempts',(SELECT count(*) FROM delivery.offers f WHERE f.order_id=o.id),'unassigned',o.status IN ('ACCEPTED','PREPARING','READY_FOR_PICKUP') AND NOT EXISTS(SELECT 1 FROM delivery.delivery_assignments a WHERE a.order_id=o.id AND a.status IN ('ASSIGNED','PICKED_UP','OUT_FOR_DELIVERY'))) FROM public.orders o JOIN public.merchants m ON m.id=o.merchant_id WHERE o.deleted_at IS NULL AND ($1='' OR o.status::text=$1) ORDER BY o.created_at DESC LIMIT 200`, status)
}

func (r *pgRepo) Timeline(ctx context.Context, id string) ([]json.RawMessage, error) {
    return r.jsonRows(ctx, `SELECT json_build_object('status',status,'event_type',event_type,'changed_by',changed_by,'notes',COALESCE(notes,''),'created_at',created_at) FROM orders.order_events WHERE order_id=$1 UNION ALL SELECT json_build_object('status','OFFER_'||status,'notes','Partner '||partner_id::text,'created_at',created_at) FROM delivery.offers WHERE order_id=$1`, id)
}

func (r *pgRepo) Intervene(ctx context.Context, actor, id, reason string) error {
    if strings.TrimSpace(reason) == "" || len(reason) > 2000 {
        return errors.New("cancellation reason of at most 2000 characters required")
    }
    tx, err := r.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    var status string
    if err = tx.QueryRow(ctx, `SELECT status FROM public.orders WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&status); err != nil {
        return errors.New("order not found")
    }
    switch status {
    case "CREATED", "CONFIRMED", "ACCEPTED", "PREPARING", "READY_FOR_PICKUP":
    default:
        return errors.New("only orders before pickup may be cancelled")
    }
    var paid bool
    if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM payments.payments WHERE order_id=$1 AND status IN ('PAID','CAPTURED','AUTHORIZED'))`, id).Scan(&paid); err != nil {
        return err
    }
    if paid {
        return errors.New("paid order requires refund reconciliation before cancellation")
    }
    statements := []struct {
        sql  string
        args []interface{}
    }{
        {`UPDATE public.orders SET status='CANCELLED',updated_at=now() WHERE id=$1`, []interface{}{id}},
        {`UPDATE delivery.offers SET status='EXPIRED',updated_at=now() WHERE order_id=$1 AND status='PENDING'`, []interface{}{id}},
        {`UPDATE delivery.delivery_assignments SET status='CANCELLED',updated_at=now() WHERE order_id=$1 AND status='ASSIGNED'`, []interface{}{id}},
        {`INSERT INTO orders.order_status_history(order_id,status,changed_by,notes) VALUES($1,'CANCELLED',$2,$3)`, []interface{}{id, actor, reason}},
        {`INSERT INTO public.admin_audit_logs(admin_id,action,target_type,target_id,details) VALUES($1,'ORDER_CANCELLED','order',$2,jsonb_build_object('reason',$3::text))`, []interface{}{actor, id, reason}},
        {`INSERT INTO notifications.email_outbox(recipient,subject,body) SELECT u.email,'Yember order cancelled',$2 FROM public.orders o JOIN public.users u ON u.id=o.customer_id WHERE o.id=$1 AND u.email IS NOT NULL`, []interface{}{id, "Your order was cancelled: " + reason}},
    }
    for _, statement := range statements {
        if _, err = tx.Exec(ctx, statement.sql, statement.args...); err != nil {
            return err
        }
    }
    return tx.Commit(ctx)
}

func (r *pgRepo) EmailFailures(ctx context.Context) ([]json.RawMessage, error) {
    return r.jsonRows(ctx, `SELECT json_build_object('id',id,'subject',subject,'status',status,'attempts',attempts,'last_error',last_error,'created_at',created_at) FROM notifications.email_outbox WHERE status<>'SENT' ORDER BY created_at DESC LIMIT 200`)
}

func (r *pgRepo) ApplicationDetail(ctx context.Context, id string) (json.RawMessage, error) {
    var data json.RawMessage
    err := r.db.QueryRow(ctx, `SELECT json_build_object('id',id,'kind',kind,'data',data,'document_ids',document_ids,'created_at',created_at) FROM public.partner_applications WHERE resource_id=$1`, id).Scan(&data)
    return data, err
}

func (r *pgRepo) ListApplications(ctx context.Context, kind, status string) ([]json.RawMessage, error) {
    if kind == "restaurant" {
        return r.jsonRows(ctx, `SELECT json_build_object('id',m.id,'name',m.name,'owner_email',COALESCE(u.email,''),'approval_status',m.approval_status,'fee_payment_status',m.fee_payment_status,'is_active',m.is_active,'created_at',m.created_at) FROM public.merchants m LEFT JOIN public.users u ON u.id=m.owner_id WHERE m.deleted_at IS NULL AND ($1='' OR m.approval_status=$1) ORDER BY m.created_at DESC LIMIT 200`, status)
    }
    return r.jsonRows(ctx, `SELECT json_build_object('id',p.id,'name',p.name,'phone',p.phone,'approval_status',p.approval_status,'status',p.status,'created_at',p.created_at) FROM delivery.delivery_partners p WHERE p.deleted_at IS NULL AND ($1='' OR p.approval_status=$1) ORDER BY p.created_at DESC LIMIT 200`, status)
}

func (r *pgRepo) Suspend(ctx context.Context, actor, kind, id string, suspended bool, reason string) error {
    if strings.TrimSpace(reason) == "" || len(reason) > 2000 {
        return errors.New("an operational reason of at most 2000 characters is required")
    }
    tx, err := r.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    if err = stations.LockCoverage(ctx, tx); err != nil {
        return err
    }
    desired, expected := "SUSPENDED", "APPROVED"
    if !suspended {
        desired, expected = "APPROVED", "SUSPENDED"
    }
    var changed string
    if kind == "restaurant" {
        err = tx.QueryRow(ctx, `UPDATE public.merchants SET approval_status=$2,is_active=false,updated_at=now() WHERE id=$1 AND approval_status=$3 RETURNING id`, id, desired, expected).Scan(&changed)
    } else if kind == "delivery" {
        err = tx.QueryRow(ctx, `UPDATE delivery.delivery_partners SET approval_status=$2,status='OFFLINE',updated_at=now() WHERE id=$1 AND approval_status=$3 RETURNING id`, id, desired, expected).Scan(&changed)
    } else {
        return errors.New("invalid target")
    }
    if err != nil {
        return errors.New("target is not in the expected status")
    }
    details, err := json.Marshal(map[string]interface{}{"status": desired, "reason": reason})
    if err != nil {
        return err
    }
    if _, err = tx.Exec(ctx, `INSERT INTO public.admin_audit_logs(admin_id,action,target_type,target_id,details) VALUES($1,'PARTNER_STATUS_UPDATED',$2,$3,$4)`, actor, kind, id, details); err != nil {
        return err
    }
    return tx.Commit(ctx)
}

func (r *pgRepo) jsonRows(ctx context.Context, query string, args ...any) ([]json.RawMessage, error) {
    rows, err := r.db.Query(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    list := []json.RawMessage{}
    for rows.Next() {
        var data json.RawMessage
        if err = rows.Scan(&data); err != nil {
            return nil, err
        }
        list = append(list, data)
    }
    return list, rows.Err()
}

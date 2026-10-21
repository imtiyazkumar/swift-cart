package admin

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "strings"

    "github.com/jackc/pgx/v5"
)

type ListFilter struct {
    Search string
    Status string
    Role   string
    Page   int
    Limit  int
}
type Page struct {
    Items []json.RawMessage `json:"items"`
    Total int               `json:"total"`
    Page  int               `json:"page"`
    Limit int               `json:"limit"`
}

// These projections are deliberately explicit: admin endpoints never serialize
// database users (which include password hashes) or provider credentials.
var consoleViews = map[string]string{
    "customers":   `SELECT u.id,COALESCE(NULLIF(u.full_name,''),u.email,u.phone,'Customer') AS name,COALESCE(u.email,'') AS email,COALESCE(u.phone,'') AS phone,u.role,CASE WHEN u.disabled_at IS NULL THEN 'ACTIVE' ELSE 'DISABLED' END AS status,u.email_verified_at,u.created_at,(SELECT count(*) FROM public.orders o WHERE o.customer_id IN(SELECT c.id FROM public.customers c WHERE c.id=u.id OR c.user_id=u.id)) AS order_count,COALESCE((SELECT sum(o.total_amount) FROM public.orders o WHERE o.status='DELIVERED' AND o.customer_id IN(SELECT c.id FROM public.customers c WHERE c.id=u.id OR c.user_id=u.id)),0) AS delivered_value,concat_ws(' ',u.full_name,u.email,u.phone,u.id::text) AS search FROM public.users u WHERE u.deleted_at IS NULL AND u.role='CUSTOMER'`,
    "users":       `SELECT u.id,COALESCE(NULLIF(u.full_name,''),u.email,u.phone,'User') AS name,COALESCE(u.email,'') AS email,COALESCE(u.phone,'') AS phone,u.role,CASE WHEN u.disabled_at IS NULL THEN 'ACTIVE' ELSE 'DISABLED' END AS status,u.email_verified_at,u.created_at,concat_ws(' ',u.full_name,u.email,u.phone,u.id::text) AS search FROM public.users u WHERE u.deleted_at IS NULL`,
    "restaurants": `SELECT m.id,m.name,m.description,COALESCE(m.phone,'') AS phone,COALESCE(u.email,m.email,'') AS email,m.owner_id,m.approval_status AS status,m.fee_payment_status,m.is_active,m.tags,m.rejection_reason,m.created_at,COALESCE((SELECT address FROM merchants.merchant_branches b WHERE b.merchant_id=m.id AND b.deleted_at IS NULL ORDER BY b.created_at,b.id LIMIT 1),'') AS address,(SELECT count(*) FROM public.orders o WHERE o.merchant_id=m.id) AS order_count,concat_ws(' ',m.name,m.email,u.email,m.id::text) AS search FROM public.merchants m LEFT JOIN public.users u ON u.id=m.owner_id WHERE m.deleted_at IS NULL`,
    "partners":    `SELECT p.id,p.name,p.phone,COALESCE(u.email,'') AS email,p.user_id,p.approval_status AS status,p.status AS availability,p.eligibility_expires_at,p.rejection_reason,p.created_at,COALESCE((SELECT vehicle_type FROM delivery.vehicles v WHERE v.partner_id=p.id ORDER BY id LIMIT 1),'') AS vehicle_type,COALESCE((SELECT registration_number FROM delivery.vehicles v WHERE v.partner_id=p.id ORDER BY id LIMIT 1),'') AS registration_number,(SELECT count(*) FROM delivery.delivery_assignments a WHERE a.partner_id=p.id AND a.status='DELIVERED') AS delivery_count,concat_ws(' ',p.name,p.phone,u.email,p.id::text) AS search FROM delivery.delivery_partners p JOIN public.users u ON u.id=p.user_id WHERE p.deleted_at IS NULL`,
    "orders":      `SELECT o.id,'Order #'||left(o.id::text,8) AS name,o.status,o.total_amount,o.currency,COALESCE(c.user_id,c.id) AS customer_id,o.merchant_id,m.name AS restaurant,COALESCE(c.name,'') AS customer,o.created_at,(SELECT count(*) FROM delivery.offers f WHERE f.order_id=o.id) AS offer_attempts,o.status IN('ACCEPTED','PREPARING','READY_FOR_PICKUP') AND NOT EXISTS(SELECT 1 FROM delivery.delivery_assignments a WHERE a.order_id=o.id AND a.status IN('ASSIGNED','PICKED_UP','OUT_FOR_DELIVERY')) AS unassigned,concat_ws(' ',o.id::text,m.name,c.name) AS search FROM public.orders o JOIN public.merchants m ON m.id=o.merchant_id JOIN public.customers c ON c.id=o.customer_id WHERE o.deleted_at IS NULL`,
    "payments":    `SELECT p.id,'Payment #'||left(p.id::text,8) AS name,p.order_id,p.provider,p.method,p.amount,p.currency,p.status,p.created_at,concat_ws(' ',p.id::text,p.order_id::text,p.provider) AS search FROM payments.payments p UNION ALL SELECT p.merchant_id AS id,m.name AS name,NULL::uuid AS order_id,'STRIPE' AS provider,'ONBOARDING' AS method,p.amount,p.currency,p.status,p.created_at,concat_ws(' ',m.name,p.merchant_id::text,'onboarding') AS search FROM payments.onboarding_payments p JOIN public.merchants m ON m.id=p.merchant_id`,
    "reviews":     `SELECT r.id,m.name AS name,r.order_id,r.rating,r.comment,COALESCE(c.name,'') AS customer,r.created_at,CASE WHEN r.hidden_at IS NULL THEN 'PUBLISHED' ELSE 'HIDDEN' END AS status,r.moderation_reason,concat_ws(' ',m.name,c.name,r.comment) AS search FROM merchants.restaurant_reviews r JOIN public.merchants m ON m.id=r.merchant_id JOIN public.customers c ON c.id=r.customer_id`,
    "coupons":     `SELECT id,code AS name,code,description,discount_type,discount_value,min_order_amount,COALESCE(max_discount_amount,0) AS max_discount_amount,is_active,valid_until,used_count,created_at,CASE WHEN is_active THEN 'ACTIVE' ELSE 'INACTIVE' END AS status,concat_ws(' ',code,description) AS search FROM orders.coupons`,
    "audit":       `SELECT a.id,a.action AS name,a.action,a.admin_id,COALESCE(u.email,'') AS actor,a.target_type,COALESCE(a.target_id,'') AS target_id,a.details,a.created_at,'RECORDED' AS status,concat_ws(' ',a.action,a.target_type,a.target_id,u.email) AS search FROM public.admin_audit_logs a JOIN public.users u ON u.id=a.admin_id`,
    "emails":      `SELECT id,subject AS name,recipient,status,attempts,last_error,next_attempt_at,sent_at,created_at,concat_ws(' ',subject,recipient,id::text) AS search FROM notifications.email_outbox`,
}

func (r *pgRepo) ConsoleList(ctx context.Context, resource string, filter ListFilter) (*Page, error) {
    base, ok := consoleViews[resource]
    if !ok {
        return nil, errors.New("unknown management resource")
    }
    if filter.Page < 1 {
        filter.Page = 1
    }
    if filter.Limit < 1 || filter.Limit > 100 {
        filter.Limit = 25
    }
    if filter.Page > 100000 {
        return nil, errors.New("page is too large")
    }
    where := ` WHERE ($1='' OR s.search ILIKE '%'||$1||'%') AND ($2='' OR s.status=$2) AND ($3='' OR to_jsonb(s)->>'role'=$3)`
    args := []interface{}{strings.TrimSpace(filter.Search), filter.Status, filter.Role}
    result := &Page{Items: []json.RawMessage{}, Page: filter.Page, Limit: filter.Limit}
    if err := r.db.QueryRow(ctx, `SELECT count(*) FROM (`+base+`) s`+where, args...).Scan(&result.Total); err != nil {
        return nil, err
    }
    args = append(args, filter.Limit, (filter.Page-1)*filter.Limit)
    rows, err := r.db.Query(ctx, `SELECT to_jsonb(s)-'search' FROM (`+base+`) s`+where+` ORDER BY s.created_at DESC,s.id LIMIT $4 OFFSET $5`, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    for rows.Next() {
        var data json.RawMessage
        if err = rows.Scan(&data); err != nil {
            return nil, err
        }
        result.Items = append(result.Items, data)
    }
    return result, rows.Err()
}

func (r *pgRepo) ConsoleDetail(ctx context.Context, resource, id string) (map[string]interface{}, error) {
    base, ok := consoleViews[resource]
    if !ok {
        return nil, errors.New("unknown management resource")
    }
    var record json.RawMessage
    if err := r.db.QueryRow(ctx, `SELECT to_jsonb(s)-'search' FROM (`+base+`) s WHERE s.id=$1`, id).Scan(&record); err != nil {
        return nil, err
    }
    result := map[string]interface{}{"record": record}
    related := map[string]string{}
    switch resource {
    case "customers", "users":
        related["orders"] = `SELECT jsonb_build_object('id',o.id,'name',m.name,'status',o.status,'total_amount',o.total_amount,'currency',o.currency,'created_at',o.created_at) FROM public.orders o JOIN public.merchants m ON m.id=o.merchant_id WHERE o.customer_id IN(SELECT id FROM public.customers WHERE id=$1 OR user_id=$1) ORDER BY o.created_at DESC LIMIT 50`
        related["addresses"] = `SELECT jsonb_build_object('id',a.id,'label',a.label,'address',concat_ws(', ',a.address_line1,a.address_line2,a.city,a.state,a.postal_code)) FROM users.customer_addresses a WHERE a.customer_id IN(SELECT id FROM public.customers WHERE id=$1 OR user_id=$1) AND a.deleted_at IS NULL`
    case "restaurants":
        related["menu"] = `SELECT jsonb_build_object('id',i.id,'name',i.name,'description',i.description,'price',i.base_price,'currency',i.currency,'is_available',i.is_available,'category',c.name) FROM catalog.items i JOIN catalog.categories c ON c.id=i.category_id WHERE i.merchant_id=$1 AND i.deleted_at IS NULL ORDER BY c.sort_order,i.name`
        related["orders"] = `SELECT jsonb_build_object('id',id,'name','Order #'||left(id::text,8),'status',status,'total_amount',total_amount,'currency',currency,'created_at',created_at) FROM public.orders WHERE merchant_id=$1 ORDER BY created_at DESC LIMIT 50`
        related["hours"] = `SELECT jsonb_build_object('id',h.id,'day',h.day_of_week,'opens',h.opens_at::text,'closes',h.closes_at::text,'closed',h.is_closed) FROM merchants.merchant_operating_hours h JOIN merchants.merchant_branches b ON b.id=h.branch_id WHERE b.merchant_id=$1 ORDER BY h.day_of_week`
    case "partners":
        related["deliveries"] = `SELECT jsonb_build_object('id',a.order_id,'name','Order #'||left(a.order_id::text,8),'status',a.status,'created_at',a.created_at) FROM delivery.delivery_assignments a WHERE a.partner_id=$1 ORDER BY a.created_at DESC LIMIT 50`
        related["earnings"] = `SELECT jsonb_build_object('total_deliveries',count(*),'total_earnings',COALESCE(sum(delivery_fee+tip_amount),0),'tips',COALESCE(sum(tip_amount),0)) FROM delivery.rider_earnings WHERE partner_id=$1`
    case "orders":
        related["items"] = `SELECT jsonb_build_object('id',id,'name',name_snapshot,'quantity',quantity,'unit_price',unit_price,'total_price',total_price) FROM orders.order_items WHERE order_id=$1`
        related["timeline"] = `SELECT jsonb_build_object('id',id,'status',status,'notes',COALESCE(notes,''),'created_at',created_at) FROM orders.order_status_history WHERE order_id=$1 ORDER BY created_at`
        related["offers"] = `SELECT jsonb_build_object('id',f.id,'partner_id',f.partner_id,'name',p.name,'status',f.status,'expires_at',f.expires_at,'created_at',f.created_at) FROM delivery.offers f JOIN delivery.delivery_partners p ON p.id=f.partner_id WHERE f.order_id=$1 ORDER BY f.created_at`
        related["address"] = `SELECT delivery_address FROM public.orders WHERE id=$1`
    }
    for key, query := range related {
        data, err := r.jsonRows(ctx, query, id)
        if err != nil {
            return nil, err
        }
        result[key] = data
    }
    if resource == "restaurants" || resource == "partners" {
        var application json.RawMessage
        err := r.db.QueryRow(ctx, `SELECT jsonb_build_object('data',data,'document_ids',document_ids,'created_at',created_at) FROM public.partner_applications WHERE resource_id=$1`, id).Scan(&application)
        if err != nil && !errors.Is(err, pgx.ErrNoRows) {
            return nil, err
        }
        if err == nil {
            result["application"] = application
        }
    }
    return result, nil
}

type ProfileEdit struct {
    Name               string `json:"name"`
    Phone              string `json:"phone"`
    Description        string `json:"description"`
    Address            string `json:"address"`
    Reason             string `json:"reason"`
    VehicleType        string `json:"vehicle_type"`
    RegistrationNumber string `json:"registration_number"`
}

func (r *pgRepo) ConsoleEditProfile(ctx context.Context, actor, resource, id string, input ProfileEdit) error {
    if strings.TrimSpace(input.Name) == "" || len(input.Name) > 200 || len(input.Phone) > 30 || len(input.Description) > 5000 || len(input.Address) > 2000 {
        return errors.New("valid name and contact details required")
    }
    if err := validateReason(input.Reason); err != nil {
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
    switch resource {
    case "customers", "users":
        condition := ""
        if resource == "customers" {
            condition = " AND role='CUSTOMER'"
        }
        err = tx.QueryRow(ctx, `UPDATE public.users SET full_name=$2,phone=NULLIF($3,''),updated_at=now() WHERE id=$1 AND deleted_at IS NULL`+condition+` RETURNING id`, id, input.Name, input.Phone).Scan(&changed)
        if err == nil {
            _, err = tx.Exec(ctx, `UPDATE public.customers SET name=$2,phone=NULLIF($3,''),updated_at=now() WHERE id=$1 OR user_id=$1`, id, input.Name, input.Phone)
        }
    case "restaurants":
        err = tx.QueryRow(ctx, `UPDATE public.merchants SET name=$2,phone=$3,description=$4,updated_at=now() WHERE id=$1 AND deleted_at IS NULL RETURNING id`, id, input.Name, input.Phone, input.Description).Scan(&changed)
        if err == nil && input.Address != "" {
            tag, e := tx.Exec(ctx, `UPDATE merchants.merchant_branches SET address=$2,updated_at=now(),updated_by=$3 WHERE id=(SELECT id FROM merchants.merchant_branches WHERE merchant_id=$1 AND deleted_at IS NULL ORDER BY created_at,id LIMIT 1)`, id, input.Address, actor)
            err = e
            if err == nil && tag.RowsAffected() == 0 {
                _, err = tx.Exec(ctx, `INSERT INTO merchants.merchant_branches(merchant_id,name,address,created_by,updated_by) VALUES($1,$2,$3,$4,$4)`, id, input.Name, input.Address, actor)
            }
        }
    case "partners":
        if len(input.VehicleType) > 100 || len(input.RegistrationNumber) > 100 {
            return errors.New("vehicle details are too long")
        }
        err = tx.QueryRow(ctx, `UPDATE delivery.delivery_partners SET name=$2,phone=$3,updated_at=now() WHERE id=$1 AND deleted_at IS NULL RETURNING id`, id, input.Name, input.Phone).Scan(&changed)
        if err == nil && input.VehicleType != "" && input.RegistrationNumber != "" {
            tag, e := tx.Exec(ctx, `UPDATE delivery.vehicles SET vehicle_type=$2,registration_number=$3 WHERE id=(SELECT id FROM delivery.vehicles WHERE partner_id=$1 ORDER BY id LIMIT 1)`, id, input.VehicleType, input.RegistrationNumber)
            err = e
            if err == nil && tag.RowsAffected() == 0 {
                _, err = tx.Exec(ctx, `INSERT INTO delivery.vehicles(partner_id,vehicle_type,registration_number) VALUES($1,$2,$3)`, id, input.VehicleType, input.RegistrationNumber)
            }
        }
    default:
        return errors.New("profile cannot be edited")
    }
    if err != nil {
        return err
    }
    if err = consoleAudit(ctx, tx, actor, "PROFILE_UPDATED", resource, id, map[string]interface{}{"reason": input.Reason}); err != nil {
        return err
    }
    return tx.Commit(ctx)
}

func validateReason(reason string) error {
    if strings.TrimSpace(reason) == "" || len(reason) > 2000 {
        return errors.New("a reason of at most 2000 characters is required")
    }
    return nil
}
func checkConsoleAdmin(ctx context.Context, tx pgx.Tx, actor string) error {
    var allowed bool
    if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.users WHERE id=$1 AND role='ADMIN' AND deleted_at IS NULL AND disabled_at IS NULL)`, actor).Scan(&allowed); err != nil {
        return err
    }
    if !allowed {
        return errors.New("administrator permission required")
    }
    return nil
}
func consoleAudit(ctx context.Context, tx pgx.Tx, actor, action, kind, id string, details interface{}) error {
    data, err := json.Marshal(details)
    if err != nil {
        return err
    }
    _, err = tx.Exec(ctx, `INSERT INTO public.admin_audit_logs(admin_id,action,target_type,target_id,details) VALUES($1,$2,$3,$4,$5)`, actor, action, kind, id, data)
    return err
}

func (r *pgRepo) ConsoleDisable(ctx context.Context, actor, id string, disabled bool, reason string) error {
    if err := validateReason(reason); err != nil {
        return err
    }
    if actor == id && disabled {
        return errors.New("you cannot disable your own administrator account")
    }
    tx, err := r.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(894213)`); err != nil {
        return err
    }
    if err = checkConsoleAdmin(ctx, tx, actor); err != nil {
        return err
    }
    var role string
    if err = tx.QueryRow(ctx, `SELECT role FROM public.users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&role); err != nil {
        return err
    }
    if disabled && role == "ADMIN" {
        var count int
        if err = tx.QueryRow(ctx, `SELECT count(*) FROM public.users WHERE role='ADMIN' AND disabled_at IS NULL AND deleted_at IS NULL`).Scan(&count); err != nil {
            return err
        }
        if count <= 1 {
            return errors.New("cannot disable the last administrator")
        }
    }
    if _, err = tx.Exec(ctx, `UPDATE public.users SET disabled_at=CASE WHEN $2 THEN now() ELSE NULL END,updated_at=now() WHERE id=$1`, id, disabled); err != nil {
        return err
    }
    if disabled {
        if _, err = tx.Exec(ctx, `UPDATE auth.refresh_sessions SET revoked_at=now() WHERE user_id=$1`, id); err != nil {
            return err
        }
        if _, err = tx.Exec(ctx, `UPDATE public.merchants SET is_active=false,updated_at=now() WHERE owner_id=$1`, id); err != nil {
            return err
        }
        if _, err = tx.Exec(ctx, `UPDATE delivery.delivery_partners SET status='OFFLINE',updated_at=now() WHERE user_id=$1`, id); err != nil {
            return err
        }
        if _, err = tx.Exec(ctx, `UPDATE delivery.offers SET status='EXPIRED',updated_at=now() WHERE status='PENDING' AND partner_id IN(SELECT id FROM delivery.delivery_partners WHERE user_id=$1)`, id); err != nil {
            return err
        }
    }
    if err = consoleAudit(ctx, tx, actor, "ACCOUNT_STATUS_UPDATED", "user", id, map[string]interface{}{"disabled": disabled, "reason": reason}); err != nil {
        return err
    }
    return tx.Commit(ctx)
}

func (r *pgRepo) ConsoleAvailability(ctx context.Context, actor, resource, id string, available bool, reason string) error {
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
    if resource == "restaurants" {
        err = tx.QueryRow(ctx, `UPDATE public.merchants m SET is_active=$2,updated_at=now() WHERE m.id=$1 AND m.deleted_at IS NULL AND (NOT $2 OR (m.approval_status='APPROVED' AND m.fee_payment_status='PAID' AND EXISTS(SELECT 1 FROM public.users u WHERE u.id=m.owner_id AND u.disabled_at IS NULL AND u.deleted_at IS NULL))) RETURNING id`, id, available).Scan(&changed)
    } else if resource == "partners" {
        status := "OFFLINE"
        if available {
            status = "ONLINE"
        }
        err = tx.QueryRow(ctx, `UPDATE delivery.delivery_partners p SET status=$2,updated_at=now() WHERE p.id=$1 AND p.deleted_at IS NULL AND (NOT $3 OR (p.approval_status='APPROVED' AND (p.eligibility_expires_at IS NULL OR p.eligibility_expires_at>now()) AND EXISTS(SELECT 1 FROM delivery.vehicles v WHERE v.partner_id=p.id AND v.is_active) AND EXISTS(SELECT 1 FROM public.users u WHERE u.id=p.user_id AND u.disabled_at IS NULL AND u.deleted_at IS NULL))) RETURNING id`, id, status, available).Scan(&changed)
    } else {
        return errors.New("unsupported resource")
    }
    if err != nil {
        return errors.New("partner is missing or does not meet approval, payment or eligibility requirements")
    }
    if resource == "partners" && !available {
        if _, err = tx.Exec(ctx, `UPDATE delivery.offers SET status='EXPIRED',updated_at=now() WHERE partner_id=$1 AND status='PENDING'`, id); err != nil {
            return err
        }
    }
    if err = consoleAudit(ctx, tx, actor, "AVAILABILITY_UPDATED", resource, id, map[string]interface{}{"available": available, "reason": reason}); err != nil {
        return err
    }
    return tx.Commit(ctx)
}

type MenuEdit struct {
    Name        string `json:"name"`
    Description string `json:"description"`
    Price       int64  `json:"price"`
    Available   bool   `json:"is_available"`
    Reason      string `json:"reason"`
}

func (r *pgRepo) ConsoleMenu(ctx context.Context, actor, restaurant, item string, input MenuEdit) error {
    if strings.TrimSpace(input.Name) == "" || len(input.Name) > 200 || len(input.Description) > 5000 || input.Price <= 0 || input.Price > 1000000000 {
        return errors.New("name and a positive price in minor currency units are required")
    }
    if err := validateReason(input.Reason); err != nil {
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
    tag, err := tx.Exec(ctx, `UPDATE catalog.items SET name=$3,description=$4,base_price=$5,is_available=$6,updated_at=now() WHERE id=$1 AND merchant_id=$2 AND deleted_at IS NULL`, item, restaurant, input.Name, input.Description, input.Price, input.Available)
    if err != nil {
        return err
    }
    if tag.RowsAffected() != 1 {
        return errors.New("menu item not found for this restaurant")
    }
    if err = consoleAudit(ctx, tx, actor, "MENU_ITEM_UPDATED", "menu_item", item, map[string]interface{}{"reason": input.Reason, "restaurant_id": restaurant, "price": input.Price, "available": input.Available}); err != nil {
        return err
    }
    return tx.Commit(ctx)
}

func (r *pgRepo) ConsoleCoupon(ctx context.Context, actor, id string, input CouponInput, reason string) error {
    input.Code = strings.ToUpper(strings.TrimSpace(input.Code))
    if input.Code == "" || len(input.Code) > 50 || len(input.Description) > 2000 || input.DiscountValue <= 0 || input.MinOrderAmount < 0 || input.MaxDiscountAmount < 0 || (input.DiscountType != "FLAT" && input.DiscountType != "PERCENTAGE") || (input.DiscountType == "PERCENTAGE" && input.DiscountValue > 100) {
        return errors.New("invalid coupon terms")
    }
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
    if id == "" {
        err = tx.QueryRow(ctx, `INSERT INTO orders.coupons(code,description,discount_type,discount_value,min_order_amount,max_discount_amount,is_active,valid_until) VALUES($1,$2,$3,$4,$5,NULLIF($6,0),$7,NULLIF($8::timestamptz,'0001-01-01'::timestamptz)) RETURNING id`, input.Code, input.Description, input.DiscountType, input.DiscountValue, input.MinOrderAmount, input.MaxDiscountAmount, input.IsActive, input.ValidUntil).Scan(&id)
    } else {
        var found string
        err = tx.QueryRow(ctx, `UPDATE orders.coupons SET code=$2,description=$3,discount_type=$4,discount_value=$5,min_order_amount=$6,max_discount_amount=NULLIF($7,0),is_active=$8,updated_at=now(),valid_until=NULLIF($9::timestamptz,'0001-01-01'::timestamptz) WHERE id=$1 RETURNING id`, id, input.Code, input.Description, input.DiscountType, input.DiscountValue, input.MinOrderAmount, input.MaxDiscountAmount, input.IsActive, input.ValidUntil).Scan(&found)
    }
    if err != nil {
        return fmt.Errorf("coupon could not be saved; the code may already exist")
    }
    if err = consoleAudit(ctx, tx, actor, "COUPON_SAVED", "coupon", id, map[string]interface{}{"reason": reason, "code": input.Code, "active": input.IsActive}); err != nil {
        return err
    }
    return tx.Commit(ctx)
}

package partnerops

import (
    "context"
    "encoding/json"
)

func orderScope(role string) string {
    if role == "MERCHANT_OWNER" {
        return `o.merchant_id IN(SELECT id FROM public.merchants WHERE owner_id=$1 AND deleted_at IS NULL)`
    }
    return `EXISTS(SELECT 1 FROM delivery.delivery_assignments a JOIN delivery.delivery_partners p ON p.id=a.partner_id WHERE a.order_id=o.id AND p.user_id=$1 AND p.deleted_at IS NULL)`
}

func (r *Repository) Orders(ctx context.Context, user, role, scope, status, search string, page, limit int) (*Page, error) {
    terminal := `o.status IN('DELIVERED','CANCELLED','REJECTED','REFUNDED')`
    where := ` WHERE o.deleted_at IS NULL AND ` + orderScope(role) + ` AND ($2='' OR o.status::text=$2) AND ($3='' OR concat_ws(' ',o.id::text,m.name,c.name) ILIKE '%'||$3||'%')`
    if scope == "history" {
        where += ` AND ` + terminal
    } else if scope == "active" {
        where += ` AND NOT (` + terminal + `)`
    }
    base := ` FROM public.orders o JOIN public.merchants m ON m.id=o.merchant_id JOIN public.customers c ON c.id=o.customer_id`
    result := &Page{Items: []json.RawMessage{}, Page: page, Limit: limit}
    if err := r.pool.QueryRow(ctx, `SELECT count(*)`+base+where, user, status, search).Scan(&result.Total); err != nil {
        return nil, err
    }
    query := `SELECT jsonb_build_object('id',o.id,'status',o.status,'restaurant',m.name,'customer',c.name,'total_amount',o.total_amount,'currency',o.currency,'created_at',o.created_at,'items_count',(SELECT COALESCE(sum(quantity),0) FROM orders.order_items WHERE order_id=o.id),'assignment_status',(SELECT a.status FROM delivery.delivery_assignments a JOIN delivery.delivery_partners p ON p.id=a.partner_id WHERE a.order_id=o.id AND p.user_id=$1 LIMIT 1),'delivery_fee',o.delivery_fee)` + base + where + ` ORDER BY o.created_at DESC,o.id LIMIT $4 OFFSET $5`
    var err error
    result.Items, err = r.rows(ctx, query, user, status, search, limit, (page-1)*limit)
    return result, err
}

func (r *Repository) Order(ctx context.Context, user, role, id string) (json.RawMessage, error) {
    // Customer contact/address is available only to the assigned courier while
    // delivery is active. Restaurant owners receive fulfillment information.
    activeCourier := `o.status NOT IN('DELIVERED','CANCELLED','REJECTED','REFUNDED') AND EXISTS(SELECT 1 FROM delivery.delivery_assignments a JOIN delivery.delivery_partners p ON p.id=a.partner_id WHERE a.order_id=o.id AND p.user_id=$1 AND a.status IN('ASSIGNED','PICKED_UP','OUT_FOR_DELIVERY'))`
    query := `SELECT jsonb_build_object(
        'id',o.id,'status',o.status,'restaurant',m.name,'customer',c.name,'total_amount',o.total_amount,'currency',o.currency,'created_at',o.created_at,'delivery_fee',o.delivery_fee,
        'pickup_address',COALESCE((SELECT address FROM merchants.merchant_branches WHERE merchant_id=m.id AND deleted_at IS NULL ORDER BY created_at,id LIMIT 1),''),
        'restaurant_phone',CASE WHEN ` + activeCourier + ` THEN COALESCE(m.phone,'') ELSE '' END,
        'customer_phone',CASE WHEN ` + activeCourier + ` THEN COALESCE(c.phone,'') ELSE '' END,
        'delivery_address',CASE WHEN ` + activeCourier + ` THEN o.delivery_address ELSE '{}'::jsonb END,
        'assignment_status',(SELECT a.status FROM delivery.delivery_assignments a JOIN delivery.delivery_partners p ON p.id=a.partner_id WHERE a.order_id=o.id AND p.user_id=$1 LIMIT 1),
        'courier',(SELECT jsonb_build_object('name',p.name,'status',a.status,'phone',CASE WHEN o.status NOT IN('DELIVERED','CANCELLED','REJECTED','REFUNDED') THEN p.phone ELSE '' END) FROM delivery.delivery_assignments a JOIN delivery.delivery_partners p ON p.id=a.partner_id WHERE a.order_id=o.id ORDER BY a.created_at DESC LIMIT 1),
        'items',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',id,'name',name_snapshot,'quantity',quantity,'unit_price',unit_price,'total_price',total_price)) FROM orders.order_items WHERE order_id=o.id),'[]'::jsonb),
        'timeline',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',id,'status',status,'notes',COALESCE(notes,''),'created_at',created_at) ORDER BY created_at) FROM orders.order_status_history WHERE order_id=o.id),'[]'::jsonb))
        FROM public.orders o JOIN public.merchants m ON m.id=o.merchant_id JOIN public.customers c ON c.id=o.customer_id WHERE o.id=$2 AND o.deleted_at IS NULL AND ` + orderScope(role)
    var data json.RawMessage
    err := r.pool.QueryRow(ctx, query, user, id).Scan(&data)
    return data, err
}

func (r *Repository) Earnings(ctx context.Context, user string, page, limit, days int) (*Page, error) {
    result := &Page{Items: []json.RawMessage{}, Page: page, Limit: limit}
    base := ` FROM delivery.rider_earnings e JOIN delivery.delivery_partners p ON p.id=e.partner_id WHERE p.user_id=$1 AND p.deleted_at IS NULL AND e.earned_at>=((now() AT TIME ZONE 'UTC')::date-($2::int-1)) AT TIME ZONE 'UTC'`
    if err := r.pool.QueryRow(ctx, `SELECT count(*)`+base, user, days).Scan(&result.Total); err != nil {
        return nil, err
    }
    var err error
    result.Items, err = r.rows(ctx, `SELECT jsonb_build_object('id',e.id,'order_id',e.order_id,'delivery_fee',e.delivery_fee,'tip_amount',e.tip_amount,'amount',e.delivery_fee+e.tip_amount,'created_at',e.earned_at)`+base+` ORDER BY e.earned_at DESC,e.id LIMIT $3 OFFSET $4`, user, days, limit, (page-1)*limit)
    return result, err
}
func (r *Repository) Reviews(ctx context.Context, user string, page, limit int) (*Page, error) {
    result := &Page{Items: []json.RawMessage{}, Page: page, Limit: limit}
    base := ` FROM merchants.restaurant_reviews r JOIN public.merchants m ON m.id=r.merchant_id JOIN public.customers c ON c.id=r.customer_id WHERE m.owner_id=$1 AND m.deleted_at IS NULL AND r.hidden_at IS NULL`
    if err := r.pool.QueryRow(ctx, `SELECT count(*)`+base, user).Scan(&result.Total); err != nil {
        return nil, err
    }
    var err error
    result.Items, err = r.rows(ctx, `SELECT jsonb_build_object('id',r.id,'order_id',r.order_id,'rating',r.rating,'comment',r.comment,'customer',c.name,'reply',r.reply,'replied_at',r.replied_at,'created_at',r.created_at)`+base+` ORDER BY r.created_at DESC,r.id LIMIT $2 OFFSET $3`, user, limit, (page-1)*limit)
    return result, err
}

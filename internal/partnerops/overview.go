package partnerops

import (
    "context"
    "encoding/json"
)

func (r *Repository) Overview(ctx context.Context, user, role string, days int) (json.RawMessage, error) {
    query := restaurantOverview
    if role == "DELIVERY_PARTNER" {
        query = deliveryOverview
    }
    var data json.RawMessage
    err := r.pool.QueryRow(ctx, query, user, days).Scan(&data)
    return data, err
}

const restaurantOverview = `
WITH owned AS (SELECT id FROM public.merchants WHERE owner_id=$1 AND deleted_at IS NULL),
all_orders AS (SELECT o.* FROM public.orders o WHERE o.merchant_id IN(SELECT id FROM owned) AND o.deleted_at IS NULL),
period AS (SELECT * FROM all_orders WHERE created_at>=((now() AT TIME ZONE 'UTC')::date-($2::int-1)) AT TIME ZONE 'UTC')
SELECT jsonb_build_object(
 'days',$2::int,'currency','INR',
 'total_orders',(SELECT count(*) FROM period),
 'delivered_orders',(SELECT count(*) FROM period WHERE status='DELIVERED'),
 'gross_sales',(SELECT COALESCE(sum(total_amount),0) FROM period WHERE status='DELIVERED'),
 'average_order_value',(SELECT COALESCE(round(avg(total_amount)),0) FROM period WHERE status='DELIVERED'),
 'cancelled_orders',(SELECT count(*) FROM period WHERE status IN('CANCELLED','REJECTED','REFUNDED')),
 'active_orders',(SELECT count(*) FROM all_orders WHERE status IN('CREATED','CONFIRMED','ACCEPTED','PREPARING','READY_FOR_PICKUP')),
 'new_orders',(SELECT count(*) FROM all_orders WHERE status IN('CREATED','CONFIRMED')),
 'rating',(SELECT COALESCE(round(avg(rating),1),0) FROM merchants.restaurant_reviews WHERE merchant_id IN(SELECT id FROM owned) AND hidden_at IS NULL),
 'review_count',(SELECT count(*) FROM merchants.restaurant_reviews WHERE merchant_id IN(SELECT id FROM owned) AND hidden_at IS NULL),
 'available_items',(SELECT count(*) FROM catalog.items WHERE merchant_id IN(SELECT id FROM owned) AND deleted_at IS NULL AND is_available),
 'unavailable_items',(SELECT count(*) FROM catalog.items WHERE merchant_id IN(SELECT id FROM owned) AND deleted_at IS NULL AND NOT is_available),
 'daily',(SELECT jsonb_agg(jsonb_build_object('day',d.day::date,'orders',(SELECT count(*) FROM period WHERE (created_at AT TIME ZONE 'UTC')::date=d.day::date),'amount',(SELECT COALESCE(sum(total_amount),0) FROM period WHERE status='DELIVERED' AND (created_at AT TIME ZONE 'UTC')::date=d.day::date)) ORDER BY d.day) FROM generate_series((now() AT TIME ZONE 'UTC')::date-($2::int-1),(now() AT TIME ZONE 'UTC')::date,interval '1 day') d(day)),
 'top_items',(SELECT COALESCE(jsonb_agg(to_jsonb(s)),'[]'::jsonb) FROM(SELECT i.name_snapshot AS name,sum(i.quantity) AS quantity,sum(i.total_price) AS amount FROM orders.order_items i JOIN period o ON o.id=i.order_id WHERE o.status='DELIVERED' GROUP BY i.name_snapshot ORDER BY sum(i.quantity) DESC,i.name_snapshot LIMIT 5)s))
`

const deliveryOverview = `
WITH owned AS (SELECT id FROM delivery.delivery_partners WHERE user_id=$1 AND deleted_at IS NULL),
earnings AS (SELECT * FROM delivery.rider_earnings WHERE partner_id IN(SELECT id FROM owned)),
period AS (SELECT * FROM earnings WHERE earned_at>=((now() AT TIME ZONE 'UTC')::date-($2::int-1)) AT TIME ZONE 'UTC')
SELECT jsonb_build_object(
 'days',$2::int,'currency','INR',
 'total_earnings',(SELECT COALESCE(sum(delivery_fee+tip_amount),0) FROM period),
 'delivery_fees',(SELECT COALESCE(sum(delivery_fee),0) FROM period),
 'tips',(SELECT COALESCE(sum(tip_amount),0) FROM period),
 'completed_deliveries',(SELECT count(*) FROM period),
 'today_earnings',(SELECT COALESCE(sum(delivery_fee+tip_amount),0) FROM earnings WHERE (earned_at AT TIME ZONE 'UTC')::date=(now() AT TIME ZONE 'UTC')::date),
 'active_deliveries',(SELECT count(*) FROM delivery.delivery_assignments WHERE partner_id IN(SELECT id FROM owned) AND status IN('ASSIGNED','PICKED_UP','OUT_FOR_DELIVERY')),
 'pending_offers',(SELECT count(*) FROM delivery.offers WHERE partner_id IN(SELECT id FROM owned) AND status='PENDING' AND expires_at>now()),
 'rating',(SELECT COALESCE(round(avg(rating),1),0) FROM delivery.partner_ratings WHERE partner_id IN(SELECT id FROM owned)),
 'rating_count',(SELECT count(*) FROM delivery.partner_ratings WHERE partner_id IN(SELECT id FROM owned)),
 'daily',(SELECT jsonb_agg(jsonb_build_object('day',d.day::date,'orders',(SELECT count(*) FROM period WHERE (earned_at AT TIME ZONE 'UTC')::date=d.day::date),'amount',(SELECT COALESCE(sum(delivery_fee+tip_amount),0) FROM period WHERE (earned_at AT TIME ZONE 'UTC')::date=d.day::date)) ORDER BY d.day) FROM generate_series((now() AT TIME ZONE 'UTC')::date-($2::int-1),(now() AT TIME ZONE 'UTC')::date,interval '1 day') d(day)))
`

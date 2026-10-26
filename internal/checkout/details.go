package checkout

import (
	"context"
	"encoding/json"
)

func (s *Service) Detail(ctx context.Context, userID, id string) (json.RawMessage, error) {
	var result json.RawMessage
	err := s.db.QueryRow(ctx, `SELECT json_build_object('id',o.id,'status',o.status,'merchant_name',m.name,'total_amount',o.total_amount,'currency',o.currency,'delivery_address',o.delivery_address,'created_at',o.created_at,'items',COALESCE((SELECT json_agg(json_build_object('name',i.name_snapshot,'quantity',i.quantity,'unit_price',i.unit_price,'total_price',i.total_price)) FROM orders.order_items i WHERE i.order_id=o.id),'[]'::json)) FROM public.orders o JOIN public.merchants m ON m.id=o.merchant_id WHERE o.id=$1 AND o.customer_id=$2 AND o.deleted_at IS NULL`, id, userID).Scan(&result)
	return result, err
}

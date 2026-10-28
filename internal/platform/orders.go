package platform

import (
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

func orderJSON() string {
	return project("orders", "o") + `||jsonb_build_object(
 'restaurant_name',r.name,'restaurant_address',r.address,'pickup_address',r.address,'restaurant_phone',r.phone,'customer_phone',(SELECT phone FROM users WHERE id=o.user_id),'assignment_status',(SELECT status FROM delivery_assignments WHERE order_id=o.id AND status NOT IN ('REJECTED','CANCELLED') AND deleted_at IS NULL ORDER BY id DESC LIMIT 1),'total',round(o.total_amount*100)::bigint,'delivery_fee_paise',round(o.delivery_fee*100)::bigint,
 'delivery_address',(SELECT ` + project("user_addresses", "a") + ` FROM user_addresses a WHERE a.id=o.address_id),
 'items',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',line.uuid,'menu_item_id',item.uuid,'variant_id',v.uuid,'name',item.name,'variant_name',v.name,'quantity',line.quantity,'unit_price',round(line.unit_price*100)::bigint,'total_price',round(line.unit_price*100)::bigint*line.quantity,'special_instructions',line.special_instructions)) FROM order_items line LEFT JOIN menu_items item ON item.id=line.menu_item_id LEFT JOIN item_variants v ON v.id=line.variant_id WHERE line.order_id=o.id),'[]'::jsonb),
 'payment',jsonb_build_object('collected',EXISTS(SELECT 1 FROM payments p WHERE p.order_id=o.id AND p.status='CAPTURED' AND p.amount_paise=round(o.total_amount*100)::bigint),'amount_paise',round(o.total_amount*100)::bigint,'provider',CASE WHEN EXISTS(SELECT 1 FROM payment_intents WHERE order_id=o.id AND method='RAZORPAY' AND status NOT IN ('CANCELLED','FAILED')) THEN 'RAZORPAY' ELSE 'COD' END,'status',CASE WHEN EXISTS(SELECT 1 FROM payments WHERE order_id=o.id AND status='CAPTURED') THEN 'PAID' ELSE 'PENDING' END,'amount',round(o.total_amount*100)::bigint,'currency',o.currency,'method',COALESCE((SELECT method FROM payments WHERE order_id=o.id AND status='CAPTURED' LIMIT 1),'PAY_ON_DELIVERY'),'online_pending',EXISTS(SELECT 1 FROM payment_intents WHERE order_id=o.id AND method='RAZORPAY' AND status IN ('CREATING','PENDING','UNKNOWN'))),
 'timeline',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',h.uuid,'status',h.to_status,'from_status',h.from_status,'created_at',h.changed_at,'note',h.note) ORDER BY h.changed_at,h.id) FROM order_status_history h WHERE h.order_id=o.id),'[]'::jsonb))`
}
func (s *Service) orderAccess(c echo.Context, tx pgx.Tx, lock bool) (int64, string, error) {
	if err := publicID(c.Param("id")); err != nil {
		return 0, "", err
	}
	query := `SELECT o.id,o.status FROM orders o JOIN restaurants r ON r.id=o.restaurant_id WHERE o.uuid=$1 AND o.deleted_at IS NULL AND ($2='ADMIN' OR ($2='CUSTOMER' AND o.user_id=$3) OR ($2='RESTAURANT_OWNER' AND r.owner_user_id=$3) OR ($2='DELIVERY_PARTNER' AND EXISTS(SELECT 1 FROM delivery_assignments a JOIN riders rider ON rider.id=a.rider_id WHERE a.order_id=o.id AND rider.user_id=$3 AND a.status NOT IN ('REJECTED','CANCELLED') AND a.deleted_at IS NULL)))`
	if lock {
		query += " FOR UPDATE OF o"
	}
	var id int64
	var status string
	err := tx.QueryRow(c.Request().Context(), query, c.Param("id"), person(c).Role, person(c).ID).Scan(&id, &status)
	if err != nil {
		return 0, "", unavailable(err)
	}
	return id, status, nil
}
func (s *Service) registerOrders(api *echo.Group) {
	list := func(c echo.Context) error {
		query := `SELECT ` + orderJSON() + ` FROM orders o JOIN restaurants r ON r.id=o.restaurant_id WHERE o.deleted_at IS NULL AND (($1='CUSTOMER' AND o.user_id=$2) OR ($1='RESTAURANT_OWNER' AND r.owner_user_id=$2) OR ($1='DELIVERY_PARTNER' AND EXISTS(SELECT 1 FROM delivery_assignments a JOIN riders rider ON rider.id=a.rider_id WHERE a.order_id=o.id AND rider.user_id=$2 AND a.status NOT IN ('REJECTED','CANCELLED') AND a.deleted_at IS NULL)))`
		if strings.HasPrefix(c.Path(), "/api/v1/partner/") {
			page, _ := strconv.Atoi(c.QueryParam("page"))
			if page < 1 {
				page = 1
			}
			if page > 10000 {
				return bad("page too large")
			}
			scope, status, search := c.QueryParam("scope"), c.QueryParam("status"), c.QueryParam("q")
			return s.one(c, "WITH visible(record) AS ("+query+"), filtered AS (SELECT record FROM visible WHERE ($3='' OR record->>'status'=$3) AND ($4='' OR record::text ILIKE '%'||$4||'%') AND ($5='' OR ($5='history')=(record->>'status' IN ('DELIVERED','CANCELLED','REJECTED','REFUNDED')))) SELECT jsonb_build_object('items',COALESCE((SELECT jsonb_agg(record) FROM (SELECT record FROM filtered ORDER BY record->>'created_at' DESC LIMIT 25 OFFSET $6) page),'[]'::jsonb),'total',(SELECT count(*) FROM filtered),'page',$7::int,'limit',25)", person(c).Role, person(c).ID, status, search, scope, (page-1)*25, page)
		}
		return s.list(c, query+" ORDER BY o.created_at DESC LIMIT 100", person(c).Role, person(c).ID)
	}
	detail := func(c echo.Context) error {
		tx, err := s.begin(c, "READ")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		id, _, err := s.orderAccess(c, tx, false)
		if err != nil {
			return err
		}
		return s.one(c, `SELECT `+orderJSON()+` FROM orders o JOIN restaurants r ON r.id=o.restaurant_id WHERE o.id=$1`, id)
	}
	api.GET("/customer/orders", list, roles("CUSTOMER"))
	api.GET("/restaurant/orders", list, roles("RESTAURANT_OWNER"))
	api.GET("/rider/deliveries", list, roles("DELIVERY_PARTNER"))
	api.GET("/partner/orders", list, roles("RESTAURANT_OWNER", "DELIVERY_PARTNER"))
	for _, path := range []string{"/customer/orders/:id", "/partner/orders/:id", "/restaurant/orders/:id", "/rider/deliveries/:id"} {
		api.GET(path, detail)
	}
	api.PATCH("/restaurant/orders/:id/status", func(c echo.Context) error {
		var input struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		}
		if c.Bind(&input) != nil {
			return bad("order status required")
		}
		input.Status = strings.ToUpper(input.Status)
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		id, current, err := s.orderAccess(c, tx, true)
		if err != nil {
			return err
		}
		allowed := map[string][]string{"CONFIRMED": {"ACCEPTED", "REJECTED"}, "ACCEPTED": {"PREPARING"}, "PREPARING": {"READY_FOR_PICKUP"}}
		valid := false
		for _, next := range allowed[current] {
			if next == input.Status {
				valid = true
			}
		}
		if !valid {
			return bad("invalid order transition")
		}
		if input.Status == "REJECTED" && !validName(input.Reason, 2000) {
			return bad("rejection reason required")
		}
		if _, err = tx.Exec(c.Request().Context(), `SELECT set_config('yember.reason',$1,true)`, input.Reason); err != nil {
			return err
		}
		if _, err = tx.Exec(c.Request().Context(), `UPDATE orders SET status=$2 WHERE id=$1`, id, input.Status); err != nil {
			return unavailable(err)
		}
		if input.Status == "READY_FOR_PICKUP" {
			if err = s.assignOrder(c, tx, id, ""); err != nil && err != pgx.ErrNoRows {
				return err
			}
		}
		return commit(c, tx)
	}, roles("RESTAURANT_OWNER"))
	api.POST("/admin/manage/orders/:id/assign", func(c echo.Context) error {
		var input struct {
			Rider string `json:"rider_id"`
		}
		if c.Bind(&input) != nil || publicID(input.Rider) != nil {
			return bad("rider UUID required")
		}
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		id, status, err := s.orderAccess(c, tx, true)
		if err != nil {
			return err
		}
		if status != "READY_FOR_PICKUP" {
			return bad("order must be ready for pickup")
		}
		if err = s.assignOrder(c, tx, id, input.Rider); err != nil {
			return bad("no eligible rider available")
		}
		return commit(c, tx)
	}, roles("ADMIN"))
	api.PATCH("/rider/deliveries/:id/status", func(c echo.Context) error {
		var input struct {
			Status string `json:"status"`
			Cash   bool   `json:"cash_collected"`
			Amount int64  `json:"amount_collected_paise"`
		}
		if c.Bind(&input) != nil {
			return bad("delivery status required")
		}
		input.Status = strings.ToUpper(input.Status)
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		order, orderStatus, err := s.orderAccess(c, tx, true)
		if err != nil {
			return err
		}
		var assignment, rider int64
		var current string
		if err = tx.QueryRow(c.Request().Context(), `SELECT a.id,a.rider_id,a.status FROM delivery_assignments a JOIN riders r ON r.id=a.rider_id WHERE a.order_id=$1 AND r.user_id=$2 AND a.status NOT IN ('REJECTED','CANCELLED') AND a.deleted_at IS NULL FOR UPDATE OF a`, order, person(c).ID).Scan(&assignment, &rider, &current); err != nil {
			return unavailable(err)
		}
		if current == input.Status {
			return c.NoContent(204)
		}
		nextOrder := orderStatus
		timeColumn := ""
		switch {
		case current == "ASSIGNED" && input.Status == "ACCEPTED":
			timeColumn = "accepted_at"
		case current == "ASSIGNED" && input.Status == "REJECTED":
		case current == "ACCEPTED" && input.Status == "PICKED_UP" && orderStatus == "READY_FOR_PICKUP":
			timeColumn = "picked_up_at"
			nextOrder = "PICKED_UP"
		case current == "PICKED_UP" && input.Status == "DELIVERED" && orderStatus == "PICKED_UP":
			if input.Cash {
				if err = s.collectCash(c, tx, order, rider, input.Amount); err != nil {
					return err
				}
			}
			var collected bool
			if err = tx.QueryRow(c.Request().Context(), `SELECT EXISTS(SELECT 1 FROM payments WHERE order_id=$1 AND status='CAPTURED')`, order).Scan(&collected); err != nil {
				return err
			}
			if !collected && !input.Cash {
				return bad("record cash collection or wait for verified online payment")
			}
			timeColumn = "delivered_at"
			nextOrder = "DELIVERED"
		default:
			return bad("invalid delivery transition")
		}
		update := `UPDATE delivery_assignments SET status=$2`
		if timeColumn != "" {
			update += "," + timeColumn + "=now()"
		}
		if _, err = tx.Exec(c.Request().Context(), update+" WHERE id=$1", assignment, input.Status); err != nil {
			return unavailable(err)
		}
		if nextOrder != orderStatus {
			if _, err = tx.Exec(c.Request().Context(), `UPDATE orders SET status=$2 WHERE id=$1`, order, nextOrder); err != nil {
				return err
			}
		}
		if nextOrder == "DELIVERED" {
			if err = s.recognize(c, tx, order); err != nil {
				return err
			}
		}
		return commit(c, tx)
	}, roles("DELIVERY_PARTNER"))
	api.POST("/rider/location", func(c echo.Context) error {
		var input struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}
		if c.Bind(&input) != nil || input.Latitude < -90 || input.Latitude > 90 || input.Longitude < -180 || input.Longitude > 180 {
			return bad("valid coordinates required")
		}
		rider, err := s.riderID(c)
		if err != nil {
			return err
		}
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), `INSERT INTO rider_locations(rider_id,latitude,longitude,recorded_at) VALUES($1,$2,$3,now())`, rider, input.Latitude, input.Longitude); err != nil {
			return err
		}
		return commit(c, tx)
	}, roles("DELIVERY_PARTNER"))
	api.GET("/customer/orders/:id/tracking", func(c echo.Context) error {
		tx, err := s.begin(c, "READ")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		order, _, err := s.orderAccess(c, tx, false)
		if err != nil {
			return err
		}
		return s.one(c, `SELECT jsonb_build_object('order_id',$2::text,'location',(SELECT jsonb_build_object('latitude',l.latitude,'longitude',l.longitude,'recorded_at',l.recorded_at) FROM rider_locations l JOIN delivery_assignments a ON a.rider_id=l.rider_id WHERE a.order_id=$1 AND a.status IN ('ACCEPTED','PICKED_UP') AND l.recorded_at>now()-interval '5 minutes' ORDER BY l.recorded_at DESC LIMIT 1))`, order, c.Param("id"))
	}, roles("CUSTOMER"))
}
func (s *Service) assignOrder(c echo.Context, tx pgx.Tx, order int64, requested string) error {
	var rider int64
	err := tx.QueryRow(c.Request().Context(), `SELECT r.id FROM riders r JOIN orders o ON o.id=$1 JOIN restaurants restaurant ON restaurant.id=o.restaurant_id JOIN users u ON u.id=r.user_id WHERE r.station_id=restaurant.station_id AND r.verification_status='APPROVED' AND r.availability_status='ONLINE' AND r.deleted_at IS NULL AND u.status='ACTIVE' AND u.deleted_at IS NULL AND ($2='' OR r.uuid::text=$2) AND EXISTS(SELECT 1 FROM registration_fees f WHERE f.delivery_agent_id=r.id AND f.status IN ('PAID','WAIVED')) AND NOT EXISTS(SELECT 1 FROM delivery_assignments a WHERE a.rider_id=r.id AND a.status IN ('ASSIGNED','ACCEPTED','PICKED_UP') AND a.deleted_at IS NULL) AND NOT EXISTS(SELECT 1 FROM delivery_assignments a WHERE a.order_id=o.id AND a.status IN ('ASSIGNED','ACCEPTED','PICKED_UP') AND a.deleted_at IS NULL) ORDER BY r.id FOR UPDATE OF r SKIP LOCKED LIMIT 1`, order, requested).Scan(&rider)
	if err != nil {
		return err
	}
	_, err = tx.Exec(c.Request().Context(), `INSERT INTO delivery_assignments(order_id,rider_id,status,assigned_at) VALUES($1,$2,'ASSIGNED',now())`, order, rider)
	return err
}
func pageOffset(c echo.Context) int {
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if offset < 0 {
		return 0
	}
	if offset > 1000000 {
		return 1000000
	}
	return offset
}

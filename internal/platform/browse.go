package platform

import (
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

func (s *Service) registerBrowse(api *echo.Group) {
	customer := api.Group("/customer", roles("CUSTOMER"))
	customer.GET("/finance/pricing", func(c echo.Context) error {
		return c.JSON(200, map[string]any{"delivery_charge": envAmount("DELIVERY_FEE_PAISE"), "online_enabled": s.Gateway != nil && s.Gateway.KeyID != "", "cod_enabled": true})
	})
	customer.GET("/finance/orders/:id", func(c echo.Context) error {
		tx, err := s.begin(c, "READ")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		order, _, err := s.orderAccess(c, tx, false)
		if err != nil {
			return err
		}
		return s.one(c, `SELECT jsonb_build_object('payment_status',COALESCE((SELECT CASE WHEN status='CAPTURED' AND amount_paise=(SELECT round(total_amount*100)::bigint FROM orders WHERE id=$1) THEN 'PAID' WHEN status='CAPTURED' THEN 'PENDING_REVIEW' ELSE status END FROM payments WHERE order_id=$1 ORDER BY id DESC LIMIT 1),'DUE'),'amount_paise',(SELECT round(total_amount*100)::bigint FROM orders WHERE id=$1))`, order)
	})
	customer.GET("/restaurants", func(c echo.Context) error {
		address := c.QueryParam("address_id")
		if publicID(address) != nil {
			return bad("select a saved delivery address")
		}
		return s.list(c, `SELECT `+project("restaurants", "r")+`||jsonb_build_object('cuisine','Restaurant','image_url',r.image_url,'menu',COALESCE((SELECT jsonb_agg(`+menuJSON()+`) FROM menu_items i JOIN menu_categories category ON category.id=i.category_id WHERE i.restaurant_id=r.id AND i.is_available AND i.deleted_at IS NULL AND category.is_active AND category.deleted_at IS NULL),'[]'::jsonb)) FROM restaurants r JOIN user_addresses a ON a.uuid=$1 AND a.user_id=$2 AND a.deleted_at IS NULL JOIN stations station ON station.id=r.station_id AND station.status='ACTIVE' AND station.deleted_at IS NULL WHERE r.status='APPROVED' AND r.is_open AND r.deleted_at IS NULL AND station_covers(r.station_id,a.latitude,a.longitude) ORDER BY r.name LIMIT 100`, address, person(c).ID)
	})
	customer.GET("/restaurants/:id", func(c echo.Context) error {
		if publicID(c.Param("id")) != nil || publicID(c.QueryParam("address_id")) != nil {
			return bad("restaurant and delivery address required")
		}
		return s.one(c, `SELECT `+project("restaurants", "r")+`||jsonb_build_object('menu',COALESCE((SELECT jsonb_agg(`+menuJSON()+` ORDER BY category.sort_order,i.name) FROM menu_items i JOIN menu_categories category ON category.id=i.category_id WHERE i.restaurant_id=r.id AND i.is_available AND i.deleted_at IS NULL AND category.is_active AND category.deleted_at IS NULL),'[]'::jsonb)) FROM restaurants r JOIN user_addresses a ON a.uuid=$2 AND a.user_id=$3 AND a.deleted_at IS NULL WHERE r.uuid=$1 AND r.status='APPROVED' AND r.is_open AND r.deleted_at IS NULL AND station_covers(r.station_id,a.latitude,a.longitude)`, c.Param("id"), c.QueryParam("address_id"), person(c).ID)
	})
	customer.POST("/coupons/validate", func(c echo.Context) error {
		var input struct {
			Code   string `json:"code"`
			Amount int64  `json:"amount"`
		}
		if c.Bind(&input) != nil || input.Amount <= 0 {
			return bad("coupon and order amount required")
		}
		var kind string
		var value, minimum int64
		var cap *int64
		if err := s.DB.QueryRow(c.Request().Context(), `SELECT discount_type,discount_value,min_order_amount_paise,max_discount_amount_paise FROM coupons c WHERE upper(code)=upper($1) AND is_active AND deleted_at IS NULL AND valid_from<=now() AND (valid_until IS NULL OR valid_until>now()) AND (usage_limit IS NULL OR usage_limit>(SELECT count(*) FROM coupon_usages WHERE coupon_id=c.id))`, strings.TrimSpace(input.Code)).Scan(&kind, &value, &minimum, &cap); err != nil || input.Amount < minimum {
			return bad("coupon is not eligible")
		}
		discount := value
		if kind == "PERCENTAGE" {
			discount = input.Amount * value / 100
		}
		if cap != nil {
			discount = min(discount, *cap)
		}
		discount = min(discount, input.Amount)
		return c.JSON(200, map[string]any{"code": strings.ToUpper(input.Code), "discount_amount_paise": discount, "discount": float64(discount) / 100, "valid": true})
	})
	// Address belongs to the customer and is revalidated for every cart mutation
	// and checkout; the schema intentionally does not persist an extra snapshot.
	customer.PUT("/cart/delivery-address", func(c echo.Context) error {
		var input struct {
			ID string `json:"address_id"`
		}
		if c.Bind(&input) != nil || publicID(input.ID) != nil {
			return bad("saved address required")
		}
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), `SELECT id FROM users WHERE id=$1 FOR UPDATE`, person(c).ID); err != nil {
			return err
		}
		var address int64
		if err = tx.QueryRow(c.Request().Context(), `SELECT id FROM user_addresses WHERE uuid=$1 AND user_id=$2 AND deleted_at IS NULL`, input.ID, person(c).ID).Scan(&address); err != nil {
			return bad("saved address was not found")
		}
		tag, err := tx.Exec(c.Request().Context(), `UPDATE carts cart SET status='ABANDONED' FROM restaurants r,user_addresses a WHERE cart.restaurant_id=r.id AND a.id=$2 AND cart.user_id=$1 AND cart.status='ACTIVE' AND cart.deleted_at IS NULL AND NOT station_covers(r.station_id,a.latitude,a.longitude)`, person(c).ID, address)
		if err != nil {
			return err
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return err
		}
		return c.JSON(200, map[string]any{"cart_cleared": tag.RowsAffected() > 0})
	})
	partner := api.Group("/partner", roles("RESTAURANT_OWNER", "DELIVERY_PARTNER"))
	partner.GET("/account", func(c echo.Context) error {
		return s.one(c, `SELECT `+project("users", "u")+`||jsonb_build_object('profile',CASE WHEN u.role='RESTAURANT_OWNER' THEN (SELECT `+project("restaurants", "r")+` FROM restaurants r WHERE r.owner_user_id=u.id AND r.deleted_at IS NULL ORDER BY r.id LIMIT 1) ELSE (SELECT `+project("riders", "a")+` FROM riders a WHERE a.user_id=u.id AND a.deleted_at IS NULL) END) FROM users u WHERE u.id=$1`, person(c).ID)
	})
	partner.PUT("/account", func(c echo.Context) error {
		var input struct {
			Name string `json:"full_name"`
		}
		if c.Bind(&input) != nil || !validName(input.Name, 160) {
			return bad("full name required")
		}
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), `UPDATE users SET full_name=$2 WHERE id=$1`, person(c).ID, input.Name); err != nil {
			return err
		}
		return commit(c, tx)
	})
	partner.GET("/overview", func(c echo.Context) error {
		return s.one(c, `WITH my_orders AS (SELECT o.* FROM orders o JOIN restaurants r ON r.id=o.restaurant_id WHERE ($1='RESTAURANT_OWNER' AND r.owner_user_id=$2) OR ($1='DELIVERY_PARTNER' AND EXISTS(SELECT 1 FROM delivery_assignments a JOIN riders rider ON rider.id=a.rider_id WHERE a.order_id=o.id AND rider.user_id=$2 AND a.status NOT IN ('REJECTED','CANCELLED') AND a.deleted_at IS NULL))) SELECT jsonb_build_object('total_orders',count(*),'active_orders',count(*) FILTER(WHERE status NOT IN ('DELIVERED','CANCELLED','REJECTED','REFUNDED')),'completed_orders',count(*) FILTER(WHERE status='DELIVERED'),'today_orders',count(*) FILTER(WHERE created_at::date=current_date)) FROM my_orders`, person(c).Role, person(c).ID)
	})
	partner.GET("/finance/summary", func(c echo.Context) error {
		if person(c).Role == "RESTAURANT_OWNER" {
			return s.one(c, `SELECT jsonb_build_object('currency','INR','gross',COALESCE(sum(e.gross_amount_paise),0),'commission',COALESCE(sum(e.fee_amount_paise),0),'adjustments',COALESCE(sum(e.adjustment_amount_paise),0),'delivery_earnings',0,'paid',COALESCE((SELECT sum(p.amount_paise) FROM restaurant_payouts p JOIN restaurants r ON r.id=p.restaurant_id WHERE r.owner_user_id=$1 AND p.status='PAID'),0),'pending',COALESCE(sum(e.net_amount_paise) FILTER(WHERE e.status IN ('PENDING','AVAILABLE','HELD')),0),'next_settlement_at',NULL) FROM restaurant_earnings e JOIN restaurants r ON r.id=e.restaurant_id WHERE r.owner_user_id=$1`, person(c).ID)
		}
		return s.one(c, `SELECT jsonb_build_object('currency','INR','gross',0,'commission',0,'adjustments',COALESCE(sum(e.adjustment_amount_paise),0),'delivery_earnings',COALESCE(sum(e.net_amount_paise),0),'paid',COALESCE((SELECT sum(p.amount_paise) FROM delivery_agent_payouts p JOIN riders r ON r.id=p.delivery_agent_id WHERE r.user_id=$1 AND p.status='PAID'),0),'pending',COALESCE(sum(e.net_amount_paise) FILTER(WHERE e.status IN ('PENDING','AVAILABLE','HELD')),0),'next_settlement_at',NULL) FROM delivery_agent_earnings e JOIN riders r ON r.id=e.delivery_agent_id WHERE r.user_id=$1`, person(c).ID)
	})
	partner.GET("/finance/ledger", func(c echo.Context) error {
		return s.list(c, `SELECT `+project("financial_transactions", "t")+`||jsonb_build_object('amount',CASE t.direction WHEN 'DEBIT' THEN t.amount_paise ELSE -t.amount_paise END,'kind',t.transaction_type,'component',t.account_code,'reference',t.event_key,'reason',t.transaction_type) FROM financial_transactions t LEFT JOIN restaurant_earnings e ON e.id=t.restaurant_earning_id LEFT JOIN restaurants r ON r.id=e.restaurant_id LEFT JOIN delivery_agent_earnings ae ON ae.id=t.agent_earning_id LEFT JOIN riders agent ON agent.id=ae.delivery_agent_id WHERE ($1='RESTAURANT_OWNER' AND r.owner_user_id=$2) OR ($1='DELIVERY_PARTNER' AND agent.user_id=$2) ORDER BY t.created_at DESC LIMIT 100 OFFSET $3`, person(c).Role, person(c).ID, pageOffset(c))
	})
	partner.GET("/finance/settlements", func(c echo.Context) error {
		if person(c).Role == "RESTAURANT_OWNER" {
			return s.list(c, `SELECT `+project("restaurant_payouts", "p")+`||jsonb_build_object('amount',p.amount_paise,'cutoff',p.created_at,'evidence',p.provider_transfer_id) FROM restaurant_payouts p JOIN restaurants r ON r.id=p.restaurant_id WHERE r.owner_user_id=$1 ORDER BY p.created_at DESC LIMIT 100 OFFSET $2`, person(c).ID, pageOffset(c))
		}
		return s.list(c, `SELECT `+project("delivery_agent_payouts", "p")+`||jsonb_build_object('amount',p.amount_paise,'cutoff',p.created_at,'evidence',p.provider_transfer_id) FROM delivery_agent_payouts p JOIN riders r ON r.id=p.delivery_agent_id WHERE r.user_id=$1 ORDER BY p.created_at DESC LIMIT 100 OFFSET $2`, person(c).ID, pageOffset(c))
	})
}
func menuJSON() string {
	return project("menu_items", "i") + `||jsonb_build_object('price',i.base_price,'base_price_paise',round(i.base_price*100)::bigint,'category',(SELECT name FROM menu_categories WHERE id=i.category_id),'variants',COALESCE((SELECT jsonb_agg(` + project("item_variants", "v") + `) FROM item_variants v WHERE v.menu_item_id=i.id AND v.is_available AND v.deleted_at IS NULL),'[]'::jsonb))`
}

func parsePositive(value string, maximum int64) (int64, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 1 || n > maximum {
		return 0, bad("positive amount required")
	}
	return n, nil
}

package platform

import (
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"os"
	"strconv"
	"strings"
)

func envAmount(name string) int64 {
	v, err := strconv.ParseInt(os.Getenv(name), 10, 64)
	if err != nil || v < 0 || v > 100000000 {
		return 0
	}
	return v
}
func (s *Service) registerCart(api *echo.Group) {
	customer := api.Group("/customer", roles("CUSTOMER"))
	customer.GET("/cart", func(c echo.Context) error {
		return s.one(c, `SELECT COALESCE((SELECT jsonb_build_object('id',cart.uuid,'restaurant_id',r.uuid,'currency','INR','subtotal',COALESCE((SELECT sum(round((i.base_price+COALESCE(v.price_delta,0))*100)::bigint*ci.quantity) FROM cart_items ci JOIN menu_items i ON i.id=ci.menu_item_id LEFT JOIN item_variants v ON v.id=ci.variant_id WHERE ci.cart_id=cart.id AND ci.deleted_at IS NULL),0),'items',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',ci.uuid,'menu_item_id',i.uuid,'item_id',i.uuid,'variant_id',v.uuid,'name',i.name,'variant_name',v.name,'quantity',ci.quantity,'special_instructions',ci.special_instructions,'unit_price',round((i.base_price+COALESCE(v.price_delta,0))*100)::bigint,'currency','INR')) FROM cart_items ci JOIN menu_items i ON i.id=ci.menu_item_id LEFT JOIN item_variants v ON v.id=ci.variant_id WHERE ci.cart_id=cart.id AND ci.deleted_at IS NULL),'[]'::jsonb)) FROM carts cart JOIN restaurants r ON r.id=cart.restaurant_id WHERE cart.user_id=$1 AND cart.status='ACTIVE' AND cart.deleted_at IS NULL),'{"items":[],"subtotal":0,"currency":"INR"}'::jsonb)`, person(c).ID)
	})
	customer.POST("/cart/items", func(c echo.Context) error {
		var input struct {
			Item         string  `json:"menu_item_id"`
			LegacyItem   string  `json:"item_id"`
			Variant      *string `json:"variant_id"`
			Quantity     int     `json:"quantity"`
			Instructions string  `json:"special_instructions"`
			Address      string  `json:"address_id"`
		}
		if c.Bind(&input) != nil {
			return bad("invalid cart line")
		}
		if input.Item == "" {
			input.Item = input.LegacyItem
		}
		if publicID(input.Item) != nil || publicID(input.Address) != nil || input.Quantity < 1 || input.Quantity > 99 || len(input.Instructions) > 2000 {
			return bad("item, address and quantity 1 to 99 required")
		}
		if input.Variant != nil && publicID(*input.Variant) != nil {
			return bad("invalid variant UUID")
		}
		tx, err := s.begin(c, "CREATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), `SELECT id FROM users WHERE id=$1 FOR UPDATE`, person(c).ID); err != nil {
			return err
		}
		var item, restaurant int64
		if err = tx.QueryRow(c.Request().Context(), `SELECT i.id,i.restaurant_id FROM menu_items i JOIN restaurants r ON r.id=i.restaurant_id JOIN user_addresses a ON a.uuid=$2 AND a.user_id=$3 AND a.deleted_at IS NULL WHERE i.uuid=$1 AND i.is_available AND i.deleted_at IS NULL AND r.is_open AND r.status='APPROVED' AND r.deleted_at IS NULL AND station_covers(r.station_id,a.latitude,a.longitude)`, input.Item, input.Address, person(c).ID).Scan(&item, &restaurant); err != nil {
			return bad("item or restaurant unavailable for the selected address")
		}
		var variant *int64
		if input.Variant != nil {
			var id int64
			if err = tx.QueryRow(c.Request().Context(), `SELECT id FROM item_variants WHERE uuid=$1 AND menu_item_id=$2 AND is_available AND deleted_at IS NULL`, *input.Variant, item).Scan(&id); err != nil {
				return bad("variant unavailable or belongs to another item")
			}
			variant = &id
		}
		var cart, currentRestaurant int64
		err = tx.QueryRow(c.Request().Context(), `SELECT id,restaurant_id FROM carts WHERE user_id=$1 AND status='ACTIVE' AND deleted_at IS NULL FOR UPDATE`, person(c).ID).Scan(&cart, &currentRestaurant)
		if err == pgx.ErrNoRows {
			err = tx.QueryRow(c.Request().Context(), `INSERT INTO carts(user_id,restaurant_id,status,expires_at) VALUES($1,$2,'ACTIVE',now()+interval '24 hours') RETURNING id`, person(c).ID, restaurant).Scan(&cart)
		} else if err == nil && currentRestaurant != restaurant {
			return bad("clear the current cart before switching restaurants")
		}
		if err != nil {
			return unavailable(err)
		}
		if _, err = tx.Exec(c.Request().Context(), `INSERT INTO cart_items(cart_id,menu_item_id,variant_id,quantity,special_instructions) VALUES($1,$2,$3,$4,NULLIF($5,'')) ON CONFLICT(cart_id,menu_item_id,variant_id) WHERE deleted_at IS NULL DO UPDATE SET quantity=cart_items.quantity+EXCLUDED.quantity,special_instructions=EXCLUDED.special_instructions`, cart, item, variant, input.Quantity, input.Instructions); err != nil {
			return bad("cart quantity cannot exceed 99")
		}
		return commit(c, tx)
	})
	customer.PATCH("/cart/items/:id", func(c echo.Context) error {
		if publicID(c.Param("id")) != nil {
			return bad("cart-line UUID required")
		}
		var input struct {
			Quantity int `json:"quantity"`
		}
		if c.Bind(&input) != nil || input.Quantity < 0 || input.Quantity > 99 {
			return bad("quantity must be 0 to 99")
		}
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		tag, err := tx.Exec(c.Request().Context(), `UPDATE cart_items ci SET quantity=GREATEST($3,1),deleted_at=CASE WHEN $3=0 THEN now() ELSE NULL END FROM carts cart WHERE ci.cart_id=cart.id AND cart.user_id=$2 AND cart.status='ACTIVE' AND ci.uuid=$1 AND ci.deleted_at IS NULL`, c.Param("id"), person(c).ID, input.Quantity)
		if err != nil {
			return unavailable(err)
		}
		if tag.RowsAffected() != 1 {
			return echo.NewHTTPError(404, "cart line not found")
		}
		return commit(c, tx)
	})
	customer.DELETE("/cart", func(c echo.Context) error {
		tx, err := s.begin(c, "DELETE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), `UPDATE carts SET status='ABANDONED',deleted_at=now() WHERE user_id=$1 AND status='ACTIVE'`, person(c).ID); err != nil {
			return err
		}
		return commit(c, tx)
	})
	customer.POST("/checkout", s.checkout)
}
func (s *Service) checkout(c echo.Context) error {
	var input struct {
		Address string `json:"address_id"`
		Key     string `json:"idempotency_key"`
		Method  string `json:"payment_method"`
		Coupon  string `json:"coupon_code"`
	}
	if c.Bind(&input) != nil || publicID(input.Address) != nil || len(input.Key) < 8 || len(input.Key) > 64 {
		return bad("saved address and stable checkout key required")
	}
	method := "PAY_ON_DELIVERY"
	if input.Method == "RAZORPAY" {
		method = "RAZORPAY"
	} else if input.Method != "" && input.Method != "CASH_ON_DELIVERY" && input.Method != "PAY_ON_DELIVERY" {
		return bad("unsupported payment method")
	}
	number := "Y" + strings.ToUpper(hashToken(person(c).UUID + "/" + input.Key)[:26])
	tx, err := s.begin(c, "CREATE")
	if err != nil {
		return err
	}
	defer tx.Rollback(c.Request().Context())
	if _, err = tx.Exec(c.Request().Context(), `SELECT id FROM users WHERE id=$1 FOR UPDATE`, person(c).ID); err != nil {
		return err
	}
	var existing json.RawMessage
	err = tx.QueryRow(c.Request().Context(), `SELECT jsonb_build_object('id',o.uuid,'status',o.status,'subtotal',round(o.total_amount*100)::bigint,'currency',o.currency) FROM orders o JOIN user_addresses a ON a.id=o.address_id JOIN payment_intents intent ON intent.order_id=o.id WHERE o.order_number=$1 AND o.user_id=$2 AND a.uuid=$3 AND intent.method=$4`, number, person(c).ID, input.Address, method).Scan(&existing)
	if err == nil {
		return c.JSON(200, existing)
	}
	if err != pgx.ErrNoRows {
		return err
	}
	var address int64
	if err = tx.QueryRow(c.Request().Context(), `SELECT id FROM user_addresses WHERE uuid=$1 AND user_id=$2 AND deleted_at IS NULL`, input.Address, person(c).ID).Scan(&address); err != nil {
		return bad("delivery address not found")
	}
	var cart, restaurant int64
	if err = tx.QueryRow(c.Request().Context(), `SELECT cart.id,cart.restaurant_id FROM carts cart JOIN restaurants r ON r.id=cart.restaurant_id JOIN user_addresses a ON a.id=$2 WHERE cart.user_id=$1 AND cart.status='ACTIVE' AND cart.deleted_at IS NULL AND (cart.expires_at IS NULL OR cart.expires_at>now()) AND r.is_open AND r.status='APPROVED' AND r.deleted_at IS NULL AND station_covers(r.station_id,a.latitude,a.longitude) FOR UPDATE OF cart`, person(c).ID, address).Scan(&cart, &restaurant); err != nil {
		return bad("active serviceable restaurant cart required")
	}
	rows, err := tx.Query(c.Request().Context(), `SELECT ci.menu_item_id,ci.variant_id,ci.quantity,round((i.base_price+COALESCE(v.price_delta,0))*100)::bigint,COALESCE(ci.special_instructions,'') FROM cart_items ci JOIN menu_items i ON i.id=ci.menu_item_id LEFT JOIN item_variants v ON v.id=ci.variant_id WHERE ci.cart_id=$1 AND ci.deleted_at IS NULL AND i.restaurant_id=$2 AND i.is_available AND i.deleted_at IS NULL AND (ci.variant_id IS NULL OR (v.menu_item_id=i.id AND v.is_available AND v.deleted_at IS NULL)) FOR SHARE OF ci,i`, cart, restaurant)
	if err != nil {
		return err
	}
	type line struct {
		item         int64
		variant      *int64
		quantity     int
		unit         int64
		instructions string
	}
	lines := []line{}
	var subtotal int64
	for rows.Next() {
		var l line
		if err = rows.Scan(&l.item, &l.variant, &l.quantity, &l.unit, &l.instructions); err != nil {
			rows.Close()
			return err
		}
		if l.unit < 0 {
			rows.Close()
			return bad("invalid menu pricing")
		}
		subtotal += l.unit * int64(l.quantity)
		lines = append(lines, l)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow(c.Request().Context(), `SELECT count(*) FROM cart_items WHERE cart_id=$1 AND deleted_at IS NULL`, cart).Scan(&count); err != nil {
		return err
	}
	if len(lines) == 0 || len(lines) != count || subtotal <= 0 || subtotal > 1000000000 {
		return bad("cart contains unavailable selections or an invalid total")
	}
	var coupon *int64
	var discount int64
	if code := strings.TrimSpace(input.Coupon); code != "" {
		var id, value, minimum int64
		var kind string
		var cap *int64
		var limit *int
		if err = tx.QueryRow(c.Request().Context(), `SELECT id,discount_type,discount_value,min_order_amount_paise,max_discount_amount_paise,usage_limit FROM coupons WHERE upper(code)=upper($1) AND is_active AND deleted_at IS NULL AND valid_from<=now() AND (valid_until IS NULL OR valid_until>now()) FOR UPDATE`, code).Scan(&id, &kind, &value, &minimum, &cap, &limit); err != nil || subtotal < minimum {
			return bad("coupon is not eligible")
		}
		var used int64
		if err = tx.QueryRow(c.Request().Context(), `SELECT count(*) FROM coupon_usages WHERE coupon_id=$1`, id).Scan(&used); err != nil {
			return err
		}
		if limit != nil && used >= int64(*limit) {
			return bad("coupon usage limit reached")
		}
		discount = value
		if kind == "PERCENTAGE" {
			discount = subtotal * value / 100
		}
		if cap != nil {
			discount = min(discount, *cap)
		}
		discount = min(discount, subtotal)
		coupon = &id
	}
	delivery := envAmount("DELIVERY_FEE_PAISE")
	total := subtotal + delivery - discount
	if total <= 0 {
		return bad("order total must be positive")
	}
	status := "CONFIRMED"
	if method == "RAZORPAY" {
		status = "PENDING_PAYMENT"
	}
	var order int64
	var uid string
	if err = tx.QueryRow(c.Request().Context(), `INSERT INTO orders(order_number,user_id,restaurant_id,address_id,status,subtotal,delivery_fee,tax_amount,discount_amount,total_amount,currency,placed_at) VALUES($1,$2,$3,$4,$5,$6::bigint/100.0,$7::bigint/100.0,0,$8::bigint/100.0,$9::bigint/100.0,'INR',now()) RETURNING id,uuid`, number, person(c).ID, restaurant, address, status, subtotal, delivery, discount, total).Scan(&order, &uid); err != nil {
		return unavailable(err)
	}
	for _, l := range lines {
		if _, err = tx.Exec(c.Request().Context(), `INSERT INTO order_items(order_id,menu_item_id,variant_id,quantity,unit_price,special_instructions) VALUES($1,$2,$3,$4,$5::bigint/100.0,NULLIF($6,''))`, order, l.item, l.variant, l.quantity, l.unit, l.instructions); err != nil {
			return err
		}
	}
	if coupon != nil {
		if _, err = tx.Exec(c.Request().Context(), `INSERT INTO coupon_usages(coupon_id,user_id,order_id,discount_amount_paise) VALUES($1,$2,$3,$4)`, *coupon, person(c).ID, order, discount); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(c.Request().Context(), `INSERT INTO payment_intents(order_id,purpose,method,stage,amount_paise,currency,status,idempotency_key) VALUES($1,'ORDER',$2,'CHECKOUT',$3,'INR','CREATED',$4)`, order, method, total, "checkout/"+person(c).UUID+"/"+input.Key); err != nil {
		return err
	}
	if _, err = tx.Exec(c.Request().Context(), `UPDATE carts SET status='CONVERTED' WHERE id=$1`, cart); err != nil {
		return err
	}
	if err = tx.Commit(c.Request().Context()); err != nil {
		return unavailable(err)
	}
	return c.JSON(201, map[string]any{"id": uid, "status": status, "subtotal": total, "currency": "INR"})
}

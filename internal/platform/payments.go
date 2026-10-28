package platform

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

func (s *Service) intentAccess(c echo.Context, tx pgx.Tx, uid string) (int64, error) {
	if err := publicID(uid); err != nil {
		return 0, err
	}
	var id int64
	err := tx.QueryRow(c.Request().Context(), `SELECT i.id FROM payment_intents i LEFT JOIN orders o ON o.id=i.order_id LEFT JOIN restaurants r ON r.id=i.restaurant_id LEFT JOIN riders agent ON agent.id=i.delivery_agent_id WHERE i.uuid=$1 AND i.deleted_at IS NULL AND (($2='CUSTOMER' AND o.user_id=$3) OR ($2='RESTAURANT_OWNER' AND r.owner_user_id=$3) OR ($2='DELIVERY_PARTNER' AND agent.user_id=$3) OR $2='ADMIN') FOR UPDATE OF i`, uid, person(c).Role, person(c).ID).Scan(&id)
	if err != nil {
		return 0, unavailable(err)
	}
	return id, nil
}
func (s *Service) registerPayments(api *echo.Group) {
	api.GET("/partner/finance/registration", func(c echo.Context) error {
		return s.one(c, `SELECT jsonb_build_object('amount_paise',f.amount_paise,'currency',f.currency,'payment_status',f.status,'refundable',false,'terms','Non-refundable registration fee. Payment does not imply approval.') FROM registration_fees f LEFT JOIN restaurants r ON r.id=f.restaurant_id LEFT JOIN riders a ON a.id=f.delivery_agent_id WHERE r.owner_user_id=$1 OR a.user_id=$1`, person(c).ID)
	}, roles("RESTAURANT_OWNER", "DELIVERY_PARTNER"))
	registration := func(c echo.Context) error {
		tx, err := s.begin(c, "PAY")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		var fee, amount int64
		var restaurant, agent, intent *int64
		var status string
		if err = tx.QueryRow(c.Request().Context(), `SELECT f.id,f.amount_paise,f.restaurant_id,f.delivery_agent_id,f.payment_intent_id,f.status FROM registration_fees f LEFT JOIN restaurants r ON r.id=f.restaurant_id LEFT JOIN riders a ON a.id=f.delivery_agent_id WHERE r.owner_user_id=$1 OR a.user_id=$1 FOR UPDATE OF f`, person(c).ID).Scan(&fee, &amount, &restaurant, &agent, &intent, &status); err != nil {
			return unavailable(err)
		}
		if status == "PAID" || status == "WAIVED" {
			return c.JSON(200, map[string]string{"status": status})
		}
		purpose := "RESTAURANT_REGISTRATION"
		if agent != nil {
			purpose = "AGENT_REGISTRATION"
		}
		var uid string
		if intent == nil {
			var id int64
			if err = tx.QueryRow(c.Request().Context(), `INSERT INTO payment_intents(restaurant_id,delivery_agent_id,purpose,method,stage,amount_paise,currency,status,idempotency_key) VALUES($1,$2,$3,'RAZORPAY','REGISTRATION',$4,'INR','CREATED',$5) RETURNING id,uuid`, restaurant, agent, purpose, amount, fmt.Sprintf("registration/%d", fee)).Scan(&id, &uid); err != nil {
				return err
			}
			if _, err = tx.Exec(c.Request().Context(), `UPDATE registration_fees SET payment_intent_id=$2 WHERE id=$1`, fee, id); err != nil {
				return err
			}
		} else {
			if err = tx.QueryRow(c.Request().Context(), `SELECT uuid FROM payment_intents WHERE id=$1`, *intent).Scan(&uid); err != nil {
				return err
			}
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return err
		}
		c.SetParamNames("id")
		c.SetParamValues(uid)
		return s.createProviderOrder(c)
	}
	api.POST("/restaurant/onboarding-payment", registration, roles("RESTAURANT_OWNER"))
	api.POST("/delivery/onboarding-payment", registration, roles("DELIVERY_PARTNER"))
	api.POST("/customer/orders/:id/payment-intent", s.onlineCheckout, roles("CUSTOMER"))
	api.POST("/customer/finance/orders/:id/checkout", s.onlineCheckout, roles("CUSTOMER"))

	api.GET("/payments/intents/:id", func(c echo.Context) error {
		tx, err := s.begin(c, "READ")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		id, err := s.intentAccess(c, tx, c.Param("id"))
		if err != nil {
			return err
		}
		return s.one(c, `SELECT `+project("payment_intents", "t")+` FROM payment_intents t WHERE id=$1`, id)
	})
	api.POST("/payments/intents/:id/order", s.createProviderOrder)
	api.POST("/payments/intents/:id/verify", func(c echo.Context) error {
		var input struct {
			Payment   string `json:"razorpay_payment_id"`
			Signature string `json:"razorpay_signature"`
		}
		if c.Bind(&input) != nil || s.Gateway == nil {
			return bad("payment reference and signature required")
		}
		tx, err := s.begin(c, "PAY")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		id, err := s.intentAccess(c, tx, c.Param("id"))
		if err != nil {
			return err
		}
		var order string
		if err = tx.QueryRow(c.Request().Context(), `SELECT provider_order_id FROM payment_intents WHERE id=$1`, id).Scan(&order); err != nil {
			return bad("provider order required")
		}
		if !signature(s.Gateway.Secret, order+"|"+input.Payment, input.Signature) {
			return bad("payment signature is invalid")
		}
		p, err := s.Gateway.payment(c.Request().Context(), input.Payment)
		if err != nil {
			return echo.NewHTTPError(503, "payment verification unavailable")
		}
		if p.OrderID != order || p.ID != input.Payment {
			return bad("provider payment does not match this checkout")
		}
		if err = s.capture(c, tx, p); err != nil {
			return err
		}
		return commit(c, tx)
	})
}

func (s *Service) capture(c echo.Context, tx pgx.Tx, p providerPayment) error {
	if p.Status != "captured" || p.Currency != "INR" || p.Amount <= 0 {
		return bad("a captured INR payment is required")
	}
	var id, amount int64
	var order, restaurant, agent *int64
	var status string
	if err := tx.QueryRow(c.Request().Context(), `SELECT id,amount_paise,order_id,restaurant_id,delivery_agent_id,status FROM payment_intents WHERE provider_order_id=$1 FOR UPDATE`, p.OrderID).Scan(&id, &amount, &order, &restaurant, &agent, &status); err != nil {
		return bad("payment intent was not found")
	}
	if amount != p.Amount {
		return bad("payment amount differs from the assessed amount")
	}
	var existing string
	if err := tx.QueryRow(c.Request().Context(), `SELECT provider_payment_id FROM payments WHERE payment_intent_id=$1 AND status IN ('CAPTURED','REFUNDED')`, id).Scan(&existing); err == nil {
		if existing == p.ID {
			return nil
		}
		return bad("another payment is already recorded")
	} else if err != pgx.ErrNoRows {
		return err
	}
	if status == "CANCELLED" {
		return bad("cancelled intent requires payment reconciliation")
	}
	if order != nil {
		var state string
		if err := tx.QueryRow(c.Request().Context(), `SELECT status FROM orders WHERE id=$1 FOR UPDATE`, *order).Scan(&state); err != nil {
			return err
		}
		if state == "CANCELLED" || state == "REJECTED" {
			return bad("late capture requires refund reconciliation")
		}
	}
	method := strings.ToUpper(p.Method)
	if method != "UPI" && method != "CARD" && method != "NETBANKING" {
		method = "OTHER"
	}
	var payment int64
	if err := tx.QueryRow(c.Request().Context(), `INSERT INTO payments(payment_intent_id,order_id,provider,provider_payment_id,method,amount_paise,currency,status,received_by,captured_at) VALUES($1,$2,'RAZORPAY',$3,$4,$5,'INR','CAPTURED','YEMBER',now()) RETURNING id`, id, order, p.ID, method, amount).Scan(&payment); err != nil {
		return unavailable(err)
	}
	if _, err := tx.Exec(c.Request().Context(), `UPDATE payment_intents SET status='SUCCEEDED' WHERE id=$1`, id); err != nil {
		return err
	}
	ref := links{Order: order, Payment: ptr(payment)}
	credit := "ORDER_RECEIPTS"
	if order == nil {
		var fee int64
		if err := tx.QueryRow(c.Request().Context(), `UPDATE registration_fees SET status='PAID',paid_at=now() WHERE payment_intent_id=$1 AND status='DUE' RETURNING id`, id).Scan(&fee); err != nil {
			return err
		}
		ref.RegistrationFee = ptr(fee)
		credit = "REGISTRATION_FEE_REVENUE"
	}
	if err := post(c, tx, "capture/"+p.ID, "COLLECTION", ref, posting{"YEMBER_BANK", amount}, posting{credit, -amount}); err != nil {
		return err
	}
	if order != nil {
		if _, err := tx.Exec(c.Request().Context(), `UPDATE orders SET status='CONFIRMED' WHERE id=$1 AND status='PENDING_PAYMENT'`, *order); err != nil {
			return err
		}
		return s.recognize(c, tx, *order)
	}
	return nil
}
func (s *Service) registerWebhook(api *echo.Group) {
	api.POST("/payments/webhooks/razorpay", func(c echo.Context) error {
		raw, err := io.ReadAll(io.LimitReader(c.Request().Body, (1<<20)+1))
		if err != nil || len(raw) > 1<<20 {
			return bad("invalid webhook body")
		}
		if s.Gateway == nil || !signature(s.Gateway.WebhookSecret, string(raw), c.Request().Header.Get("X-Razorpay-Signature")) {
			return echo.NewHTTPError(401, "invalid webhook signature")
		}
		eventID := c.Request().Header.Get("X-Razorpay-Event-Id")
		if eventID == "" || len(eventID) > 160 {
			return bad("provider event ID required")
		}
		var event struct {
			Event   string `json:"event"`
			Payload struct {
				Payment struct {
					Entity providerPayment `json:"entity"`
				} `json:"payment"`
				Refund struct {
					Entity providerRefund `json:"entity"`
				} `json:"refund"`
			} `json:"payload"`
		}
		if json.Unmarshal(raw, &event) != nil {
			return bad("invalid webhook event")
		}
		// Preserve the signed provider event for reconciliation. This protected
		// inbox payload is excluded from API projections and activity snapshots.
		var payload map[string]any
		if json.Unmarshal(raw, &payload) != nil || payload == nil || event.Event == "" || len(event.Event) > 100 {
			return bad("invalid webhook event")
		}
		payload["_yember_digest"] = hashToken(string(raw))
		storedPayload, _ := json.Marshal(payload)
		_, err = s.DB.Exec(c.Request().Context(), `INSERT INTO payment_webhook_events(provider,provider_event_id,event_type,payload_json,signature_valid,processing_status) VALUES('RAZORPAY',$1,$2,$3::jsonb,true,'RECEIVED') ON CONFLICT(provider,provider_event_id) DO NOTHING`, eventID, event.Event, string(storedPayload))
		if err != nil {
			return err
		}
		tx, err := s.DB.Begin(c.Request().Context())
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		var id int64
		var status, digest string
		if err = tx.QueryRow(c.Request().Context(), `SELECT id,processing_status,payload_json->>'_yember_digest' FROM payment_webhook_events WHERE provider='RAZORPAY' AND provider_event_id=$1 FOR UPDATE`, eventID).Scan(&id, &status, &digest); err != nil {
			return err
		}
		if digest != hashToken(string(raw)) {
			return bad("event ID was reused with another payload")
		}
		if status == "PROCESSED" {
			return c.NoContent(204)
		}
		if event.Event == "payment.captured" {
			p, err := s.Gateway.payment(c.Request().Context(), event.Payload.Payment.Entity.ID)
			if err != nil {
				return echo.NewHTTPError(503, "provider verification unavailable")
			}
			if err = s.capture(c, tx, p); err != nil {
				return err
			}
		} else if event.Event == "refund.processed" || event.Event == "refund.failed" {
			providerID := event.Payload.Refund.Entity.ID
			if !strings.HasPrefix(providerID, "rfnd_") || len(providerID) > 120 {
				return bad("invalid refund reference")
			}
			var result providerRefund
			if err = s.Gateway.request(c.Request().Context(), "GET", "/refunds/"+providerID, nil, &result); err != nil {
				return echo.NewHTTPError(503, "provider verification unavailable")
			}
			if err = s.completeRefund(c, tx, result); err != nil {
				return err
			}
		} else {
			if _, err = tx.Exec(c.Request().Context(), `UPDATE payment_webhook_events SET processing_status='IGNORED',processed_at=now() WHERE id=$1`, id); err != nil {
				return err
			}
			return commit(c, tx)
		}
		if _, err = tx.Exec(c.Request().Context(), `UPDATE payment_webhook_events SET processing_status='PROCESSED',processed_at=now() WHERE id=$1`, id); err != nil {
			return err
		}
		return commit(c, tx)
	})
}

func (s *Service) createProviderOrder(c echo.Context) error {
	if s.Gateway == nil || s.Gateway.KeyID == "" || s.Gateway.Secret == "" {
		return echo.NewHTTPError(503, "Razorpay is not configured")
	}
	tx, err := s.begin(c, "PAY")
	if err != nil {
		return err
	}
	defer tx.Rollback(c.Request().Context())
	id, err := s.intentAccess(c, tx, c.Param("id"))
	if err != nil {
		return err
	}
	var amount int64
	var status, method string
	var provider *string
	if err = tx.QueryRow(c.Request().Context(), `SELECT amount_paise,status,method,provider_order_id FROM payment_intents WHERE id=$1`, id).Scan(&amount, &status, &method, &provider); err != nil {
		return err
	}
	if method != "RAZORPAY" {
		return bad("online payment intent required")
	}
	if provider != nil {
		return s.checkoutResponse(c, c.Param("id"), *provider, amount, status)
	}
	if status != "CREATED" {
		return bad("provider response requires reconciliation; do not create another payment")
	}
	if _, err = tx.Exec(c.Request().Context(), `UPDATE payment_intents SET status='CREATING' WHERE id=$1`, id); err != nil {
		return err
	}
	if err = tx.Commit(c.Request().Context()); err != nil {
		return err
	}
	var result struct {
		ID       string `json:"id"`
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
	}
	err = s.Gateway.request(c.Request().Context(), "POST", "/orders", map[string]any{"amount": amount, "currency": "INR", "receipt": c.Param("id"), "partial_payment": false, "notes": map[string]string{"payment_intent_uuid": c.Param("id")}}, &result)
	if err != nil || !strings.HasPrefix(result.ID, "order_") || result.Amount != amount || result.Currency != "INR" {
		_, _ = s.DB.Exec(c.Request().Context(), `UPDATE payment_intents SET status='UNKNOWN' WHERE id=$1 AND status='CREATING'`, id)
		return echo.NewHTTPError(503, "provider response requires reconciliation; retrying will not create another charge")
	}
	if _, err = s.DB.Exec(c.Request().Context(), `UPDATE payment_intents SET provider_order_id=$2,status='PENDING' WHERE id=$1 AND status='CREATING'`, id, result.ID); err != nil {
		return err
	}
	return s.checkoutResponse(c, c.Param("id"), result.ID, amount, "PENDING")
}

func (s *Service) onlineCheckout(c echo.Context) error {
	if s.Gateway == nil || s.Gateway.KeyID == "" || s.Gateway.Secret == "" {
		return echo.NewHTTPError(503, "Razorpay is not configured")
	}
	tx, err := s.begin(c, "PAY")
	if err != nil {
		return err
	}
	defer tx.Rollback(c.Request().Context())
	order, status, err := s.orderAccess(c, tx, true)
	if err != nil {
		return err
	}
	if status == "CANCELLED" || status == "REJECTED" || status == "REFUNDED" {
		return bad("order no longer accepts payment")
	}
	var paid bool
	if err = tx.QueryRow(c.Request().Context(), `SELECT EXISTS(SELECT 1 FROM payments WHERE order_id=$1 AND status='CAPTURED')`, order).Scan(&paid); err != nil {
		return err
	}
	if paid {
		return bad("order payment is already collected")
	}
	var uid string
	if err = tx.QueryRow(c.Request().Context(), `SELECT uuid FROM payment_intents WHERE order_id=$1 AND method='RAZORPAY' AND status NOT IN ('CANCELLED','FAILED') ORDER BY id DESC LIMIT 1`, order).Scan(&uid); err == pgx.ErrNoRows {
		if _, err = tx.Exec(c.Request().Context(), `UPDATE payment_intents SET status='CANCELLED' WHERE order_id=$1 AND method='PAY_ON_DELIVERY' AND status='CREATED'`, order); err != nil {
			return err
		}
		if err = tx.QueryRow(c.Request().Context(), `INSERT INTO payment_intents(order_id,purpose,method,stage,amount_paise,currency,status,idempotency_key) SELECT id,'ORDER','RAZORPAY',CASE WHEN status='PENDING_PAYMENT' THEN 'CHECKOUT' ELSE 'DELIVERY' END,round(total_amount*100)::bigint,'INR','CREATED',$2 FROM orders WHERE id=$1 RETURNING uuid`, order, fmt.Sprintf("online/%d", order)).Scan(&uid); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err = tx.Commit(c.Request().Context()); err != nil {
		return err
	}
	c.SetParamNames("id")
	c.SetParamValues(uid)
	return s.createProviderOrder(c)
}

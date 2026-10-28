package platform

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

type providerRefund struct {
	ID        string `json:"id"`
	PaymentID string `json:"payment_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Status    string `json:"status"`
	Receipt   string `json:"receipt"`
}

func (s *Service) registerRefunds(api *echo.Group) {
	admin := api.Group("/admin", roles("ADMIN"))
	admin.POST("/payments/:id/refunds", func(c echo.Context) error {
		if publicID(c.Param("id")) != nil {
			return bad("payment UUID required")
		}
		if s.Gateway == nil || s.Gateway.KeyID == "" || s.Gateway.Secret == "" {
			return echo.NewHTTPError(503, "Razorpay is not configured")
		}
		var input struct {
			Amount int64  `json:"amount_paise"`
			Reason string `json:"reason"`
			Key    string `json:"idempotency_key"`
		}
		if c.Bind(&input) != nil || input.Amount <= 0 || !validName(input.Reason, 255) || len(input.Key) < 8 || len(input.Key) > 64 {
			return bad("positive refund amount, reason and stable idempotency key required")
		}
		tx, err := s.begin(c, "REFUND")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		var payment, order, amount int64
		var provider string
		if err = tx.QueryRow(c.Request().Context(), `SELECT id,order_id,amount_paise,provider_payment_id FROM payments WHERE uuid=$1 AND provider='RAZORPAY' AND status IN ('CAPTURED','REFUNDED') AND order_id IS NOT NULL FOR UPDATE`, c.Param("id")).Scan(&payment, &order, &amount, &provider); err != nil {
			return bad("captured online order payment required; registration fees are non-refundable")
		}
		uid := uuid.NewSHA1(uuid.NameSpaceOID, []byte(c.Param("id")+"/refund/"+input.Key)).String()
		var priorAmount int64
		var priorReason, priorStatus string
		if err = tx.QueryRow(c.Request().Context(), `SELECT amount_paise,reason,status FROM refunds WHERE uuid=$1`, uid).Scan(&priorAmount, &priorReason, &priorStatus); err == nil {
			if priorAmount != input.Amount || priorReason != input.Reason {
				return bad("idempotency key was already used with other refund terms")
			}
			return c.JSON(200, map[string]string{"id": uid, "status": priorStatus})
		} else if err != pgx.ErrNoRows {
			return err
		}
		var reserved int64
		if err = tx.QueryRow(c.Request().Context(), `SELECT COALESCE(sum(amount_paise),0)::bigint FROM refunds WHERE payment_id=$1 AND status<>'FAILED'`, payment).Scan(&reserved); err != nil {
			return err
		}
		if input.Amount > amount-reserved {
			return bad("refund exceeds the remaining payment amount")
		}
		if _, err = tx.Exec(c.Request().Context(), `INSERT INTO refunds(uuid,payment_id,order_id,amount_paise,reason,status) VALUES($1,$2,$3,$4,$5,'PROCESSING')`, uid, payment, order, input.Amount, input.Reason); err != nil {
			return err
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return err
		}
		// Commit before the provider call. A timeout reserves the amount and requires
		// reconciliation instead of silently issuing a second refund.
		var result providerRefund
		err = s.Gateway.request(c.Request().Context(), "POST", "/payments/"+url.PathEscape(provider)+"/refund", map[string]any{"amount": input.Amount, "speed": "normal", "receipt": uid, "notes": map[string]string{"refund_uuid": uid}}, &result)
		if err != nil || !strings.HasPrefix(result.ID, "rfnd_") || result.PaymentID != provider || result.Amount != input.Amount || result.Currency != "INR" {
			_, _ = s.DB.Exec(c.Request().Context(), `UPDATE refunds SET status='UNKNOWN' WHERE uuid=$1 AND status='PROCESSING'`, uid)
			return echo.NewHTTPError(503, "refund response requires reconciliation; its amount remains reserved")
		}
		if _, err = s.DB.Exec(c.Request().Context(), `UPDATE refunds SET provider_refund_id=$2,status='PENDING' WHERE uuid=$1 AND status='PROCESSING'`, uid, result.ID); err != nil {
			return err
		}
		if result.Status == "processed" {
			tx, err = s.begin(c, "REFUND")
			if err != nil {
				return err
			}
			defer tx.Rollback(c.Request().Context())
			if err = s.completeRefund(c, tx, result); err != nil {
				return err
			}
			if err = tx.Commit(c.Request().Context()); err != nil {
				return err
			}
		}
		return c.JSON(201, map[string]string{"id": uid, "provider_refund_id": result.ID})
	})
	admin.POST("/refunds/:id/reconcile", func(c echo.Context) error {
		if publicID(c.Param("id")) != nil || s.Gateway == nil {
			return bad("refund reference required")
		}
		var input struct {
			Provider string `json:"provider_refund_id"`
		}
		if c.Bind(&input) != nil || !strings.HasPrefix(input.Provider, "rfnd_") || len(input.Provider) > 120 {
			return bad("provider refund ID required")
		}
		var result providerRefund
		if err := s.Gateway.request(c.Request().Context(), "GET", "/refunds/"+url.PathEscape(input.Provider), nil, &result); err != nil {
			return echo.NewHTTPError(503, "provider verification unavailable")
		}
		tx, err := s.begin(c, "REFUND")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		var payment string
		var amount int64
		var known *string
		if err = tx.QueryRow(c.Request().Context(), `SELECT p.provider_payment_id,r.amount_paise,r.provider_refund_id FROM refunds r JOIN payments p ON p.id=r.payment_id WHERE r.uuid=$1 FOR UPDATE OF r`, c.Param("id")).Scan(&payment, &amount, &known); err != nil {
			return unavailable(err)
		}
		if result.ID != input.Provider || result.PaymentID != payment || result.Amount != amount || result.Currency != "INR" || (known != nil && *known != result.ID) || (known == nil && result.Receipt != c.Param("id")) {
			return bad("provider refund does not match the original request")
		}
		if _, err = tx.Exec(c.Request().Context(), `UPDATE refunds SET provider_refund_id=$2 WHERE uuid=$1`, c.Param("id"), result.ID); err != nil {
			return err
		}
		if err = s.completeRefund(c, tx, result); err != nil {
			return err
		}
		return commit(c, tx)
	})
}
func (s *Service) completeRefund(c echo.Context, tx pgx.Tx, result providerRefund) error {
	var refund, payment, order, amount int64
	var status, provider string
	if err := tx.QueryRow(c.Request().Context(), `SELECT r.id,r.payment_id,r.order_id,r.amount_paise,r.status,p.provider_payment_id FROM refunds r JOIN payments p ON p.id=r.payment_id WHERE r.provider_refund_id=$1 FOR UPDATE OF r,p`, result.ID).Scan(&refund, &payment, &order, &amount, &status, &provider); err != nil {
		return bad("refund request must be reconciled before applying this event")
	}
	if result.PaymentID != provider || result.Amount != amount || result.Currency != "INR" {
		return bad("provider refund terms do not match")
	}
	if status == "COMPLETED" {
		return nil
	}
	if result.Status == "failed" {
		_, err := tx.Exec(c.Request().Context(), `UPDATE refunds SET status='FAILED' WHERE id=$1`, refund)
		return err
	}
	if result.Status != "processed" {
		return nil
	}
	if err := post(c, tx, fmt.Sprintf("refund/%d", refund), "REFUND", links{Order: ptr(order), Payment: ptr(payment)}, posting{"REFUND_EXPENSE", amount}, posting{"YEMBER_BANK", -amount}); err != nil {
		return err
	}
	var recognized bool
	if err := tx.QueryRow(c.Request().Context(), `SELECT EXISTS(SELECT 1 FROM restaurant_earnings WHERE order_id=$1)`, order).Scan(&recognized); err != nil {
		return err
	}
	if recognized {
		// Settlement and earning reversals remain separately accountable; hold any
		// unallocated earnings until their refund allocation is explicitly reviewed.
		if _, err := tx.Exec(c.Request().Context(), `UPDATE restaurant_earnings SET status='HELD' WHERE order_id=$1 AND status<>'REVERSED'`, order); err != nil {
			return err
		}
		if _, err := tx.Exec(c.Request().Context(), `UPDATE delivery_agent_earnings SET status='HELD' WHERE order_id=$1 AND status<>'REVERSED'`, order); err != nil {
			return err
		}
		if _, err := tx.Exec(c.Request().Context(), `INSERT INTO reconciliation_runs(source,status,difference_paise,period_start,period_end,summary_json) VALUES('RAZORPAY','EXCEPTIONS',$1,now(),now(),jsonb_build_object('refund_reference',$2::text,'reason','Refund requires separate earning and payout allocation review'))`, amount, result.ID); err != nil {
			return err
		}
	} else {
		if err := post(c, tx, fmt.Sprintf("refund-release/%d", refund), "REVERSAL", links{Order: ptr(order), Payment: ptr(payment)}, posting{"ORDER_RECEIPTS", amount}, posting{"REFUND_EXPENSE", -amount}); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(c.Request().Context(), `UPDATE refunds SET status='COMPLETED',completed_at=now() WHERE id=$1`, refund); err != nil {
		return err
	}
	var total, returned int64
	if err := tx.QueryRow(c.Request().Context(), `SELECT p.amount_paise,COALESCE((SELECT sum(amount_paise) FROM refunds WHERE payment_id=p.id AND status='COMPLETED'),0)::bigint FROM payments p WHERE p.id=$1`, payment).Scan(&total, &returned); err != nil {
		return err
	}
	if returned == total {
		if _, err := tx.Exec(c.Request().Context(), `UPDATE payments SET status='REFUNDED' WHERE id=$1`, payment); err != nil {
			return err
		}
		if _, err := tx.Exec(c.Request().Context(), `UPDATE orders SET status='REFUNDED' WHERE id=$1`, order); err != nil {
			return err
		}
	}
	return nil
}

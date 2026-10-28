package finance

import (
	"encoding/csv"
	"encoding/json"
	"github.com/imtiyazkumar/yember-core/internal/legacy/auth"
	"github.com/imtiyazkumar/yember-core/internal/legacy/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func Register(api *echo.Group, pool *pgxpool.Pool, cfg *config.Config, log *zap.Logger) {
	s := New(pool)
	partner := api.Group("/partner/finance", auth.JWTMiddleware(cfg.JWTSecret, log, pool), auth.RBACMiddleware("RESTAURANT_OWNER", "DELIVERY_PARTNER"))
	partner.Use(noStore)
	partner.GET("/registration", func(c echo.Context) error {
		return jsonQuery(c, pool, `SELECT jsonb_build_object('amount_paise',COALESCE((SELECT amount FROM finance.onboarding_attempts WHERE user_id=u.id AND status='PAID' ORDER BY created_at DESC LIMIT 1),(p.value->>'amount')::bigint),'currency',p.value->>'currency','terms',p.value->>'terms','refundable',false,'payment_status',CASE WHEN EXISTS(SELECT 1 FROM finance.onboarding_attempts WHERE user_id=u.id AND status='PAID') OR EXISTS(SELECT 1 FROM public.restaurants WHERE owner_id=u.id AND deleted_at IS NULL AND fee_payment_status='PAID') THEN 'PAID' ELSE 'DUE' END) FROM public.users u JOIN public.platform_settings p ON p.key=CASE WHEN u.role='RESTAURANT_OWNER' THEN 'onboarding_fee' ELSE 'delivery_registration_fee' END WHERE u.id=$1`, c.Get("userID"))
	})
	partner.GET("/cash-handovers", func(c echo.Context) error {
		return jsonQuery(c, pool, `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT c.order_id,c.amount AS amount_paise,COALESCE(h.status,'COLLECTED') AS status,c.created_at FROM finance.cod_custody c JOIN finance.order_financials f ON f.order_id=c.order_id JOIN public.riders p ON p.id=c.partner_id LEFT JOIN finance.cash_handovers h ON h.order_id=c.order_id WHERE p.user_id=$1 OR f.restaurant_user=$1 ORDER BY c.created_at DESC LIMIT 100)x`, c.Get("userID"))
	})
	partner.POST("/cash-handovers/:id", func(c echo.Context) error {
		var input struct {
			Amount int64 `json:"amount_paise"`
		}
		if c.Bind(&input) != nil {
			return echo.NewHTTPError(400, "invalid handover")
		}
		if err := s.SubmitCashHandover(c.Request().Context(), c.Get("userID").(string), c.Param("id"), input.Amount); err != nil {
			return echo.NewHTTPError(409, err.Error())
		}
		return c.NoContent(204)
	}, auth.RBACMiddleware("DELIVERY_PARTNER"))
	partner.POST("/cash-handovers/:id/confirm", func(c echo.Context) error {
		var input struct {
			Amount int64 `json:"amount_paise"`
			Accept bool  `json:"accept"`
		}
		if c.Bind(&input) != nil {
			return echo.NewHTTPError(400, "invalid acknowledgement")
		}
		if err := s.ConfirmCashHandover(c.Request().Context(), c.Get("userID").(string), c.Param("id"), input.Amount, input.Accept); err != nil {
			return echo.NewHTTPError(409, err.Error())
		}
		return c.NoContent(204)
	}, auth.RBACMiddleware("RESTAURANT_OWNER"))
	partner.GET("/summary", func(c echo.Context) error {
		raw, err := s.Summary(c.Request().Context(), c.Get("userID").(string))
		return respond(c, raw, err)
	})
	partner.GET("/bank", func(c echo.Context) error {
		return jsonQuery(c, pool, `SELECT COALESCE((SELECT jsonb_build_object('id',id,'last_four',last_four,'status',status,'created_at',created_at) FROM finance.bank_accounts WHERE user_id=$1 AND status IN ('PENDING','VERIFIED')),'null'::jsonb)`, c.Get("userID"))
	})
	partner.PUT("/bank", func(c echo.Context) error {
		var input BankInput
		if c.Bind(&input) != nil {
			return echo.NewHTTPError(400, "invalid payout account")
		}
		if err := s.SaveBank(c.Request().Context(), c.Get("userID").(string), input); err != nil {
			return echo.NewHTTPError(409, err.Error())
		}
		return c.NoContent(204)
	})
	partner.GET("/ledger", func(c echo.Context) error {
		return jsonQuery(c, pool, `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT e.id,j.order_id,j.kind,e.component,-e.amount AS amount,j.reference,j.reason,e.created_at,f.gross,f.commission_bps,f.commission,f.delivery_charge,f.rider_base,f.total FROM finance.entries e JOIN finance.journals j ON j.id=e.journal_id LEFT JOIN finance.order_financials f ON f.order_id=j.order_id WHERE e.user_id=$1 ORDER BY e.created_at DESC,e.id LIMIT 100 OFFSET $2)x`, c.Get("userID"), offset(c))
	})
	partner.GET("/settlements", func(c echo.Context) error {
		return jsonQuery(c, pool, `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT p.id,p.batch_id,p.amount,p.currency,p.status,p.provider,p.provider_id,p.evidence,p.created_at,p.updated_at,b.cutoff FROM finance.payouts p JOIN finance.batches b ON b.id=p.batch_id WHERE p.user_id=$1 ORDER BY p.created_at DESC,p.id LIMIT 100 OFFSET $2)x`, c.Get("userID"), offset(c))
	})
	customer := api.Group("/customer/finance", auth.JWTMiddleware(cfg.JWTSecret, log, pool), auth.RBACMiddleware("CUSTOMER"))
	customer.Use(noStore)
	customer.POST("/orders/:id/checkout", func(c echo.Context) error {
		session, err := s.Checkout(c.Request().Context(), c.Get("userID").(string), c.Param("id"))
		if err != nil {
			return echo.NewHTTPError(409, err.Error())
		}
		return c.JSON(200, session)
	})
	customer.GET("/orders/:id", func(c echo.Context) error {
		return jsonQuery(c, pool, `SELECT jsonb_build_object('order_id',f.order_id,'gross',f.gross,'discount',f.discount,'delivery_charge',f.delivery_charge,'tax',f.tax,'total',f.total,'currency',f.currency,'payment_status',COALESCE((SELECT status FROM finance.payment_attempts WHERE order_id=f.order_id ORDER BY attempt DESC LIMIT 1),'UNPAID'),'refunded',COALESCE((SELECT sum(e.amount) FROM finance.entries e JOIN finance.journals j ON j.id=e.journal_id WHERE j.order_id=f.order_id AND j.kind='REFUND' AND e.account='REFUND_EXPENSE'),0)) FROM finance.order_financials f JOIN public.orders o ON o.id=f.order_id WHERE f.order_id=$1 AND o.customer_id=$2`, c.Param("id"), c.Get("userID"))
	})
	api.POST("/webhooks/razorpay", func(c echo.Context) error {
		raw, err := io.ReadAll(http.MaxBytesReader(c.Response(), c.Request().Body, 1<<20))
		if err != nil {
			return echo.NewHTTPError(400, "invalid webhook")
		}
		if !VerifyRazorpay(raw, c.Request().Header.Get("X-Razorpay-Signature"), os.Getenv("RAZORPAY_WEBHOOK_SECRET")) {
			return echo.NewHTTPError(400, "invalid signature")
		}
		if err = s.ApplyRazorpay(c.Request().Context(), c.Request().Header.Get("X-Razorpay-Event-Id"), raw); err != nil {
			log.Warn("finance webhook rejected", zap.Error(err))
			return echo.NewHTTPError(409, "event requires retry or reconciliation")
		}
		return c.NoContent(200)
	})
	api.POST("/webhooks/razorpayx", func(c echo.Context) error {
		raw, err := io.ReadAll(http.MaxBytesReader(c.Response(), c.Request().Body, 1<<20))
		if err != nil || !VerifyRazorpay(raw, c.Request().Header.Get("X-Razorpay-Signature"), os.Getenv("RAZORPAYX_WEBHOOK_SECRET")) {
			return echo.NewHTTPError(400, "invalid payout webhook")
		}
		var event struct {
			Payload struct {
				Payout struct {
					Entity struct {
						ID        string `json:"id"`
						Reference string `json:"reference_id"`
					} `json:"entity"`
				} `json:"payout"`
			} `json:"payload"`
		}
		if json.Unmarshal(raw, &event) != nil {
			return echo.NewHTTPError(400, "invalid payout event")
		}
		actor := os.Getenv("FINANCE_AUTOMATION_ADMIN_ID")
		var allowed bool
		if actor == "" {
			return echo.NewHTTPError(503, "payout reconciliation operator is not configured")
		}
		if err = pool.QueryRow(c.Request().Context(), `SELECT EXISTS(SELECT 1 FROM finance.permissions p JOIN public.users u ON u.id=p.user_id WHERE p.user_id=$1 AND p.permission='RECONCILE' AND u.role='ADMIN' AND u.disabled_at IS NULL AND u.deleted_at IS NULL)`, actor).Scan(&allowed); err != nil || !allowed {
			return echo.NewHTTPError(503, "payout reconciliation unavailable")
		}
		var id, status string
		if err = pool.QueryRow(c.Request().Context(), `SELECT id,status FROM finance.payouts WHERE id::text=$1 AND provider_id=$2`, event.Payload.Payout.Entity.Reference, event.Payload.Payout.Entity.ID).Scan(&id, &status); err != nil {
			return echo.NewHTTPError(409, "payout reference requires reconciliation")
		}
		if status == "FAILED" {
			return c.NoContent(200)
		}
		if err = s.ReconcileRazorpayX(c.Request().Context(), actor, id); err != nil {
			return echo.NewHTTPError(409, "payout requires retry or review")
		}
		return c.NoContent(200)
	})
	customer.GET("/pricing", func(c echo.Context) error {
		return jsonQuery(c, pool, `SELECT jsonb_build_object('delivery_charge',delivery_charge,'online_enabled',enabled,'cod_enabled',$1::boolean,'currency','INR') FROM finance.policy`, os.Getenv("CUSTOMER_PAYMENT_METHOD") == "CASH_ON_DELIVERY")
	})

	admin := api.Group("/admin/finance", auth.JWTMiddleware(cfg.JWTSecret, log, pool), auth.RBACMiddleware("ADMIN"))
	admin.Use(noStore)
	permission := func(value string) echo.MiddlewareFunc {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				var allowed bool
				if err := pool.QueryRow(c.Request().Context(), `SELECT EXISTS(SELECT 1 FROM finance.permissions WHERE user_id=$1 AND permission=$2)`, c.Get("userID"), value).Scan(&allowed); err != nil {
					return echo.NewHTTPError(503, "finance authorization unavailable")
				}
				if !allowed {
					return echo.NewHTTPError(403, "finance permission required: "+value)
				}
				return next(c)
			}
		}
	}
	admin.GET("/overview", func(c echo.Context) error {
		return jsonQuery(c, pool, `SELECT jsonb_build_object('currency','INR','collected',COALESCE((SELECT sum(amount) FROM finance.entries WHERE account='CASH' AND component='collection'),0),'commission',COALESCE((SELECT -sum(amount) FROM finance.entries WHERE account='COMMISSION'),0),'outstanding',COALESCE((SELECT -sum(amount) FROM finance.entries WHERE account='PARTNER_PAYABLE'),0),'refunds',COALESCE((SELECT sum(e.amount) FROM finance.entries e JOIN finance.journals j ON j.id=e.journal_id WHERE j.kind='REFUND' AND e.account='REFUND_EXPENSE'),0),'open_discrepancies',(SELECT count(*) FROM finance.discrepancies WHERE resolved_at IS NULL),'policy',(SELECT to_jsonb(p) FROM finance.policy p))`)
	}, permission("READ"))
	queries := map[string]string{
		"cash-handovers":      `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT h.uuid AS id,c.order_id,c.amount AS amount,COALESCE(h.status,'COLLECTED') AS status,p.user_id AS agent_user_id,f.restaurant_user,COALESCE((SELECT sum(e.amount) FROM finance.entries e JOIN finance.journals j ON j.id=e.journal_id WHERE j.order_id=c.order_id AND e.account='RESTAURANT_FEE_RECEIVABLE'),0) AS fee_receivable,c.created_at FROM finance.cod_custody c JOIN finance.order_financials f ON f.order_id=c.order_id JOIN public.riders p ON p.id=c.partner_id LEFT JOIN finance.cash_handovers h ON h.order_id=c.order_id ORDER BY c.created_at DESC LIMIT 100 OFFSET $1)x`,
		"onboarding-payments": `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT id,restaurant_id,delivery_agent_id,user_id,amount,currency,status,provider_id,payment_reference,created_at FROM finance.onboarding_attempts ORDER BY created_at DESC,id LIMIT 100 OFFSET $1)x`,
		"payments":            `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT id,order_id,provider,amount,currency,status,provider_id,payment_reference,created_at FROM finance.payment_attempts ORDER BY created_at DESC,id LIMIT 100 OFFSET $1)x`,
		"balances":            `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT u.id,u.full_name,u.role,-sum(e.amount) AS pending FROM finance.entries e JOIN public.users u ON u.id=e.user_id WHERE e.account='PARTNER_PAYABLE' GROUP BY u.id ORDER BY u.id LIMIT 100 OFFSET $1)x`,
		"banks":               `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT b.id,b.user_id,u.full_name,u.role,b.last_four,b.status,b.verification_reference,b.created_at FROM finance.bank_accounts b JOIN public.users u ON u.id=b.user_id WHERE b.status IN ('PENDING','VERIFIED') ORDER BY b.created_at DESC LIMIT 100 OFFSET $1)x`,
		"batches":             `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT b.*,COALESCE((SELECT sum(amount) FROM finance.payouts WHERE batch_id=b.id),0) AS amount,(SELECT count(*) FROM finance.payouts WHERE batch_id=b.id) AS payees FROM finance.batches b ORDER BY cutoff DESC LIMIT 100 OFFSET $1)x`,
		"payouts":             `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT p.*,b.cutoff FROM finance.payouts p JOIN finance.batches b ON b.id=p.batch_id ORDER BY p.created_at DESC,p.id LIMIT 100 OFFSET $1)x`,
		"discrepancies":       `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT * FROM finance.discrepancies ORDER BY created_at DESC LIMIT 100 OFFSET $1)x`,
		"refunds":             `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT * FROM finance.refund_requests ORDER BY created_at DESC LIMIT 100 OFFSET $1)x`,
	}
	admin.GET("/batches/:id", func(c echo.Context) error {
		return jsonQuery(c, pool, `SELECT jsonb_build_object('batch',to_jsonb(b),'payees',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',p.id,'user_id',p.user_id,'full_name',u.full_name,'role',u.role,'amount',p.amount,'status',p.status,'bank_last_four',a.last_four)) FROM finance.payouts p JOIN public.users u ON u.id=p.user_id LEFT JOIN finance.bank_accounts a ON a.id=p.bank_account_id WHERE p.batch_id=b.id),'[]'::jsonb)) FROM finance.batches b WHERE b.id=$1`, c.Param("id"))
	}, permission("READ"))
	for name, query := range queries {
		admin.GET("/"+name, func(c echo.Context) error { return jsonQuery(c, pool, query, offset(c)) }, permission("READ"))
	}
	admin.GET("/orders/:id", func(c echo.Context) error {
		return jsonQuery(c, pool, `SELECT jsonb_build_object('breakdown',to_jsonb(f),'ledger',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',j.id,'kind',j.kind,'reference',j.reference,'reason',j.reason,'created_at',j.created_at,'entries',(SELECT jsonb_agg(to_jsonb(e)) FROM finance.entries e WHERE e.journal_id=j.id))) FROM finance.journals j WHERE j.order_id=f.order_id),'[]')) FROM finance.order_financials f WHERE order_id=$1`, c.Param("id"))
	}, permission("READ"))
	admin.POST("/batches", func(c echo.Context) error {
		id, err := s.Generate(c.Request().Context(), c.Get("userID").(string), time.Now())
		if err != nil {
			return echo.NewHTTPError(409, "could not prepare settlement batch")
		}
		return c.JSON(201, map[string]string{"id": id})
	}, permission("PREPARE"))
	admin.POST("/batches/:id/approve", func(c echo.Context) error {
		return action(c, func(reason string) error {
			return s.Approve(c.Request().Context(), c.Get("userID").(string), c.Param("id"), reason)
		})
	}, permission("APPROVE"))
	admin.POST("/payouts/:id/initiate", func(c echo.Context) error {
		return action(c, func(reason string) error {
			return s.Initiate(c.Request().Context(), c.Get("userID").(string), c.Param("id"), reason)
		})
	}, permission("PAYOUT"))
	admin.POST("/banks/:id/verify", func(c echo.Context) error {
		return action(c, func(reason string) error {
			return s.VerifyBank(c.Request().Context(), c.Get("userID").(string), c.Param("id"), reason)
		})
	}, permission("RECONCILE"))
	admin.POST("/payouts/:id/reconcile", func(c echo.Context) error {
		var input struct {
			Status    string `json:"status"`
			Reference string `json:"reference"`
		}
		if c.Bind(&input) != nil {
			return echo.NewHTTPError(400, "invalid reconciliation")
		}
		if err := s.Confirm(c.Request().Context(), c.Get("userID").(string), c.Param("id"), input.Status, input.Reference); err != nil {
			return echo.NewHTTPError(409, err.Error())
		}
		return c.NoContent(204)
	}, permission("RECONCILE"))
	admin.PUT("/restaurants/:id/commission", func(c echo.Context) error {
		var input struct {
			BPS    *int   `json:"commission_bps"`
			Reason string `json:"reason"`
		}
		if c.Bind(&input) != nil || input.BPS == nil {
			return echo.NewHTTPError(400, "invalid commission")
		}
		if err := s.Commission(c.Request().Context(), c.Get("userID").(string), c.Param("id"), input.Reason, *input.BPS); err != nil {
			return echo.NewHTTPError(409, err.Error())
		}
		return c.NoContent(204)
	}, permission("CONFIGURE"))
	admin.POST("/orders/:id/refunds", func(c echo.Context) error {
		var input struct {
			Amount int64  `json:"amount"`
			Key    string `json:"request_key"`
			Reason string `json:"reason"`
		}
		if c.Bind(&input) != nil {
			return echo.NewHTTPError(400, "invalid refund")
		}
		if err := s.Refund(c.Request().Context(), c.Get("userID").(string), c.Param("id"), input.Key, input.Reason, input.Amount); err != nil {
			return echo.NewHTTPError(409, err.Error())
		}
		return c.NoContent(202)
	}, permission("ADJUST"))
	admin.POST("/orders/:id/cod-remittance", func(c echo.Context) error {
		var input struct {
			Reference string `json:"reference"`
		}
		if c.Bind(&input) != nil {
			return echo.NewHTTPError(400, "invalid remittance")
		}
		if err := s.COD(c.Request().Context(), c.Get("userID").(string), c.Param("id"), input.Reference); err != nil {
			return echo.NewHTTPError(409, err.Error())
		}
		return c.NoContent(204)
	}, permission("RECONCILE"))
	admin.POST("/payouts/:id/provider-submit", func(c echo.Context) error {
		if err := s.SubmitRazorpayX(c.Request().Context(), c.Param("id")); err != nil {
			return echo.NewHTTPError(409, err.Error())
		}
		return c.NoContent(202)
	}, permission("PAYOUT"))
	admin.POST("/payouts/:id/provider-reconcile", func(c echo.Context) error {
		if err := s.ReconcileRazorpayX(c.Request().Context(), c.Get("userID").(string), c.Param("id")); err != nil {
			return echo.NewHTTPError(409, err.Error())
		}
		return c.NoContent(204)
	}, permission("RECONCILE"))
	admin.POST("/payments/:id/recover-link", func(c echo.Context) error {
		var input struct {
			Reference string `json:"reference"`
		}
		if c.Bind(&input) != nil {
			return echo.NewHTTPError(400, "provider reference required")
		}
		if err := s.RecoverLink(c.Request().Context(), c.Get("userID").(string), c.Param("id"), input.Reference); err != nil {
			return echo.NewHTTPError(409, err.Error())
		}
		return c.NoContent(204)
	}, permission("RECONCILE"))
	admin.POST("/adjustments", func(c echo.Context) error {
		var in struct {
			User      string `json:"user_id"`
			Order     string `json:"order_id"`
			Key       string `json:"request_key"`
			Component string `json:"component"`
			Reason    string `json:"reason"`
			Amount    int64  `json:"amount"`
		}
		if c.Bind(&in) != nil {
			return echo.NewHTTPError(400, "invalid adjustment")
		}
		if err := s.Adjustment(c.Request().Context(), c.Get("userID").(string), in.User, in.Order, in.Key, in.Component, in.Reason, in.Amount); err != nil {
			return echo.NewHTTPError(409, err.Error())
		}
		return c.NoContent(204)
	}, permission("ADJUST"))
	admin.PUT("/policy", func(c echo.Context) error {
		var in struct {
			Interval int    `json:"interval_days"`
			Delivery int64  `json:"delivery_charge"`
			Rider    int64  `json:"rider_base"`
			Enabled  bool   `json:"enabled"`
			Reason   string `json:"reason"`
		}
		if c.Bind(&in) != nil || in.Interval < 1 || in.Interval > 90 || in.Delivery < 0 || in.Delivery > 10000000 || in.Rider < 0 || in.Rider > in.Delivery || len(in.Reason) < 10 {
			return echo.NewHTTPError(400, "valid schedule, pricing and reason required; rider base may not exceed delivery charge")
		}
		tx, err := pool.Begin(c.Request().Context())
		if err != nil {
			return echo.NewHTTPError(503, "finance unavailable")
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), `UPDATE finance.policy SET interval_days=$1,delivery_charge=$2,rider_base=$3,enabled=$4,updated_at=now()`, in.Interval, in.Delivery, in.Rider, in.Enabled); err != nil {
			return echo.NewHTTPError(409, "policy update failed")
		}
		if err = audit(c.Request().Context(), tx, c.Get("userID").(string), "FINANCE_POLICY_CHANGED", "policy", in.Reason); err != nil {
			return err
		}
		return tx.Commit(c.Request().Context())
	}, permission("CONFIGURE"))

	admin.POST("/payouts/:id/cancel", func(c echo.Context) error {
		return action(c, func(reason string) error {
			return s.CancelPayout(c.Request().Context(), c.Get("userID").(string), c.Param("id"), reason)
		})
	}, permission("APPROVE"))
	admin.POST("/discrepancies/:id/resolve", func(c echo.Context) error {
		return action(c, func(reason string) error {
			return s.ResolveDiscrepancy(c.Request().Context(), c.Get("userID").(string), c.Param("id"), reason)
		})
	}, permission("ADJUST"), permission("RECONCILE"))

	admin.GET("/health", func(c echo.Context) error {
		return jsonQuery(c, pool, `SELECT jsonb_build_object('email_pending',(SELECT count(*) FROM notifications.email_outbox WHERE status IN ('PENDING','SENDING')),'email_failed',(SELECT count(*) FROM notifications.email_outbox WHERE status='FAILED'),'push_failed',(SELECT count(*) FROM notifications.push_outbox WHERE status='FAILED'),'refund_jobs_review',(SELECT count(*) FROM finance.refund_jobs WHERE status='REVIEW'),'payout_unknown',(SELECT count(*) FROM finance.payouts WHERE status='UNKNOWN'),'stuck_orders',(SELECT count(*) FROM public.orders WHERE status IN ('CREATED','CONFIRMED','ACCEPTED','PREPARING','READY_FOR_PICKUP') AND updated_at<now()-interval '30 minutes'),'oldest_email_seconds',(SELECT COALESCE(extract(epoch FROM now()-min(created_at)),0) FROM notifications.email_outbox WHERE status IN ('PENDING','SENDING')))`)
	}, permission("READ"))

	admin.POST("/discrepancies/:id/reversal", func(c echo.Context) error {
		return action(c, func(reason string) error {
			return s.ReconcileReversal(c.Request().Context(), c.Get("userID").(string), c.Param("id"), reason)
		})
	}, permission("RECONCILE"))

	admin.GET("/export", func(c echo.Context) error {
		rows, err := pool.Query(c.Request().Context(), `SELECT j.id::text,j.order_id::text,j.kind,j.reference,j.created_at,e.account,e.user_id::text,e.component,e.amount FROM finance.entries e JOIN finance.journals j ON j.id=e.journal_id ORDER BY j.created_at,e.id`)
		if err != nil {
			return echo.NewHTTPError(503, "export unavailable")
		}
		defer rows.Close()
		c.Response().Header().Set("Content-Disposition", "attachment; filename=finance-ledger.csv")
		c.Response().Header().Set("Content-Type", "text/csv")
		w := csv.NewWriter(c.Response())
		defer w.Flush()
		_ = w.Write([]string{"journal", "order", "kind", "reference", "timestamp", "account", "payee", "component", "minor_units"})
		for rows.Next() {
			values, err := rows.Values()
			if err != nil {
				return err
			}
			record := make([]string, len(values))
			for i, value := range values {
				raw, _ := json.Marshal(value)
				v := strings.Trim(string(raw), "\"")
				if strings.ContainsAny(v[:min(1, len(v))], "=+-@\t\r") {
					v = "'" + v
				}
				record[i] = v
			}
			if err = w.Write(record); err != nil {
				return err
			}
		}
		return rows.Err()
	}, permission("READ"))
}
func noStore(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error { c.Response().Header().Set("Cache-Control", "no-store"); return next(c) }
}
func offset(c echo.Context) int {
	v, _ := strconv.Atoi(c.QueryParam("offset"))
	if v < 0 || v > 1000000 {
		return 0
	}
	return v
}
func respond(c echo.Context, raw json.RawMessage, err error) error {
	if err != nil {
		return echo.NewHTTPError(404, "financial record unavailable")
	}
	return c.JSON(http.StatusOK, raw)
}
func jsonQuery(c echo.Context, pool *pgxpool.Pool, query string, args ...any) error {
	var raw json.RawMessage
	err := pool.QueryRow(c.Request().Context(), query, args...).Scan(&raw)
	return respond(c, raw, err)
}
func action(c echo.Context, fn func(string) error) error {
	var input struct {
		Reason string `json:"reason"`
	}
	if c.Bind(&input) != nil {
		return echo.NewHTTPError(400, "reason required")
	}
	if err := fn(input.Reason); err != nil {
		return echo.NewHTTPError(409, err.Error())
	}
	return c.NoContent(204)
}

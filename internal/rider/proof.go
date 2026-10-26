package rider

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"math/big"
	"os"
	"time"
)

type proofKey struct{}
type completionInput struct {
	Code          string
	CashCollected bool
}

func WithCompletion(ctx context.Context, code string, cash bool) context.Context {
	return context.WithValue(ctx, proofKey{}, completionInput{code, cash})
}
func createCompletion(ctx context.Context, tx pgx.Tx, order string, resend ...bool) error {
	value, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return err
	}
	code := fmt.Sprintf("%06d", value.Int64())
	hash := sha256.Sum256([]byte(code))
	retry := len(resend) > 0 && resend[0]
	result, err := tx.Exec(ctx, `INSERT INTO delivery.completion_challenges(order_id,code_hash,expires_at) VALUES($1,$2,now()+interval '6 hours') ON CONFLICT(order_id) DO UPDATE SET code_hash=EXCLUDED.code_hash,expires_at=EXCLUDED.expires_at,attempts=0,sent_at=now(),resend_count=delivery.completion_challenges.resend_count+1 WHERE $3 AND delivery.completion_challenges.consumed_at IS NULL AND delivery.completion_challenges.sent_at<now()-interval '1 minute' AND delivery.completion_challenges.resend_count<3`, order, hex.EncodeToString(hash[:]), retry)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		if retry {
			return errors.New("wait one minute between resends; at most three resends per order")
		}
		return nil
	}
	if _, err = tx.Exec(ctx, `INSERT INTO notifications.sms_outbox(recipient,body,expires_at) SELECT '+91'||u.phone,$2,now()+interval '6 hours' FROM public.orders o JOIN public.users u ON u.id=o.customer_id WHERE o.id=$1 AND u.phone ~ '^[6-9][0-9]{9}$'`, order, "Your Yember delivery code is "+code+". Share only after receiving your order and paying any cash due."); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO notifications.email_outbox(recipient,subject,body) SELECT u.email,'Your Yember delivery confirmation code',$2 FROM public.orders o JOIN public.users u ON u.id=o.customer_id WHERE o.id=$1 AND u.email IS NOT NULL`, order, "Share "+code+" with your rider only after receiving your order and paying any cash due. Never share sign-in codes.")
	return err
}

// Only the authenticated customer can rotate a code; riders never receive the secret from an API.
func RegisterCustomerProof(g *echo.Group, pool *pgxpool.Pool) {
	g.POST("/orders/:id/resend-delivery-code", func(c echo.Context) error {
		ctx := c.Request().Context()
		tx, err := pool.Begin(ctx)
		if err != nil {
			return echo.NewHTTPError(503, "delivery recovery unavailable")
		}
		defer tx.Rollback(ctx)
		var id string
		if err = tx.QueryRow(ctx, `SELECT id FROM public.orders WHERE id=$1 AND customer_id=$2 AND status IN ('PICKED_UP','OUT_FOR_DELIVERY') AND deleted_at IS NULL FOR UPDATE`, c.Param("id"), c.Get("userID")).Scan(&id); err != nil {
			return echo.NewHTTPError(404, "eligible order not found")
		}
		if err = createCompletion(ctx, tx, id, true); err != nil {
			return echo.NewHTTPError(409, err.Error())
		}
		if err = tx.Commit(ctx); err != nil {
			return echo.NewHTTPError(503, "delivery recovery unavailable")
		}
		return c.NoContent(204)
	})
}
func verifyCompletion(ctx context.Context, tx pgx.Tx, order, partner string) error {
	input, _ := ctx.Value(proofKey{}).(completionInput)
	if os.Getenv("DELIVERY_PROOF_REQUIRED") != "true" && os.Getenv("ENVIRONMENT") != "production" && input.Code == "" {
		return nil
	}
	var expected string
	var attempts int
	var expiry time.Time
	if err := tx.QueryRow(ctx, `SELECT code_hash,attempts,expires_at FROM delivery.completion_challenges WHERE order_id=$1 AND consumed_at IS NULL FOR UPDATE`, order).Scan(&expected, &attempts, &expiry); err != nil {
		return errors.New("delivery confirmation challenge unavailable")
	}
	digest := sha256.Sum256([]byte(input.Code))
	valid := len(input.Code) == 6 && attempts < 5 && time.Now().Before(expiry) && subtle.ConstantTimeCompare([]byte(expected), []byte(hex.EncodeToString(digest[:]))) == 1
	if !valid {
		if _, err := tx.Exec(ctx, `UPDATE delivery.completion_challenges SET attempts=attempts+1 WHERE order_id=$1`, order); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return errors.New("delivery code invalid, expired or exhausted; contact support")
	}
	var cod bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM finance.payment_attempts WHERE order_id=$1 AND provider='COD')`, order).Scan(&cod); err != nil {
		return err
	}
	if cod && !input.CashCollected {
		return errors.New("confirm collection of the full server-calculated cash amount before completing delivery")
	}
	if _, err := tx.Exec(ctx, `UPDATE delivery.completion_challenges SET consumed_at=now() WHERE order_id=$1`, order); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO delivery.completion_proofs(order_id,partner_id) VALUES($1,$2)`, order, partner); err != nil {
		return err
	}
	if cod {
		_, err := tx.Exec(ctx, `INSERT INTO finance.cod_custody(order_id,partner_id,amount) SELECT order_id,$2,total FROM finance.order_financials WHERE order_id=$1`, order, partner)
		return err
	}
	return nil
}

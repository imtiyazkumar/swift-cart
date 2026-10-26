package rider

import (
	"context"
	"github.com/imtiyazkumar/yember-core/internal/devseed"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"github.com/labstack/echo/v4"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestDeliveryCodeReplayResendOwnershipAndAttempts(t *testing.T) {
	t.Setenv("DELIVERY_PROOF_REQUIRED", "true")
	pool := testdb.New(t)
	ctx := context.Background()
	if err := devseed.Seed(ctx, pool, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	var order string
	if err := pool.QueryRow(ctx, `INSERT INTO public.orders(customer_id,merchant_id,status,total_price,delivery_address) VALUES($1,$2,'PICKED_UP',100,'{"latitude":34,"longitude":74}') RETURNING id`, devseed.CustomerID, devseed.RestaurantID).Scan(&order); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = createCompletion(ctx, tx, order); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var body string
	if err = pool.QueryRow(ctx, `SELECT body FROM notifications.email_outbox WHERE subject='Your Yember delivery confirmation code' ORDER BY created_at DESC LIMIT 1`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`[0-9]{6}`).FindString(body)
	if code == "" {
		t.Fatal("missing customer code")
	}
	tx, _ = pool.Begin(ctx)
	if err = createCompletion(ctx, tx, order); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM notifications.email_outbox WHERE subject='Your Yember delivery confirmation code'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate code notification %d %v", count, err)
	}
	wrong := "000000"
	if wrong == code {
		wrong = "111111"
	}
	tx, _ = pool.Begin(ctx)
	if err = verifyCompletion(WithCompletion(ctx, wrong, false), tx, order, devseed.PartnerID); err == nil {
		t.Fatal("wrong code accepted")
	}
	var attempts int
	if err = pool.QueryRow(ctx, `SELECT attempts FROM delivery.completion_challenges WHERE order_id=$1`, order).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatalf("failed attempt not durable %d %v", attempts, err)
	}
	e := echo.New()
	g := e.Group("", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { c.Set("userID", devseed.OwnerID); return next(c) }
	})
	RegisterCustomerProof(g, pool)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("POST", "/orders/"+order+"/resend-delivery-code", nil))
	if rec.Code != 404 {
		t.Fatalf("other user resend returned %d", rec.Code)
	}
	tx, _ = pool.Begin(ctx)
	if err = verifyCompletion(WithCompletion(ctx, code, false), tx, order, devseed.PartnerID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx, _ = pool.Begin(ctx)
	defer tx.Rollback(ctx)
	if err = verifyCompletion(WithCompletion(ctx, code, false), tx, order, devseed.PartnerID); err == nil {
		t.Fatal("completion proof replay accepted")
	}
}

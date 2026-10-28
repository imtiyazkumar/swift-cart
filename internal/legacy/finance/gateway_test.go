package finance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/imtiyazkumar/yember-core/internal/legacy/devseed"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRazorpayCaptureDuplicateRefundAndSnapshot(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if err := devseed.Seed(ctx, p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	var order string
	if err := p.QueryRow(ctx, `INSERT INTO public.orders(customer_id,restaurant_id,status,total_price,delivery_address) VALUES($1,$2,'CREATED',1000,'{"latitude":34,"longitude":74}') RETURNING id`, devseed.CustomerID, devseed.RestaurantID).Scan(&order); err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = Snapshot(ctx, tx, order); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, `UPDATE public.restaurants SET commission_bps=2000 WHERE id=$1`, devseed.RestaurantID); err != nil {
		t.Fatal(err)
	}
	var commission, total int64
	if err = p.QueryRow(ctx, `SELECT commission,total FROM finance.order_financials WHERE order_id=$1`, order).Scan(&commission, &total); err != nil || commission != 15000 || total != 104000 {
		t.Fatal("commission snapshot or delivery fee", commission, total, err)
	}
	var attempt string
	if err = p.QueryRow(ctx, `INSERT INTO finance.payment_attempts(order_id,provider,amount,currency,status) VALUES($1,'RAZORPAY',104000,'INR','CREATING') RETURNING id`, order).Scan(&attempt); err != nil {
		t.Fatal(err)
	}
	event := map[string]any{"event": "payment_link.paid", "payload": map[string]any{"payment_link": map[string]any{"entity": map[string]any{"id": "plink_test", "reference_id": attempt, "status": "paid", "amount": 104000, "amount_paid": 104000, "currency": "INR", "order_id": "order_test"}}, "payment": map[string]any{"entity": map[string]any{"id": "pay_test", "status": "captured", "amount": 104000, "currency": "INR", "order_id": "order_test", "fee": 100}}}}
	raw, _ := json.Marshal(event)
	s := New(p)
	if err = s.ApplyRazorpay(ctx, "event_capture_1", raw); err != nil {
		t.Fatal(err)
	}
	if err = s.ApplyRazorpay(ctx, "event_capture_1", raw); err != nil {
		t.Fatal(err)
	}
	refund := map[string]any{"event": "refund.processed", "payload": map[string]any{"refund": map[string]any{"entity": map[string]any{"id": "rfnd_test", "payment_id": "pay_test", "amount": 20000, "status": "processed"}}}}
	raw, _ = json.Marshal(refund)
	if err = s.ApplyRazorpay(ctx, "event_refund_1", raw); err != nil {
		t.Fatal(err)
	}
	if err = s.ApplyRazorpay(ctx, "event_refund_2", raw); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM finance.journals WHERE kind='REFUND'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("refund duplicated", count, err)
	}
	var refunded int64
	if err = p.QueryRow(ctx, `SELECT COALESCE(sum(e.amount),0) FROM finance.entries e JOIN finance.journals j ON j.id=e.journal_id WHERE j.order_id=$1 AND j.kind='REFUND' AND e.account='REFUND_EXPENSE'`, order).Scan(&refunded); err != nil || refunded != 20000 {
		t.Fatal("incorrect refund", refunded, err)
	}
	var status string
	if err = p.QueryRow(ctx, `SELECT status FROM public.orders WHERE id=$1`, order).Scan(&status); err != nil || status == "REFUNDED" {
		t.Fatal("partial refund incorrectly became full", status, err)
	}
}

func TestRazorpayUncertainCreationDoesNotDuplicate(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if err := devseed.Seed(ctx, p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	var order string
	if err := p.QueryRow(ctx, `INSERT INTO public.orders(customer_id,restaurant_id,status,total_price,delivery_address) VALUES($1,$2,'PAYMENT_PENDING',1000,'{"latitude":34,"longitude":74}') RETURNING id`, devseed.CustomerID, devseed.RestaurantID).Scan(&order); err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = Snapshot(ctx, tx, order); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, `UPDATE finance.policy SET enabled=true`); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RAZORPAY_KEY_ID", "rzp_test_fixture")
	t.Setenv("RAZORPAY_KEY_SECRET", "test_secret")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(503) }))
	defer server.Close()
	s := New(p)
	s.Gateway.BaseURL = server.URL
	if _, err = s.Checkout(ctx, devseed.CustomerID, order); err == nil {
		t.Fatal("uncertain creation accepted")
	}
	if _, err = s.Checkout(ctx, devseed.CustomerID, order); err == nil {
		t.Fatal("uncertain operation must be reconciled")
	}
	if calls != 1 {
		t.Fatal("duplicate provider request", calls)
	}
}
func TestRazorpaySignatures(t *testing.T) {
	raw := []byte(`{"event":"payment_link.paid"}`)
	mac := hmac.New(sha256.New, []byte("test-webhook-secret"))
	mac.Write(raw)
	signature := hex.EncodeToString(mac.Sum(nil))
	if !VerifyRazorpay(raw, signature, "test-webhook-secret") {
		t.Fatal("valid signature rejected")
	}
	if VerifyRazorpay(append(raw, ' '), signature, "test-webhook-secret") || VerifyRazorpay(raw, signature, "") {
		t.Fatal("invalid signature accepted")
	}
}

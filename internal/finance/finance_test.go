package finance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/imtiyazkumar/yember-core/internal/devseed"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"testing"
	"time"
)

func TestCommissionAndCutoff(t *testing.T) {
	for _, c := range []struct {
		gross int64
		rate  int
		want  int64
	}{{10000, 1500, 1500}, {1, 1500, 0}, {10, 1500, 2}, {10000, 0, 0}, {10000, 10000, 10000}} {
		got, err := Commission(c.gross, c.rate)
		if err != nil || got != c.want {
			t.Fatalf("commission %+v got %d: %v", c, got, err)
		}
	}
	if _, err := Commission(-1, 1500); err == nil {
		t.Fatal("negative basis accepted")
	}
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := Cutoff(anchor.Add(31*24*time.Hour), anchor, 15); !got.Equal(anchor.Add(30 * 24 * time.Hour)) {
		t.Fatal(got)
	}
}
func TestBankEncryption(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	t.Setenv("FINANCE_BANK_KEYS", `{"v1":"`+key+`"}`)
	t.Setenv("FINANCE_BANK_ACTIVE_KEY", "v1")
	input := BankInput{"Test Holder", "123456789012", "HDFC0000123"}
	raw, id, err := encryptBank("user", "bank", input)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := decryptBank("user", "bank", id, raw)
	if err != nil || plain != input {
		t.Fatal("bank encryption failed", err)
	}
	if _, err = decryptBank("other", "bank", id, raw); err == nil {
		t.Fatal("bank ciphertext transferable across owners")
	}
	raw[len(raw)-1] ^= 1
	if _, err = decryptBank("user", "bank", id, raw); err == nil {
		t.Fatal("tampering accepted")
	}
}
func TestCaptureLedgerRecognitionAndImmutability(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if err := devseed.Seed(ctx, p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	var order string
	if err := p.QueryRow(ctx, `INSERT INTO public.orders(customer_id,merchant_id,status,total_price,delivery_address) VALUES($1,$2,'CREATED',1000,'{"latitude":34,"longitude":74}') RETURNING id`, devseed.CustomerID, devseed.RestaurantID).Scan(&order); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE finance.policy SET delivery_charge=0,rider_base=0`); err != nil {
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
	var attempt string
	if err = p.QueryRow(ctx, `INSERT INTO finance.payment_attempts(order_id,provider,amount,currency,status,provider_id) VALUES($1,'RAZORPAY',100000,'INR','PENDING','plink_test') RETURNING id`, order).Scan(&attempt); err != nil {
		t.Fatal(err)
	}
	s := New(p)
	event := map[string]any{"id": "event-one", "type": "checkout.session.completed", "data": map[string]any{"object": CheckoutSession{ID: "plink_test", Amount: 100000, Currency: "INR", PaymentStatus: "paid", Reference: order, PaymentIntent: "pay_test"}}}
	raw, _ := json.Marshal(event)
	if err = s.applyConfirmedEvent(ctx, raw); err != nil {
		t.Fatal(err)
	}
	if err = s.applyConfirmedEvent(ctx, raw); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM finance.journals WHERE kind='CAPTURE'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate capture %d %v", count, err)
	}
	if _, err = p.Exec(ctx, `UPDATE public.orders SET status='DELIVERED' WHERE id=$1`, order); err != nil {
		t.Fatal(err)
	}
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = Recognize(ctx, tx, order); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var payable int64
	if err = p.QueryRow(ctx, `SELECT -sum(e.amount) FROM finance.entries e JOIN finance.order_financials f ON f.restaurant_user=e.user_id WHERE f.order_id=$1`, order).Scan(&payable); err != nil || payable != 85000 {
		t.Fatalf("payable=%d %v", payable, err)
	}
	if _, err = p.Exec(ctx, `UPDATE finance.order_financials SET commission=0 WHERE order_id=$1`, order); err == nil {
		t.Fatal("snapshot editable")
	}
	if _, err = p.Exec(ctx, `UPDATE finance.entries SET amount=1`); err == nil {
		t.Fatal("ledger editable")
	}
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO finance.journals(event_key,kind,reference,reason) VALUES('bad','BAD','bad','bad')`)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err == nil {
		t.Fatal("empty journal accepted")
	}
}

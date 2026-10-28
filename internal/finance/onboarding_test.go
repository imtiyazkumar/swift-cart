package finance

import (
	"context"
	"encoding/json"
	"github.com/imtiyazkumar/yember-core/internal/devseed"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"testing"
)

func TestOnboardingCaptureReplayAndReferenceIsolation(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if err := devseed.Seed(ctx, p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := p.QueryRow(ctx, `INSERT INTO finance.onboarding_attempts(restaurant_id,user_id,amount,currency,provider_id,status) VALUES($1,$2,10000,'INR','plink_onboarding','PENDING') RETURNING id`, devseed.RestaurantID, devseed.OwnerID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	s := New(p)
	event := map[string]any{"event": "payment_link.paid", "payload": map[string]any{"payment_link": map[string]any{"entity": map[string]any{"id": "plink_onboarding", "reference_id": id, "amount": 10000, "amount_paid": 10000, "currency": "INR", "status": "paid", "order_id": "order_provider"}}, "payment": map[string]any{"entity": map[string]any{"id": "pay_onboarding", "order_id": "order_provider", "amount": 10000, "currency": "INR", "status": "captured"}}}}
	raw, _ := json.Marshal(event)
	if err := s.ApplyRazorpay(ctx, "onboarding-event-1", raw); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyRazorpay(ctx, "onboarding-event-2", raw); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM finance.journals WHERE kind='ONBOARDING'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("capture duplicated: %d %v", count, err)
	}
	if err := s.captureOnboarding(ctx, PaymentLink{ID: "plink_other", Reference: id, Currency: "INR"}, "pay_other", 10000, "INR"); err == nil {
		t.Fatal("different provider payment accepted")
	}
	if err := s.captureOnboarding(ctx, PaymentLink{ID: "plink_onboarding", Reference: id, Currency: "INR"}, "pay_onboarding", 9999, "INR"); err == nil {
		t.Fatal("incorrect amount accepted")
	}
}

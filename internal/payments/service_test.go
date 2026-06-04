package payments

import (
	"context"
	"testing"
)

type fakePaymentRepo struct {
	intent *Intent
}

func (f *fakePaymentRepo) CreateIntent(ctx context.Context, intent *Intent) error {
	f.intent = intent
	intent.ID = "payment-1"
	return nil
}

func (f *fakePaymentRepo) MarkWebhookProcessed(ctx context.Context, provider, eventID string) error {
	return nil
}

func TestCreateIntentAppliesDefaults(t *testing.T) {
	repo := &fakePaymentRepo{}
	svc := NewService(repo)

	intent, err := svc.CreateIntent(context.Background(), &Intent{OrderID: "order-1", Amount: 14900})
	if err != nil {
		t.Fatalf("create intent failed: %v", err)
	}
	if intent.Provider != "RAZORPAY" || intent.Method != "UPI" || intent.Currency != "INR" || intent.Status != "CREATED" {
		t.Fatalf("defaults not applied: %+v", intent)
	}
	if repo.intent == nil || repo.intent.ID != "payment-1" {
		t.Fatal("expected repository to create intent")
	}
}

func TestCreateIntentRequiresPositiveAmount(t *testing.T) {
	svc := NewService(&fakePaymentRepo{})

	if _, err := svc.CreateIntent(context.Background(), &Intent{OrderID: "order-1"}); err == nil {
		t.Fatal("expected amount validation error")
	}
}

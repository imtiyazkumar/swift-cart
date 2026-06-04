package merchant

import (
	"context"
	"testing"
)

type fakeMerchantRepo struct {
	merchant *Merchant
}

func (f *fakeMerchantRepo) CreateMerchant(ctx context.Context, m *Merchant) (string, error) {
	f.merchant = m
	return "merchant-1", nil
}

func (f *fakeMerchantRepo) GetMerchantByID(ctx context.Context, id string) (*Merchant, error) {
	return f.merchant, nil
}

func (f *fakeMerchantRepo) ListMerchants(ctx context.Context) ([]*Merchant, error) {
	return []*Merchant{f.merchant}, nil
}

func (f *fakeMerchantRepo) UpdateMerchant(ctx context.Context, m *Merchant) error {
	f.merchant = m
	return nil
}

func (f *fakeMerchantRepo) DeleteMerchant(ctx context.Context, id string) error {
	return nil
}

func TestCreateMerchantAppliesOnboardingDefaults(t *testing.T) {
	svc := NewService(&fakeMerchantRepo{})

	created, err := svc.CreateMerchant(context.Background(), &Merchant{Name: "Fresh Bowl", Email: "owner@example.test"})
	if err != nil {
		t.Fatalf("create merchant failed: %v", err)
	}
	if created.MerchantType != "RESTAURANT" || created.OnboardingStatus != "PROFILE_CREATED" || created.ApprovalStatus != "PENDING" {
		t.Fatalf("defaults not applied: %+v", created)
	}
	if created.ID != "merchant-1" {
		t.Fatalf("expected repository id, got %q", created.ID)
	}
}

func TestGetMerchantRequiresID(t *testing.T) {
	svc := NewService(&fakeMerchantRepo{})

	if _, err := svc.GetMerchant(context.Background(), ""); err == nil {
		t.Fatal("expected id validation error")
	}
}

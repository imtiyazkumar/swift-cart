package restaurant

import (
	"context"
	"testing"
)

type fakeRestaurantRepo struct{}

func (f *fakeRestaurantRepo) GetByOwnerID(ctx context.Context, ownerID string) (*RestaurantProfile, error) {
	return &RestaurantProfile{ID: "rest-1", Name: "Ahdoos Restaurant", IsActive: true}, nil
}

func (f *fakeRestaurantRepo) UpdateStatus(ctx context.Context, merchantID string, isActive bool) error {
	return nil
}

func (f *fakeRestaurantRepo) ListCategories(ctx context.Context, merchantID string) ([]*MenuCategory, error) {
	return []*MenuCategory{{ID: "cat-1", Name: "Wazwan"}}, nil
}

func (f *fakeRestaurantRepo) CreateCategory(ctx context.Context, cat *MenuCategory) error {
	return nil
}

func (f *fakeRestaurantRepo) SaveMenuItem(ctx context.Context, item *MenuItemInput) (string, error) {
	return "item-1", nil
}

func (f *fakeRestaurantRepo) ToggleItemAvailability(ctx context.Context, itemID string, isAvailable bool) error {
	return nil
}

func (f *fakeRestaurantRepo) GetIncomingOrders(ctx context.Context, merchantID string) ([]*RestaurantOrder, error) {
	return []*RestaurantOrder{{ID: "order-1", TotalAmount: 450}}, nil
}

func (f *fakeRestaurantRepo) UpdateOrderStatus(ctx context.Context, merchantID, orderID, status string) error {
	return nil
}

func (f *fakeRestaurantRepo) GetDailySummary(ctx context.Context, merchantID string) (*DailySalesSummary, error) {
	return &DailySalesSummary{TotalOrders: 10, TotalSales: 5000}, nil
}

func TestRestaurantServiceGetProfile(t *testing.T) {
	svc := NewService(&fakeRestaurantRepo{})
	profile, err := svc.GetProfileByOwner(context.Background(), "owner-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile.Name != "Ahdoos Restaurant" {
		t.Fatalf("expected Ahdoos Restaurant, got %s", profile.Name)
	}
}

func TestRestaurantServiceValidation(t *testing.T) {
	svc := NewService(&fakeRestaurantRepo{})
	if _, err := svc.SaveMenuItem(context.Background(), &MenuItemInput{}); err == nil {
		t.Fatal("expected validation error for invalid item input")
	}
}

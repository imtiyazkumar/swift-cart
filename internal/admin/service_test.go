package admin

import (
	"context"
	"testing"
)

type fakeAdminRepo struct{}

func (f *fakeAdminRepo) GetStats(ctx context.Context) (*PlatformStats, error) {
	return &PlatformStats{TotalUsers: 100, TotalOrders: 500}, nil
}

func (f *fakeAdminRepo) ListUsers(ctx context.Context) ([]*UserManagementItem, error) {
	return []*UserManagementItem{{ID: "user-1", Email: "test@yember.app"}}, nil
}

func (f *fakeAdminRepo) UpdateUserRole(ctx context.Context, userID, newRole string) error {
	return nil
}

func (f *fakeAdminRepo) ListPendingMerchants(ctx context.Context) ([]*PendingMerchant, error) {
	return []*PendingMerchant{{ID: "m-1", Name: "New Resto"}}, nil
}

func (f *fakeAdminRepo) ApproveMerchant(ctx context.Context, merchantID, status string) error {
	return nil
}

func (f *fakeAdminRepo) ListPendingRiders(ctx context.Context) ([]*PendingRider, error) {
	return []*PendingRider{{ID: "r-1", Name: "New Rider"}}, nil
}

func (f *fakeAdminRepo) ApproveRider(ctx context.Context, partnerID, status string) error {
	return nil
}

func (f *fakeAdminRepo) CreateCoupon(ctx context.Context, c *CouponInput) error {
	return nil
}

func (f *fakeAdminRepo) ListCoupons(ctx context.Context) ([]*CouponInput, error) {
	return []*CouponInput{{Code: "VALLEY20"}}, nil
}

func (f *fakeAdminRepo) DeleteCoupon(ctx context.Context, couponID string) error {
	return nil
}

func TestAdminServiceGetStats(t *testing.T) {
	svc := NewService(&fakeAdminRepo{})
	stats, err := svc.GetDashboardStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalUsers != 100 {
		t.Fatalf("expected 100 users, got %d", stats.TotalUsers)
	}
}

func TestAdminServiceValidation(t *testing.T) {
	svc := NewService(&fakeAdminRepo{})
	if err := svc.UpdateUserRole(context.Background(), "", "ADMIN"); err == nil {
		t.Fatal("expected validation error for empty userID")
	}
}

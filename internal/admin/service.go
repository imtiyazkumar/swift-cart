package admin

import (
    "context"
    "errors"
)

type Service struct {
    repo Repository
}

func NewService(r Repository) *Service {
    return &Service{repo: r}
}

func (s *Service) GetDashboardStats(ctx context.Context) (*PlatformStats, error) {
    return s.repo.GetStats(ctx)
}

func (s *Service) ListUsers(ctx context.Context) ([]*UserManagementItem, error) {
    return s.repo.ListUsers(ctx)
}

func (s *Service) UpdateUserRole(ctx context.Context, userID, newRole string) error {
    if userID == "" || newRole == "" {
        return errors.New("userID and newRole required")
    }
    return s.repo.UpdateUserRole(ctx, userID, newRole)
}

func (s *Service) ListPendingMerchants(ctx context.Context) ([]*PendingMerchant, error) {
    return s.repo.ListPendingMerchants(ctx)
}

func (s *Service) ApproveMerchant(ctx context.Context, merchantID, status string) error {
    if merchantID == "" || status == "" {
        return errors.New("merchantID and status required")
    }
    return s.repo.ApproveMerchant(ctx, merchantID, status)
}

func (s *Service) ListPendingRiders(ctx context.Context) ([]*PendingRider, error) {
    return s.repo.ListPendingRiders(ctx)
}

func (s *Service) ApproveRider(ctx context.Context, partnerID, status string) error {
    if partnerID == "" || status == "" {
        return errors.New("partnerID and status required")
    }
    return s.repo.ApproveRider(ctx, partnerID, status)
}

func (s *Service) CreateCoupon(ctx context.Context, coupon *CouponInput) error {
    if coupon.Code == "" || coupon.DiscountValue <= 0 {
        return errors.New("valid code and discount value required")
    }
    return s.repo.CreateCoupon(ctx, coupon)
}

func (s *Service) ListCoupons(ctx context.Context) ([]*CouponInput, error) {
    return s.repo.ListCoupons(ctx)
}

func (s *Service) DeleteCoupon(ctx context.Context, couponID string) error {
    if couponID == "" {
        return errors.New("couponID required")
    }
    return s.repo.DeleteCoupon(ctx, couponID)
}

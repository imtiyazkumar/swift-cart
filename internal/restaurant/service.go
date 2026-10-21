package restaurant

import (
    "context"
    "errors"
    "math"
)

type Service struct {
    repo Repository
}

func NewService(r Repository) *Service {
    return &Service{repo: r}
}

func (s *Service) GetProfileByOwner(ctx context.Context, ownerID string) (*RestaurantProfile, error) {
    if ownerID == "" {
        return nil, errors.New("ownerID required")
    }
    return s.repo.GetByOwnerID(ctx, ownerID)
}

func (s *Service) UpdateStatus(ctx context.Context, merchantID string, isActive bool) error {
    if merchantID == "" {
        return errors.New("merchantID required")
    }
    return s.repo.UpdateStatus(ctx, merchantID, isActive)
}

func (s *Service) ListCategories(ctx context.Context, merchantID string) ([]*MenuCategory, error) {
    if merchantID == "" {
        return nil, errors.New("merchantID required")
    }
    return s.repo.ListCategories(ctx, merchantID)
}

func (s *Service) CreateCategory(ctx context.Context, cat *MenuCategory) error {
    if cat.MerchantID == "" || cat.Name == "" {
        return errors.New("merchantID and category name required")
    }
    return s.repo.CreateCategory(ctx, cat)
}

func (s *Service) SaveMenuItem(ctx context.Context, item *MenuItemInput) (string, error) {
    if item.MerchantID == "" || item.Name == "" || item.Price <= 0 || item.Price > 10000000 || math.IsNaN(item.Price) || math.IsInf(item.Price, 0) || math.Abs(item.Price*100-math.Round(item.Price*100)) > 0.00001 {
        return "", errors.New("merchantID, name, and valid price required")
    }
    return s.repo.SaveMenuItem(ctx, item)
}

func (s *Service) ToggleItemAvailability(ctx context.Context, itemID string, isAvailable bool) error {
    if itemID == "" {
        return errors.New("itemID required")
    }
    return s.repo.ToggleItemAvailability(ctx, itemID, isAvailable)
}

func (s *Service) GetIncomingOrders(ctx context.Context, merchantID string) ([]*RestaurantOrder, error) {
    if merchantID == "" {
        return nil, errors.New("merchantID required")
    }
    return s.repo.GetIncomingOrders(ctx, merchantID)
}

func (s *Service) UpdateOrderStatus(ctx context.Context, merchantID, orderID, status string) error {
    if merchantID == "" || orderID == "" || status == "" {
        return errors.New("merchantID, orderID, and status required")
    }
    return s.repo.UpdateOrderStatus(ctx, merchantID, orderID, status)
}

func (s *Service) GetDailySummary(ctx context.Context, merchantID string) (*DailySalesSummary, error) {
    if merchantID == "" {
        return nil, errors.New("merchantID required")
    }
    return s.repo.GetDailySummary(ctx, merchantID)
}

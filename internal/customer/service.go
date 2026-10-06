package customer

import (
    "context"
    "errors"
    "time"

    "github.com/google/uuid"
)

type Service struct {
    repo Repository
}

func NewService(r Repository) *Service {
    return &Service{repo: r}
}

func (s *Service) CreateCustomer(ctx context.Context, c *Customer) (*Customer, error) {
    if c.Name == "" || c.Email == "" {
        return nil, errors.New("name and email are required")
    }
    if c.ID == "" {
        c.ID = uuid.NewString()
    }
    now := time.Now().UTC()
    c.CreatedAt = now
    c.UpdatedAt = now
    id, err := s.repo.CreateCustomer(ctx, c)
    if err != nil {
        return nil, err
    }
    c.ID = id
    return c, nil
}

func (s *Service) GetCustomer(ctx context.Context, id string) (*Customer, error) {
    if id == "" {
        return nil, errors.New("id is required")
    }
    return s.repo.GetCustomerByID(ctx, id)
}

func (s *Service) ListCustomers(ctx context.Context) ([]*Customer, error) {
    return s.repo.ListCustomers(ctx)
}

func (s *Service) UpdateCustomer(ctx context.Context, c *Customer) error {
    if c.ID == "" {
        return errors.New("id is required")
    }
    c.UpdatedAt = time.Now().UTC()
    return s.repo.UpdateCustomer(ctx, c)
}

func (s *Service) DeleteCustomer(ctx context.Context, id string) error {
    if id == "" {
        return errors.New("id is required")
    }
    return s.repo.DeleteCustomer(ctx, id)
}

// Ecosystem Service Methods
func (s *Service) ListRestaurants(ctx context.Context, city, search string) ([]*Restaurant, error) {
    return s.repo.ListRestaurants(ctx, city, search)
}

func (s *Service) GetRestaurant(ctx context.Context, id string) (*Restaurant, error) {
    if id == "" {
        return nil, errors.New("restaurant id is required")
    }
    return s.repo.GetRestaurantByID(ctx, id)
}

func (s *Service) ToggleFavorite(ctx context.Context, customerID, merchantID string) (bool, error) {
    if customerID == "" || merchantID == "" {
        return false, errors.New("customerID and merchantID are required")
    }
    return s.repo.ToggleFavorite(ctx, customerID, merchantID)
}

func (s *Service) GetFavorites(ctx context.Context, customerID string) ([]string, error) {
    if customerID == "" {
        return nil, errors.New("customerID is required")
    }
    return s.repo.GetFavorites(ctx, customerID)
}

func (s *Service) ValidateCoupon(ctx context.Context, code string, amount int64) (*Coupon, int64, error) {
    if code == "" {
        return nil, 0, errors.New("coupon code is required")
    }
    return s.repo.ValidateCoupon(ctx, code, amount)
}

func (s *Service) SubmitReview(ctx context.Context, customerID string, req *ReviewRequest) error {
    if req.OrderID == "" || req.Rating < 1 || req.Rating > 5 {
        return errors.New("valid orderID and rating (1-5) are required")
    }
    return s.repo.SubmitReview(ctx, customerID, req)
}

func (s *Service) GetOrderTracking(ctx context.Context, orderID string) (*OrderTracking, error) {
    if orderID == "" {
        return nil, errors.New("orderID is required")
    }
    return s.repo.GetOrderTracking(ctx, orderID)
}

func (s *Service) GetNotifications(ctx context.Context, customerID string) ([]*NotificationItem, error) {
    return s.repo.GetNotifications(ctx, customerID)
}

func (s *Service) AddAddress(ctx context.Context, customerID, label, addressLine1, city, state, postalCode string) error {
    if customerID == "" || addressLine1 == "" {
        return errors.New("customerID and address are required")
    }
    return s.repo.AddAddress(ctx, customerID, label, addressLine1, city, state, postalCode)
}

func (s *Service) ListAddresses(ctx context.Context, customerID string) ([]map[string]string, error) {
    if customerID == "" {
        return nil, errors.New("customerID is required")
    }
    return s.repo.ListAddresses(ctx, customerID)
}


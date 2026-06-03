package order

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

// CreateOrder validates and creates a new order.
func (s *Service) CreateOrder(ctx context.Context, o *Order) (*Order, error) {
    if o.MerchantID == "" || o.CustomerID == "" {
        return nil, errors.New("merchant_id and customer_id are required")
    }
    if o.ID == "" {
        o.ID = uuid.NewString()
    }
    now := time.Now().UTC()
    o.CreatedAt = now
    o.UpdatedAt = now
    o.Status = "pending"
    if err := s.repo.Create(ctx, o); err != nil {
        return nil, err
    }
    return o, nil
}

func (s *Service) GetOrder(ctx context.Context, id string) (*Order, error) {
    if id == "" {
        return nil, errors.New("id is required")
    }
    return s.repo.GetByID(ctx, id)
}

func (s *Service) ListOrders(ctx context.Context) ([]*Order, error) {
    return s.repo.List(ctx)
}

func (s *Service) UpdateOrderStatus(ctx context.Context, id, status string) error {
    if id == "" || status == "" {
        return errors.New("id and status are required")
    }
    return s.repo.UpdateStatus(ctx, id, status)
}

func (s *Service) DeleteOrder(ctx context.Context, id string) error {
    if id == "" {
        return errors.New("id is required")
    }
    return s.repo.Delete(ctx, id)
}

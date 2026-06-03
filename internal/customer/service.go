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

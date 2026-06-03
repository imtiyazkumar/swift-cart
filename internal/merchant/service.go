package merchant

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

// CreateMerchant validates and creates a new merchant.
func (s *Service) CreateMerchant(ctx context.Context, m *Merchant) (*Merchant, error) {
    if m.Name == "" || m.Email == "" {
        return nil, errors.New("name and email are required")
    }
    // generate ID if not set
    if m.ID == "" {
        m.ID = uuid.NewString()
    }
    // set timestamps
    now := time.Now().UTC()
    m.CreatedAt = now
    m.UpdatedAt = now
    id, err := s.repo.CreateMerchant(ctx, m)
    if err != nil {
        return nil, err
    }
    m.ID = id
    return m, nil
}

func (s *Service) GetMerchant(ctx context.Context, id string) (*Merchant, error) {
    if id == "" {
        return nil, errors.New("id is required")
    }
    return s.repo.GetMerchantByID(ctx, id)
}

func (s *Service) ListMerchants(ctx context.Context) ([]*Merchant, error) {
    return s.repo.ListMerchants(ctx)
}

func (s *Service) UpdateMerchant(ctx context.Context, m *Merchant) error {
    if m.ID == "" {
        return errors.New("id is required")
    }
    m.UpdatedAt = time.Now().UTC()
    return s.repo.UpdateMerchant(ctx, m)
}

func (s *Service) DeleteMerchant(ctx context.Context, id string) error {
    if id == "" {
        return errors.New("id is required")
    }
    return s.repo.DeleteMerchant(ctx, id)
}

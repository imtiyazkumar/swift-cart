package catalog

import (
	"context"
	"errors"
	"strings"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateCategory(ctx context.Context, c *Category) (*Category, error) {
	if c.MerchantID == "" || strings.TrimSpace(c.Name) == "" {
		return nil, errors.New("merchant_id and name are required")
	}
	c.IsActive = true
	if err := s.repo.CreateCategory(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) CreateItem(ctx context.Context, item *Item) (*Item, error) {
	if item.MerchantID == "" || item.CategoryID == "" || strings.TrimSpace(item.Name) == "" {
		return nil, errors.New("merchant_id, category_id and name are required")
	}
	if item.Currency == "" {
		item.Currency = "INR"
	}
	if item.ItemType == "" {
		item.ItemType = "STANDARD"
	}
	if item.SubstitutionPolicy == "" {
		item.SubstitutionPolicy = "NONE"
	}
	item.IsAvailable = true
	if item.BasePrice < 0 {
		return nil, errors.New("base_price cannot be negative")
	}
	if err := s.repo.CreateItem(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) GetItem(ctx context.Context, id string) (*Item, error) {
	if id == "" {
		return nil, errors.New("id is required")
	}
	return s.repo.GetItem(ctx, id)
}

func (s *Service) ListMerchantItems(ctx context.Context, merchantID string) ([]*Item, error) {
	if merchantID == "" {
		return nil, errors.New("merchant_id is required")
	}
	return s.repo.ListMerchantItems(ctx, merchantID)
}

func (s *Service) SearchItems(ctx context.Context, q string, limit int) ([]*Item, error) {
	return s.repo.SearchItems(ctx, strings.TrimSpace(q), limit)
}

func (s *Service) UpdateItem(ctx context.Context, item *Item) error {
	if item.ID == "" {
		return errors.New("id is required")
	}
	return s.repo.UpdateItem(ctx, item)
}

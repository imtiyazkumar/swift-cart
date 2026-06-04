package payments

import (
	"context"
	"errors"
)

type Repository interface {
	CreateIntent(ctx context.Context, intent *Intent) error
	MarkWebhookProcessed(ctx context.Context, provider, eventID string) error
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateIntent(ctx context.Context, intent *Intent) (*Intent, error) {
	if intent.OrderID == "" || intent.Amount <= 0 {
		return nil, errors.New("order_id and positive amount are required")
	}
	if intent.Currency == "" {
		intent.Currency = "INR"
	}
	if intent.Provider == "" {
		intent.Provider = "RAZORPAY"
	}
	if intent.Method == "" {
		intent.Method = "UPI"
	}
	intent.Status = "CREATED"
	if err := s.repo.CreateIntent(ctx, intent); err != nil {
		return nil, err
	}
	return intent, nil
}

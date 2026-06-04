package notifications

import (
	"context"
	"errors"

	"github.com/imtiyazkumar/swiftkart/pkg/events"
)

type Repository interface {
	Log(ctx context.Context, n *Notification) error
}

type Service struct {
	repo Repository
	bus  events.Bus
}

func NewService(repo Repository, bus events.Bus) *Service {
	return &Service{repo: repo, bus: bus}
}

func (s *Service) Send(ctx context.Context, n *Notification) error {
	if n.UserID == "" || n.Channel == "" || n.TemplateKey == "" {
		return errors.New("user_id, channel and template_key are required")
	}
	n.Status = "QUEUED"
	return s.repo.Log(ctx, n)
}

func (s *Service) SubscribeOrderEvents() {
	if s.bus == nil {
		return
	}
	s.bus.Subscribe("ORDER_CREATED", func(ctx context.Context, event events.Event) error {
		userID, _ := event.Payload["customer_id"].(string)
		return s.Send(ctx, &Notification{UserID: userID, Channel: "PUSH", TemplateKey: "order_created", Payload: event.Payload})
	})
}

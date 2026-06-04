package notifications

import (
	"context"
	"testing"

	"github.com/imtiyazkumar/swiftkart/pkg/events"
)

type fakeNotificationRepo struct {
	notification *Notification
}

func (f *fakeNotificationRepo) Log(ctx context.Context, n *Notification) error {
	f.notification = n
	n.ID = "notification-1"
	return nil
}

func TestSendQueuesNotification(t *testing.T) {
	repo := &fakeNotificationRepo{}
	svc := NewService(repo, nil)

	err := svc.Send(context.Background(), &Notification{UserID: "user-1", Channel: "PUSH", TemplateKey: "order_created"})
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if repo.notification == nil || repo.notification.Status != "QUEUED" {
		t.Fatalf("expected queued notification, got %+v", repo.notification)
	}
}

func TestSendRequiresRequiredFields(t *testing.T) {
	svc := NewService(&fakeNotificationRepo{}, nil)

	if err := svc.Send(context.Background(), &Notification{UserID: "user-1"}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestSubscribeOrderEventsLogsNotification(t *testing.T) {
	repo := &fakeNotificationRepo{}
	bus := events.NewInMemoryBus()
	svc := NewService(repo, bus)
	svc.SubscribeOrderEvents()

	err := bus.Publish(context.Background(), events.Event{
		Type:    "ORDER_CREATED",
		Payload: map[string]interface{}{"customer_id": "customer-1", "order_id": "order-1"},
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if repo.notification == nil || repo.notification.UserID != "customer-1" || repo.notification.TemplateKey != "order_created" {
		t.Fatalf("expected order notification, got %+v", repo.notification)
	}
}

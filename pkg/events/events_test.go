package events

import (
	"context"
	"testing"
)

func TestInMemoryBusPublishesToSpecificAndWildcardHandlers(t *testing.T) {
	bus := NewInMemoryBus()
	var calls int

	bus.Subscribe("ORDER_CREATED", func(ctx context.Context, event Event) error {
		calls++
		if event.Type != "ORDER_CREATED" {
			t.Fatalf("unexpected event type %q", event.Type)
		}
		return nil
	})
	bus.Subscribe("*", func(ctx context.Context, event Event) error {
		calls++
		return nil
	})

	if err := bus.Publish(context.Background(), Event{Type: "ORDER_CREATED"}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 handler calls, got %d", calls)
	}
}

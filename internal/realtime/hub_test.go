package realtime

import (
	"context"
	"testing"
	"time"

	"github.com/imtiyazkumar/swiftkart/pkg/events"
)

func TestHubPublishesToSubscribedChannel(t *testing.T) {
	hub := NewHub()
	ch, cancel := hub.Subscribe("order:1")
	defer cancel()

	hub.Publish(context.Background(), "order:1", events.Event{Type: "ORDER_CREATED"})

	select {
	case event := <-ch:
		if event.Type != "ORDER_CREATED" {
			t.Fatalf("unexpected event type %q", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestHubCancelUnsubscribesChannel(t *testing.T) {
	hub := NewHub()
	ch, cancel := hub.Subscribe("order:1")
	cancel()

	hub.Publish(context.Background(), "order:1", events.Event{Type: "ORDER_CREATED"})

	if _, ok := <-ch; ok {
		t.Fatal("expected channel to be closed")
	}
}

package events

import (
	"context"
	"sync"
	"time"
)

type Event struct {
	ID            string                 `json:"id"`
	Type          string                 `json:"type"`
	AggregateID   string                 `json:"aggregate_id"`
	AggregateType string                 `json:"aggregate_type"`
	Payload       map[string]interface{} `json:"payload"`
	OccurredAt    time.Time              `json:"occurred_at"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
}

type Handler func(ctx context.Context, event Event) error

type Bus interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(eventType string, handler Handler)
}

type InMemoryBus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{handlers: make(map[string][]Handler)}
}

func (b *InMemoryBus) Subscribe(eventType string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

func (b *InMemoryBus) Publish(ctx context.Context, event Event) error {
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}

	b.mu.RLock()
	handlers := append([]Handler(nil), b.handlers[event.Type]...)
	handlers = append(handlers, b.handlers["*"]...)
	b.mu.RUnlock()

	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

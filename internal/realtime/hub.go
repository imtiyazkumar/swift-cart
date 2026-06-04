package realtime

import (
	"context"
	"sync"

	"github.com/yourorg/swiftkart/pkg/events"
)

type Hub struct {
	mu       sync.RWMutex
	channels map[string][]chan events.Event
}

func NewHub() *Hub {
	return &Hub{channels: make(map[string][]chan events.Event)}
}

func (h *Hub) Subscribe(channel string) (<-chan events.Event, func()) {
	ch := make(chan events.Event, 16)
	h.mu.Lock()
	h.channels[channel] = append(h.channels[channel], ch)
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		subs := h.channels[channel]
		for i, sub := range subs {
			if sub == ch {
				h.channels[channel] = append(subs[:i], subs[i+1:]...)
				close(ch)
				return
			}
		}
	}
	return ch, cancel
}

func (h *Hub) Publish(ctx context.Context, channel string, event events.Event) {
	h.mu.RLock()
	subs := append([]chan events.Event(nil), h.channels[channel]...)
	h.mu.RUnlock()
	for _, sub := range subs {
		select {
		case sub <- event:
		case <-ctx.Done():
			return
		default:
		}
	}
}

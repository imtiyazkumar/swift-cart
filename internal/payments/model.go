package payments

import (
	"context"
	"time"
)

type Intent struct {
	ID             string    `json:"id"`
	OrderID        string    `json:"order_id"`
	Provider       string    `json:"provider"`
	Method         string    `json:"method"`
	Amount         int64     `json:"amount"`
	Currency       string    `json:"currency"`
	Status         string    `json:"status"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}

type Gateway interface {
	CreateIntent(ctx context.Context, intent Intent) (Intent, error)
	ValidateWebhook(signature string, payload []byte) error
}

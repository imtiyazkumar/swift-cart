package notifications

import "time"

type Notification struct {
	ID          string                 `json:"id"`
	UserID      string                 `json:"user_id"`
	Channel     string                 `json:"channel"`
	TemplateKey string                 `json:"template_key"`
	Payload     map[string]interface{} `json:"payload"`
	Status      string                 `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
}

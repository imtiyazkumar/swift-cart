package delivery

import "time"

const (
	PartnerOnline  = "ONLINE"
	PartnerOffline = "OFFLINE"
)

type Partner struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	Name           string    `json:"name"`
	Phone          string    `json:"phone"`
	Status         string    `json:"status"`
	ApprovalStatus string    `json:"approval_status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type LocationUpdate struct {
	PartnerID  string    `json:"partner_id"`
	Latitude   float64   `json:"latitude"`
	Longitude  float64   `json:"longitude"`
	RecordedAt time.Time `json:"recorded_at"`
}

type Assignment struct {
	ID        string    `json:"id"`
	OrderID   string    `json:"order_id"`
	PartnerID string    `json:"partner_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

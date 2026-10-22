package rider

import "time"

type RiderProfile struct {
    ID                   string     `json:"id"`
    UserID               string     `json:"user_id"`
    Name                 string     `json:"name"`
    Phone                string     `json:"phone"`
    Status               string     `json:"status"`          // ONLINE, OFFLINE, BUSY
    ApprovalStatus       string     `json:"approval_status"` // PENDING, APPROVED, REJECTED
    VehicleType          string     `json:"vehicle_type,omitempty"`
    RegNumber            string     `json:"registration_number,omitempty"`
    CreatedAt            time.Time  `json:"created_at"`
    EligibilityExpiresAt *time.Time `json:"eligibility_expires_at,omitempty"`
}

type LocationUpdate struct {
    Latitude  float64 `json:"latitude"`
    Longitude float64 `json:"longitude"`
}

type DeliveryJob struct {
    OrderID         string     `json:"order_id"`
    MerchantName    string     `json:"merchant_name"`
    MerchantAddress string     `json:"merchant_address"`
    CustomerName    string     `json:"customer_name"`
    CustomerAddress string     `json:"customer_address"`
    DeliveryFee     int64      `json:"delivery_fee"`
    Status          string     `json:"status"` // ASSIGNED, PICKED_UP, OUT_FOR_DELIVERY, DELIVERED
    CreatedAt       time.Time  `json:"created_at"`
    ExpiresAt       *time.Time `json:"expires_at,omitempty"`
}

type RiderEarningsSummary struct {
    TotalDeliveries int64 `json:"total_deliveries"`
    TotalEarnings   int64 `json:"total_earnings"`
    TodayEarnings   int64 `json:"today_earnings"`
    TipsCollected   int64 `json:"tips_collected"`
}

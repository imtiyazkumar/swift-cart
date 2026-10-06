package admin

import "time"

type PlatformStats struct {
    TotalUsers       int64 `json:"total_users"`
    TotalMerchants   int64 `json:"total_merchants"`
    TotalRiders      int64 `json:"total_riders"`
    TotalOrders      int64 `json:"total_orders"`
    TotalRevenue     int64 `json:"total_revenue"`
    ActiveDeliveries int64 `json:"active_deliveries"`
}

type UserManagementItem struct {
    ID        string    `json:"id"`
    Email     string    `json:"email"`
    FullName  string    `json:"full_name"`
    Phone     string    `json:"phone"`
    Role      string    `json:"role"`
    CreatedAt time.Time `json:"created_at"`
}

type PendingMerchant struct {
    ID               string    `json:"id"`
    Name             string    `json:"name"`
    OwnerEmail       string    `json:"owner_email"`
    MerchantType     string    `json:"merchant_type"`
    OnboardingStatus string    `json:"onboarding_status"`
    ApprovalStatus   string    `json:"approval_status"`
    CreatedAt        time.Time `json:"created_at"`
}

type PendingRider struct {
    ID             string    `json:"id"`
    Name           string    `json:"name"`
    Phone          string    `json:"phone"`
    ApprovalStatus string    `json:"approval_status"`
    VehicleType    string    `json:"vehicle_type,omitempty"`
    CreatedAt      time.Time `json:"created_at"`
}

type CouponInput struct {
    ID                string    `json:"id,omitempty"`
    Code              string    `json:"code"`
    Description       string    `json:"description"`
    DiscountType      string    `json:"discount_type"` // PERCENTAGE or FLAT
    DiscountValue     int64     `json:"discount_value"`
    MinOrderAmount    int64     `json:"min_order_amount"`
    MaxDiscountAmount int64     `json:"max_discount_amount"`
    IsActive          bool      `json:"is_active"`
    ValidUntil        time.Time `json:"valid_until,omitempty"`
}

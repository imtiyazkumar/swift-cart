package restaurant

import "time"

type RestaurantProfile struct {
    ID               string    `json:"id"`
    OwnerID          string    `json:"owner_id"`
    Name             string    `json:"name"`
    MerchantType     string    `json:"merchant_type"`
    IsActive         bool      `json:"is_active"`
    OnboardingStatus string    `json:"onboarding_status"`
    ApprovalStatus   string    `json:"approval_status"`
    FeePaymentStatus string    `json:"fee_payment_status"`
    Tags             []string  `json:"tags"`
    BranchAddress    string    `json:"branch_address,omitempty"`
    CreatedAt        time.Time `json:"created_at"`
}

type MenuCategory struct {
    ID         string    `json:"id"`
    MerchantID string    `json:"merchant_id"`
    Name       string    `json:"name"`
    SortOrder  int       `json:"sort_order"`
    IsActive   bool      `json:"is_active"`
    CreatedAt  time.Time `json:"created_at"`
}

type MenuItemInput struct {
    ID          string  `json:"id,omitempty"`
    MerchantID  string  `json:"merchant_id"`
    CategoryID  string  `json:"category_id"`
    Name        string  `json:"name"`
    Description string  `json:"description"`
    IsVeg       bool    `json:"is_veg"`
    Price       float64 `json:"price"` // In currency units e.g. 450.00
    IsAvailable bool    `json:"is_available"`
    ImageURL    string  `json:"image_url,omitempty"`
}

type RestaurantOrder struct {
    ID            string    `json:"id"`
    CustomerName  string    `json:"customer_name"`
    CustomerPhone string    `json:"customer_phone"`
    ItemsSummary  string    `json:"items_summary"`
    TotalAmount   int64     `json:"total_amount"`
    Status        string    `json:"status"`
    Notes         string    `json:"notes,omitempty"`
    CreatedAt     time.Time `json:"created_at"`
}

type DailySalesSummary struct {
    TotalOrders int64 `json:"total_orders"`
    TotalSales  int64 `json:"total_sales"`
    AvgOrderVal int64 `json:"avg_order_val"`
}

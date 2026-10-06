package customer

import "time"

type Customer struct {
    ID        string    `json:"id"`
    UserID    string    `json:"user_id,omitempty"`
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    Phone     string    `json:"phone"`
    Gender    string    `json:"gender,omitempty"`
    Address   string    `json:"address,omitempty"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

type MenuItem struct {
    ID          string  `json:"id"`
    Name        string  `json:"name"`
    Description string  `json:"description"`
    Price       float64 `json:"price"`
    Image       string  `json:"image"`
    IsVeg       bool    `json:"isVeg"`
    Rating      string  `json:"rating,omitempty"`
    Reviews     string  `json:"reviews,omitempty"`
    Tag         string  `json:"tag,omitempty"`
}

type Restaurant struct {
    ID       string     `json:"id"`
    Name     string     `json:"name"`
    Cuisine  string     `json:"cuisine"`
    Rating   string     `json:"rating"`
    Reviews  string     `json:"reviews"`
    Time     string     `json:"time"`
    Distance string     `json:"distance"`
    Fee      string     `json:"fee"`
    Offer    string     `json:"offer"`
    Image    string     `json:"image"`
    Accent   string     `json:"accent"`
    Menu     []MenuItem `json:"menu"`
}

type Coupon struct {
    ID                string    `json:"id"`
    Code              string    `json:"code"`
    Description       string    `json:"description"`
    DiscountType      string    `json:"discount_type"`
    DiscountValue     int64     `json:"discount_value"`
    MinOrderAmount    int64     `json:"min_order_amount"`
    MaxDiscountAmount int64     `json:"max_discount_amount,omitempty"`
    IsActive          bool      `json:"is_active"`
    ValidUntil        time.Time `json:"valid_until,omitempty"`
}

type ReviewRequest struct {
    OrderID string `json:"order_id"`
    Rating  int    `json:"rating"`
    Comment string `json:"comment"`
}

type OrderTracking struct {
    OrderID           string    `json:"order_id"`
    Status            string    `json:"status"`
    MerchantName      string    `json:"merchant_name"`
    MerchantAddress   string    `json:"merchant_address"`
    RiderName         string    `json:"rider_name,omitempty"`
    RiderPhone        string    `json:"rider_phone,omitempty"`
    RiderLatitude     float64   `json:"rider_latitude,omitempty"`
    RiderLongitude    float64   `json:"rider_longitude,omitempty"`
    EstimatedMinutes  int       `json:"estimated_minutes"`
    UpdatedAt         time.Time `json:"updated_at"`
}

type NotificationItem struct {
    ID          string    `json:"id"`
    UserID      string    `json:"user_id"`
    Title       string    `json:"title"`
    Message     string    `json:"message"`
    Channel     string    `json:"channel"`
    TemplateKey string    `json:"template_key"`
    Status      string    `json:"status"`
    CreatedAt   time.Time `json:"created_at"`
    Read        bool      `json:"read"`
}


package order

import (
    "time"
)

type Order struct {
    ID         string    `json:"id"`
    MerchantID string    `json:"merchant_id"`
    CustomerID string    `json:"customer_id"`
    Status     string    `json:"status"`
    TotalPrice float64   `json:"total_price"`
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`
}

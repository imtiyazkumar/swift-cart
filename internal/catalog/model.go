package catalog

import "time"

const (
	MerchantTypeRestaurant = "RESTAURANT"
	MerchantTypeGrocery    = "GROCERY"
	MerchantTypePharmacy   = "PHARMACY"
	MerchantTypePetStore   = "PET_STORE"
	MerchantTypeFlowerShop = "FLOWER_SHOP"
)

type Category struct {
	ID         string    `json:"id"`
	MerchantID string    `json:"merchant_id"`
	Name       string    `json:"name"`
	SortOrder  int       `json:"sort_order"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Item struct {
	ID                 string    `json:"id"`
	MerchantID         string    `json:"merchant_id"`
	CategoryID         string    `json:"category_id"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	ItemType           string    `json:"item_type"`
	IsVegetarian       *bool     `json:"is_vegetarian,omitempty"`
	BasePrice          int64     `json:"base_price"`
	Currency           string    `json:"currency"`
	IsAvailable        bool      `json:"is_available"`
	InventoryTracked   bool      `json:"inventory_tracked"`
	SubstitutionPolicy string    `json:"substitution_policy"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type Variant struct {
	ID        string    `json:"id"`
	ItemID    string    `json:"item_id"`
	Name      string    `json:"name"`
	Price     int64     `json:"price"`
	SKU       string    `json:"sku"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

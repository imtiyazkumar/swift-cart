package merchant

import "time"

type Merchant struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Email            string    `json:"email"`
	Phone            string    `json:"phone"`
	MerchantType     string    `json:"merchant_type"`
	OnboardingStatus string    `json:"onboarding_status"`
	ApprovalStatus   string    `json:"approval_status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

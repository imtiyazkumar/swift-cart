package finance

import (
	"context"
	"errors"
	"github.com/imtiyazkumar/yember-core/internal/stations"
	"github.com/jackc/pgx/v5"
	"net/url"
	"time"
)

type OnboardingCheckout struct {
	URL    string `json:"url"`
	Status string `json:"status"`
}

func (s *Service) OnboardingCheckout(ctx context.Context, user string) (*OnboardingCheckout, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var merchant, approval, paid string
	if err = tx.QueryRow(ctx, `SELECT id,approval_status,fee_payment_status FROM public.merchants WHERE owner_id=$1 AND deleted_at IS NULL ORDER BY created_at LIMIT 1 FOR UPDATE`, user).Scan(&merchant, &approval, &paid); err != nil {
		return nil, errors.New("restaurant not found")
	}
	if paid == "PAID" {
		return &OnboardingCheckout{Status: "PAID"}, nil
	}
	if approval != "APPROVED" {
		return nil, errors.New("restaurant approval required")
	}
	if err = stations.ValidateRestaurant(ctx, tx, merchant); err != nil {
		return nil, err
	}
	var id, status, providerID, linkURL string
	var expiry *time.Time
	err = tx.QueryRow(ctx, `SELECT id,status,COALESCE(provider_id,''),COALESCE(checkout_url,''),expires_at FROM finance.onboarding_attempts WHERE merchant_id=$1 ORDER BY created_at DESC LIMIT 1`, merchant).Scan(&id, &status, &providerID, &linkURL, &expiry)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}
	if status == "PAID" {
		return &OnboardingCheckout{Status: "PAID"}, nil
	}
	if id != "" && status != "EXPIRED" {
		if providerID == "" {
			return nil, errors.New("onboarding payment creation requires reconciliation")
		}
		if expiry != nil && time.Now().Before(*expiry) {
			return &OnboardingCheckout{URL: linkURL, Status: "PENDING"}, nil
		}
		_ = tx.Rollback(ctx)
		var link PaymentLink
		if err = s.Gateway.request(ctx, "GET", "/payment_links/"+url.PathEscape(providerID), "", nil, &link); err != nil {
			return nil, err
		}
		if link.Status != "expired" && link.Status != "cancelled" {
			return nil, errors.New("existing onboarding payment awaits provider confirmation")
		}
		if _, err = s.DB.Exec(ctx, `UPDATE finance.onboarding_attempts SET status='EXPIRED' WHERE id=$1 AND status='PENDING'`, id); err != nil {
			return nil, err
		}
		return s.OnboardingCheckout(ctx, user)
	}
	var amount int64
	var currency string
	if err = tx.QueryRow(ctx, `SELECT (value->>'amount')::bigint,value->>'currency' FROM public.platform_settings WHERE key='onboarding_fee'`).Scan(&amount, &currency); err != nil || amount <= 0 || currency != "INR" {
		return nil, errors.New("valid INR onboarding fee policy required")
	}
	if err = tx.QueryRow(ctx, `INSERT INTO finance.onboarding_attempts(merchant_id,user_id,amount,currency) VALUES($1,$2,$3,$4) RETURNING id`, merchant, user, amount, currency).Scan(&id); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	var link PaymentLink
	if err = s.Gateway.request(ctx, "POST", "/payment_links", "", map[string]any{"amount": amount, "currency": currency, "accept_partial": false, "reference_id": id, "expire_by": time.Now().Add(30 * time.Minute).Unix(), "description": "Yember restaurant onboarding", "notes": map[string]string{"yember_onboarding": merchant}}, &link); err != nil {
		return nil, err
	}
	if link.ID == "" || link.URL == "" {
		return nil, errors.New("invalid onboarding payment response")
	}
	_, err = s.DB.Exec(ctx, `UPDATE finance.onboarding_attempts SET status=CASE WHEN status='CREATING' THEN 'PENDING' ELSE status END,provider_id=COALESCE(provider_id,$2),checkout_url=$3,expires_at=to_timestamp($4) WHERE id=$1`, id, link.ID, link.URL, link.Expires)
	return &OnboardingCheckout{URL: link.URL, Status: "PENDING"}, err
}
func (s *Service) captureOnboarding(ctx context.Context, link PaymentLink, payment string, amount int64, currency string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var merchant, user, status, existingProvider, existingPayment string
	var expected int64
	var expectedCurrency string
	if err = tx.QueryRow(ctx, `SELECT merchant_id,user_id,amount,currency,status,COALESCE(provider_id,''),COALESCE(payment_reference,'') FROM finance.onboarding_attempts WHERE id::text=$1 FOR UPDATE`, link.Reference).Scan(&merchant, &user, &expected, &expectedCurrency, &status, &existingProvider, &existingPayment); err != nil {
		return err
	}
	if amount != expected || currency != expectedCurrency || link.Currency != expectedCurrency {
		return errors.New("onboarding payment amount/currency mismatch")
	}
	if payment == "" || link.ID == "" || (existingProvider != "" && existingProvider != link.ID) || (existingPayment != "" && existingPayment != payment) {
		return errors.New("onboarding payment reference mismatch")
	}
	if status == "PAID" {
		return nil
	}
	result, updateErr := tx.Exec(ctx, `UPDATE finance.onboarding_attempts SET status='PAID',provider_id=COALESCE(provider_id,$2),payment_reference=$3 WHERE id::text=$1 AND (provider_id IS NULL OR provider_id=$2) AND (payment_reference IS NULL OR payment_reference=$3)`, link.Reference, link.ID, payment)
	if updateErr != nil {
		return updateErr
	}
	if result.RowsAffected() != 1 {
		return errors.New("onboarding payment transition rejected")
	}
	if err = post(ctx, tx, "onboarding/"+link.Reference, "ONBOARDING", "", payment, "Provider-confirmed onboarding fee", "", Line{"CASH", "", "onboarding_collection", amount}, Line{"ONBOARDING_REVENUE", "", "onboarding_fee", -amount}); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE public.merchants SET fee_payment_status='PAID',onboarding_status='COMPLETED',is_active=(approval_status='APPROVED' AND COALESCE(public.station_covers(station_id,latitude,longitude),false)),updated_at=now() WHERE id=$1`, merchant); err != nil {
		return err
	}
	if err = notify(ctx, tx, user, "Yember onboarding payment confirmed", "Your restaurant onboarding fee has been verified. Approval and active station coverage are also required before opening."); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

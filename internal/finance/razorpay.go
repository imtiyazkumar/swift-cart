package finance

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Razorpay struct {
	Client  *http.Client
	BaseURL string
}

func (g *Razorpay) request(ctx context.Context, method, path, key string, body any, out any) error {
	id, secret := os.Getenv("RAZORPAY_KEY_ID"), os.Getenv("RAZORPAY_KEY_SECRET")
	if path == "/contacts" || path == "/fund_accounts" || path == "/payouts" || strings.HasPrefix(path, "/payouts/") {
		if payoutID := os.Getenv("RAZORPAYX_KEY_ID"); payoutID != "" {
			id, secret = payoutID, os.Getenv("RAZORPAYX_KEY_SECRET")
		}
	}
	if id == "" || secret == "" {
		return errors.New("Razorpay is not configured")
	}
	if strings.HasPrefix(id, "rzp_live_") && os.Getenv("FINANCE_LIVE_APPROVED") != "true" {
		return errors.New("live finance requires provider/legal activation approval")
	}
	base := g.BaseURL
	if base == "" {
		base = "https://api.razorpay.com/v1"
	}
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.SetBasicAuth(id, secret)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-Payout-Idempotency", key)
	}
	client := g.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return errors.New("Razorpay response uncertain; reconcile the existing operation before retry")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Razorpay returned HTTP %d; reconcile the existing operation", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}
func VerifyRazorpay(raw []byte, signature, secret string) bool {
	if secret == "" {
		return false
	}
	received, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(raw)
	return hmac.Equal(received, mac.Sum(nil))
}

type PaymentLink struct {
	ID        string `json:"id"`
	URL       string `json:"short_url"`
	Reference string `json:"reference_id"`
	Status    string `json:"status"`
	Amount    int64  `json:"amount"`
	Paid      int64  `json:"amount_paid"`
	Currency  string `json:"currency"`
	Expires   int64  `json:"expire_by"`
	OrderID   string `json:"order_id"`
}

func (s *Service) Checkout(ctx context.Context, user, order string) (*CheckoutSession, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var amount int64
	var currency, status string
	var enabled bool
	err = tx.QueryRow(ctx, `SELECT f.total,f.currency,o.status,p.enabled FROM finance.order_financials f JOIN public.orders o ON o.id=f.order_id CROSS JOIN finance.policy p WHERE f.order_id=$1 AND o.customer_id=$2 FOR UPDATE OF o`, order, user).Scan(&amount, &currency, &status, &enabled)
	if err != nil {
		return nil, errors.New("order not found")
	}
	if !enabled {
		return nil, errors.New("central finance is not activated")
	}
	if status != "PAYMENT_PENDING" && status != "PICKED_UP" && status != "OUT_FOR_DELIVERY" {
		return nil, errors.New("order is not awaiting payment")
	}
	var id, providerID, paymentStatus, provider string
	var attempt int
	var started *time.Time
	err = tx.QueryRow(ctx, `SELECT id,COALESCE(provider_id,''),status,attempt,request_started_at,provider FROM finance.payment_attempts WHERE order_id=$1 ORDER BY attempt DESC LIMIT 1`, order).Scan(&id, &providerID, &paymentStatus, &attempt, &started, &provider)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}
	var paid bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM finance.payment_attempts WHERE order_id=$1 AND status='PAID')`, order).Scan(&paid); err != nil {
		return nil, err
	}
	if paid {
		return nil, errors.New("order payment is already collected")
	}
	if providerID != "" && paymentStatus != "EXPIRED" {
		var link PaymentLink
		if err = s.Gateway.request(ctx, "GET", "/payment_links/"+url.PathEscape(providerID), "", nil, &link); err != nil {
			return nil, err
		}
		if link.Status == "paid" {
			return nil, errors.New("payment is awaiting backend reconciliation")
		}
		if link.Status != "expired" && link.Status != "cancelled" {
			return &CheckoutSession{ID: link.ID, URL: link.URL, Amount: link.Amount, Currency: link.Currency, Status: link.Status}, nil
		}
		if _, err = tx.Exec(ctx, `UPDATE finance.payment_attempts SET status='EXPIRED' WHERE id=$1`, id); err != nil {
			return nil, err
		}
		paymentStatus = "EXPIRED"
	}
	if id == "" || paymentStatus == "EXPIRED" || provider == "COD" {
		attempt++
		started = nil
		if err = tx.QueryRow(ctx, `INSERT INTO finance.payment_attempts(order_id,attempt,provider,amount,currency) VALUES($1,$2,'RAZORPAY',$3,$4) RETURNING id`, order, attempt, amount, currency).Scan(&id); err != nil {
			return nil, err
		}
	}
	if started != nil {
		return nil, errors.New("checkout creation pending reconciliation; use the existing order, do not place a duplicate")
	}
	if _, err = tx.Exec(ctx, `UPDATE finance.payment_attempts SET request_started_at=now() WHERE id=$1`, id); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	// Payment Links do not provide payout-style idempotency. Claim once durably and recover by reference.
	var link PaymentLink
	body := map[string]any{"amount": amount, "currency": currency, "accept_partial": false, "reference_id": id, "description": "Yember order " + order, "expire_by": time.Now().Add(30 * time.Minute).Unix(), "notes": map[string]string{"yember_order": order}}
	if err = s.Gateway.request(ctx, "POST", "/payment_links", "", body, &link); err != nil {
		return nil, err
	}
	if link.ID == "" || link.URL == "" {
		return nil, errors.New("invalid Razorpay checkout response")
	}
	_, err = s.DB.Exec(ctx, `UPDATE finance.payment_attempts SET provider_id=$2,checkout_url=$3,expires_at=to_timestamp($4),status=CASE WHEN status='CREATING' THEN 'PENDING' ELSE status END WHERE id=$1`, id, link.ID, link.URL, link.Expires)
	return &CheckoutSession{ID: link.ID, URL: link.URL, Amount: amount, Currency: currency, Status: link.Status}, err
}

func (s *Service) ApplyRazorpay(ctx context.Context, eventID string, raw []byte) error {
	var event struct {
		Event   string `json:"event"`
		Payload struct {
			Link struct {
				Entity PaymentLink `json:"entity"`
			} `json:"payment_link"`
			Payment struct {
				Entity struct {
					ID       string `json:"id"`
					Amount   int64  `json:"amount"`
					Currency string `json:"currency"`
					Status   string `json:"status"`
					OrderID  string `json:"order_id"`
					Fee      int64  `json:"fee"`
					Tax      int64  `json:"tax"`
				} `json:"entity"`
			} `json:"payment"`
			Refund struct {
				Entity struct {
					ID        string `json:"id"`
					PaymentID string `json:"payment_id"`
					Amount    int64  `json:"amount"`
					Status    string `json:"status"`
				} `json:"entity"`
			} `json:"refund"`
		} `json:"payload"`
	}
	if eventID == "" || len(eventID) > 200 || json.Unmarshal(raw, &event) != nil {
		return errors.New("invalid Razorpay event")
	}
	switch event.Event {
	case "payment_link.paid":
		link, pay := event.Payload.Link.Entity, event.Payload.Payment.Entity
		if link.Status != "paid" || pay.Status != "captured" || pay.ID == "" || link.Amount != link.Paid || pay.Amount != link.Amount || pay.OrderID != link.OrderID {
			return errors.New("captured complete payment required")
		}
		var order string
		var onboarding bool
		if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM finance.onboarding_attempts WHERE id::text=$1)`, link.Reference).Scan(&onboarding); err != nil {
			return err
		}
		if onboarding {
			return s.captureOnboarding(ctx, link, pay.ID, pay.Amount, pay.Currency)
		}
		// Recover an uncertain creation response using the signed provider reference and verify amount/currency.
		err := s.DB.QueryRow(ctx, `UPDATE finance.payment_attempts SET provider_id=COALESCE(provider_id,$2) WHERE id=$1 AND provider='RAZORPAY' AND amount=$3 AND currency=$4 AND (provider_id IS NULL OR provider_id=$2) RETURNING order_id`, link.Reference, link.ID, link.Amount, link.Currency).Scan(&order)
		if err != nil {
			return errors.New("payment attempt reference mismatch")
		}
		session := CheckoutSession{ID: link.ID, Amount: pay.Amount, Currency: pay.Currency, PaymentStatus: "paid", Reference: order, PaymentIntent: pay.ID}
		canonical, _ := json.Marshal(map[string]any{"id": eventID, "type": "checkout.session.completed", "data": map[string]any{"object": session}})
		if err = s.applyConfirmedEvent(ctx, canonical); err != nil {
			return err
		}
		if pay.Fee > 0 {
			tx, err := s.DB.Begin(ctx)
			if err != nil {
				return err
			}
			defer tx.Rollback(ctx)
			if err = post(ctx, tx, "gateway-fee/"+pay.ID, "GATEWAY_FEE", order, pay.ID, "Provider-reported payment fee", "", Line{"GATEWAY_FEE", "", "gateway_fee", pay.Fee}, Line{"CASH", "", "gateway_fee", -pay.Fee}); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
	case "refund.processed":
		refund := event.Payload.Refund.Entity
		canonical, _ := json.Marshal(map[string]any{"id": eventID, "type": "refund.updated", "data": map[string]any{"object": map[string]any{"id": refund.ID, "payment_intent": refund.PaymentID, "amount": refund.Amount, "currency": "INR", "status": "succeeded"}}})
		return s.applyConfirmedEvent(ctx, canonical)
	case "payment.dispute.created", "payment.dispute.lost":
		_, err := s.DB.Exec(ctx, `INSERT INTO finance.discrepancies(event_key,kind,details) VALUES($1,'CHARGEBACK',jsonb_build_object('provider_event',$1::text)) ON CONFLICT DO NOTHING`, eventID)
		return err
	}
	return nil
}

func (s *Service) Refund(ctx context.Context, actor, order, key, reason string, amount int64) error {
	if len(key) < 8 || len(key) > 120 || len(reason) < 5 || amount <= 0 {
		return errors.New("refund amount, stable request key and reason required")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM public.orders WHERE id=$1 FOR UPDATE`, order); err != nil {
		return err
	}
	var payment string
	var total, requested int64
	if err = tx.QueryRow(ctx, `SELECT payment_reference,amount FROM finance.payment_attempts WHERE order_id=$1 AND status='PAID' AND provider='RAZORPAY'`, order).Scan(&payment, &total); err != nil {
		return errors.New("captured Razorpay payment required")
	}
	var id string
	var priorStatus string
	err = tx.QueryRow(ctx, `SELECT id,status FROM finance.refund_requests WHERE request_key=$1 AND order_id=$2 AND amount=$3`, key, order, amount).Scan(&id, &priorStatus)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if id != "" {
		if priorStatus == "SUCCEEDED" || priorStatus == "PENDING" {
			return nil
		}
		return errors.New("refund response uncertain; reconcile existing refund before retry")
	}
	if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(amount),0) FROM finance.refund_requests WHERE order_id=$1 AND status<>'FAILED'`, order).Scan(&requested); err != nil {
		return err
	}
	if amount > total-requested {
		return errors.New("refund exceeds captured available amount")
	}
	if err = tx.QueryRow(ctx, `INSERT INTO finance.refund_requests(order_id,request_key,amount,reason,requested_by) VALUES($1,$2,$3,$4,$5) RETURNING id`, order, key, amount, reason, actor).Scan(&id); err != nil {
		return err
	}
	if err = audit(ctx, tx, actor, "REFUND_REQUESTED", order, reason); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	var refund struct {
		ID string `json:"id"`
	}
	// No undocumented refund idempotency guarantee: unknown responses require provider reconciliation.
	if err = s.Gateway.request(ctx, "POST", "/payments/"+url.PathEscape(payment)+"/refund", "", map[string]any{"amount": amount, "receipt": id, "notes": map[string]string{"request_key": key}}, &refund); err != nil {
		return err
	}
	if refund.ID == "" {
		return errors.New("refund provider reference missing")
	}
	_, err = s.DB.Exec(ctx, `UPDATE finance.refund_requests SET provider_id=$2,status=CASE WHEN status='SUCCEEDED' THEN status ELSE 'PENDING' END WHERE id=$1`, id, refund.ID)
	return err
}

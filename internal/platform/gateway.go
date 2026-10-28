package platform

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Gateway struct {
	BaseURL, KeyID, Secret, WebhookSecret string
	Client                                *http.Client
}

func GatewayFromEnv() *Gateway {
	return &Gateway{BaseURL: "https://api.razorpay.com/v1", KeyID: os.Getenv("RAZORPAY_KEY_ID"), Secret: os.Getenv("RAZORPAY_KEY_SECRET"), WebhookSecret: os.Getenv("RAZORPAY_WEBHOOK_SECRET"), Client: &http.Client{Timeout: 15 * time.Second}}
}
func (g *Gateway) request(ctx context.Context, method, path string, body any, result any) error {
	if g == nil || g.KeyID == "" || g.Secret == "" {
		return fmt.Errorf("Razorpay is not configured")
	}
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(g.BaseURL, "/")+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.SetBasicAuth(g.KeyID, g.Secret)
	request.Header.Set("Content-Type", "application/json")
	client := g.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("provider response is uncertain")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("provider returned HTTP %d; reconciliation required", response.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result); err != nil {
		return fmt.Errorf("provider returned an invalid response")
	}
	return nil
}
func signature(secret, value, signature string) bool {
	if secret == "" {
		return false
	}
	raw, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	return hmac.Equal(raw, mac.Sum(nil))
}

type providerPayment struct {
	ID       string `json:"id"`
	OrderID  string `json:"order_id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Status   string `json:"status"`
	Method   string `json:"method"`
}

func (g *Gateway) payment(ctx context.Context, id string) (providerPayment, error) {
	var p providerPayment
	if !strings.HasPrefix(id, "pay_") || len(id) > 120 {
		return p, fmt.Errorf("invalid provider payment reference")
	}
	err := g.request(ctx, "GET", "/payments/"+url.PathEscape(id), nil, &p)
	return p, err
}

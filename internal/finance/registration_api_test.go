package finance

import (
	"context"
	"encoding/json"
	"github.com/imtiyazkumar/yember-core/config"
	"github.com/imtiyazkumar/yember-core/internal/auth"
	"github.com/imtiyazkumar/yember-core/internal/devseed"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"net/http/httptest"
	"testing"
)

func TestPartnerRegistrationStatusContract(t *testing.T) {
	p := testdb.New(t)
	if err := devseed.Seed(context.Background(), p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{JWTSecret: "a-secret-with-more-than-32-characters"}
	e := echo.New()
	Register(e.Group("/api/v1"), p, cfg, zap.NewNop())
	for _, tc := range []struct {
		user, role, status string
		amount             int64
		httpStatus         int
	}{{devseed.OwnerID, "RESTAURANT_OWNER", "PAID", 20000, 200}, {devseed.DeliveryUserID, "DELIVERY_PARTNER", "DUE", 15000, 200}, {devseed.CustomerID, "CUSTOMER", "", 0, 403}} {
		token, err := auth.GenerateAccessToken(cfg.JWTSecret, tc.user, tc.role)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest("GET", "/api/v1/partner/finance/registration", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		e.ServeHTTP(response, request)
		if response.Code != tc.httpStatus {
			t.Fatalf("%s status %d: %s", tc.role, response.Code, response.Body)
		}
		if response.Code != 200 {
			continue
		}
		var result struct {
			Amount     int64  `json:"amount_paise"`
			Status     string `json:"payment_status"`
			Refundable bool   `json:"refundable"`
			Currency   string `json:"currency"`
		}
		if err = json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Amount != tc.amount || result.Status != tc.status || result.Refundable || result.Currency != "INR" {
			t.Fatalf("%s incorrect registration response: %+v", tc.role, result)
		}
	}
}

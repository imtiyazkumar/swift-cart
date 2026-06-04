package custommiddleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestInMemoryRateLimiterAllowsThenLimits(t *testing.T) {
	e := echo.New()
	mw := NewInMemoryRateLimiter(RateLimiterConfig{Limit: 1, Period: time.Minute})
	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	firstReq := httptest.NewRequest(http.MethodGet, "/", nil)
	firstRec := httptest.NewRecorder()
	if err := handler(e.NewContext(firstReq, firstRec)); err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first request 200, got %d", firstRec.Code)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/", nil)
	secondRec := httptest.NewRecorder()
	if err := handler(e.NewContext(secondReq, secondRec)); err != nil {
		t.Fatalf("second request returned unexpected error: %v", err)
	}
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request 429, got %d", secondRec.Code)
	}
}

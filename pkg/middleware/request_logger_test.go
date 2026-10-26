package custommiddleware

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestIDSurvivesTimeoutResponseBuffering(t *testing.T) {
	for _, failure := range []bool{false, true} {
		core, logs := observer.New(zap.InfoLevel)
		e := echo.New()
		e.Use(RequestID())
		e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{Timeout: time.Second}))
		e.Use(RequestLogger(zap.New(core)))
		e.GET("/admin/manage/:resource", func(c echo.Context) error {
			if failure {
				return echo.NewHTTPError(500, "test failure")
			}
			return c.NoContent(200)
		})
		recorder := httptest.NewRecorder()
		e.ServeHTTP(recorder, httptest.NewRequest("GET", "/admin/manage/orders", nil))
		entries := logs.FilterMessage("request").All()
		if len(entries) != 1 {
			t.Fatal("missing request log")
		}
		fields := entries[0].ContextMap()
		id := recorder.Header().Get(echo.HeaderXRequestID)
		if id == "" || fields["request_id"] != id {
			t.Fatalf("request correlation lost: %v", fields)
		}
		if fields["resource"] != "orders" {
			t.Fatal("resource missing from log")
		}
	}
}

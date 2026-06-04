package common

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestErrorHelpersSetHTTPStatus(t *testing.T) {
	tests := []struct {
		name string
		err  interface{ Error() string }
		code int
	}{
		{name: "bad request", err: BadRequest("bad"), code: http.StatusBadRequest},
		{name: "not found", err: NotFound("missing"), code: http.StatusNotFound},
		{name: "internal", err: Internal("boom"), code: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpErr, ok := tt.err.(*echo.HTTPError)
			if !ok {
				t.Fatalf("expected echo HTTP error")
			}
			if httpErr.Code != tt.code {
				t.Fatalf("expected status %d, got %d", tt.code, httpErr.Code)
			}
		})
	}
}

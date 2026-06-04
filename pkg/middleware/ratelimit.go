package custommiddleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// RateLimiterConfig defines the parameters for the in-process rate limiter.
type RateLimiterConfig struct {
	// Limit is the number of allowed requests per period.
	Limit int
	// Period defines the duration of the limit window.
	Period time.Duration
}

type rateWindow struct {
	count     int
	resetTime time.Time
}

// NewInMemoryRateLimiter creates an Echo middleware that enforces per-instance IP rate limits.
func NewInMemoryRateLimiter(cfg RateLimiterConfig) echo.MiddlewareFunc {
	var mu sync.Mutex
	windows := make(map[string]rateWindow)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			clientIP := c.RealIP()
			now := time.Now()

			mu.Lock()
			window := windows[clientIP]
			if window.resetTime.IsZero() || now.After(window.resetTime) {
				window = rateWindow{resetTime: now.Add(cfg.Period)}
			}
			window.count++
			windows[clientIP] = window
			remaining := cfg.Limit - window.count
			resetSeconds := int(time.Until(window.resetTime).Seconds())
			limited := window.count > cfg.Limit

			for ip, win := range windows {
				if now.After(win.resetTime) {
					delete(windows, ip)
				}
			}
			mu.Unlock()

			if limited {
				c.Response().Header().Set("X-RateLimit-Remaining", "0")
				c.Response().Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetSeconds))
				return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			}

			if remaining < 0 {
				remaining = 0
			}
			c.Response().Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			c.Response().Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetSeconds))
			return next(c)
		}
	}
}

package custommiddleware

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/go-redis/redis/v8"
    redisrate "github.com/go-redis/redis_rate/v9"
    "github.com/labstack/echo/v4"
)

// RateLimiterConfig defines the parameters for the Redis rate limiter.
type RateLimiterConfig struct {
    // Limit is the number of allowed requests per period.
    Limit int
    // Period defines the duration of the limit window.
    Period time.Duration
}

// NewRedisRateLimiter creates an Echo middleware that enforces a Redis‑backed rate limit.
func NewRedisRateLimiter(rdb *redis.Client, cfg RateLimiterConfig) echo.MiddlewareFunc {
    limiter := redisrate.NewLimiter(rdb)
    keyPrefix := "ratelimit"

    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            clientIP := c.RealIP()
            key := fmt.Sprintf("%s:%s", keyPrefix, clientIP)
            // Allow one request according to the configured rate.
            res, err := limiter.Allow(context.Background(), key, redisrate.Limit{Rate: cfg.Limit, Period: cfg.Period})
            if err != nil {
                return c.JSON(http.StatusInternalServerError, map[string]string{"error": "rate limit internal error"})
            }
            if res.Allowed == 0 {
                return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
            }
            c.Response().Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", res.Allowed))
            c.Response().Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", int(res.ResetAfter.Seconds())))
            return next(c)
        }
    }
}

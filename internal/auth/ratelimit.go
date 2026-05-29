package auth

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/labstack/echo/v4"
    "github.com/redis/go-redis/v9"
    "go.uber.org/zap"
)

// RateLimitMiddleware limits requests per IP using Redis token bucket.
// limit: maximum number of requests per minute.
func RateLimitMiddleware(rdb *redis.Client, limit int) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            ip := c.RealIP()
            if ip == "" {
                ip = "unknown"
            }
            key := fmt.Sprintf("rl:%s", ip)
            ctx := context.Background()
            // Increment counter
            cnt, err := rdb.Incr(ctx, key).Result()
            if err != nil {
                // Log and allow request to avoid denial of service on Redis failure
                // Assuming a logger is attached to Echo context; fallback to stdout
                if logger, ok := c.Get("logger").(*zap.Logger); ok {
                    logger.Error("rate limit redis error", zap.Error(err))
                }
                return next(c)
            }
            // Set expiry of 1 minute on first hit
            if cnt == 1 {
                rdb.Expire(ctx, key, time.Minute)
            }
            if cnt > int64(limit) {
                return echo.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded")
            }
            return next(c)
        }
    }
}

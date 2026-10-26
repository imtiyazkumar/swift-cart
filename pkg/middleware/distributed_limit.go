package custommiddleware

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"
	"time"
)

// PostgreSQL is the shared authority across replicas. Ingress must also bound traffic.
func DistributedRateLimiter(pool *pgxpool.Pool, scope string, limit int) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			if scope == "api" && (path == "/healthz" || path == "/readyz" || strings.HasPrefix(path, "/api/v1/webhooks/") || strings.HasPrefix(path, "/api/v1/menu-images/")) {
				return next(c)
			}
			now := time.Now().UTC().Truncate(time.Minute)
			var count int
			err := pool.QueryRow(c.Request().Context(), `INSERT INTO auth.rate_windows(scope,client,window_start,count) VALUES($1,$2,$3,1) ON CONFLICT(scope,client,window_start) DO UPDATE SET count=auth.rate_windows.count+1 RETURNING count`, scope, c.RealIP(), now).Scan(&count)
			if err != nil {
				return echo.NewHTTPError(http.StatusServiceUnavailable, "request protection unavailable").SetInternal(err)
			}
			if count > limit {
				c.Response().Header().Set("Retry-After", "60")
				return echo.NewHTTPError(http.StatusTooManyRequests, "request rate exceeded")
			}
			return next(c)
		}
	}
}

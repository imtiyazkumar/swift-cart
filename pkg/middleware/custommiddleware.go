package custommiddleware

import (
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"net/http"
	"time"
)

// RequestLogger logs request details using zap.
func RequestLogger(logger *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			res := c.Response()
			started := time.Now()
			err := next(c)
			status := res.Status
			if err != nil && !res.Committed {
				status = http.StatusInternalServerError
			}
			if httpErr, ok := err.(*echo.HTTPError); ok {
				status = httpErr.Code
			}
			logger.Info("request",
				zap.String("method", req.Method),
				zap.String("route", c.Path()),
				zap.String("request_id", res.Header().Get(echo.HeaderXRequestID)),
				zap.Duration("duration", time.Since(started)),
				zap.Int("status", status),
				zap.String("remote_ip", c.RealIP()),
			)
			return err
		}
	}
}

// ErrorHandler converts Go errors into JSON problem responses.
func ErrorHandler(logger *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if err := next(c); err != nil {
				// Log the error
				logger.Error("handler error", zap.Error(err))
				// Return generic error response (avoid leaking details)
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			}
			return nil
		}
	}
}

package custommiddleware

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
	"net/http"
	"time"
)

const requestIDContextKey = "yember.requestID"

// Retain the ID in the request context as timeout response writers buffer headers.
func RequestID() echo.MiddlewareFunc {
	return middleware.RequestIDWithConfig(middleware.RequestIDConfig{RequestIDHandler: func(c echo.Context, id string) { c.Set(requestIDContextKey, id) }})
}

// RequestLogger logs request details using zap.
func RequestLogger(logger *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			res := c.Response()
			started := time.Now()
			err := next(c)
			requestID, _ := c.Get(requestIDContextKey).(string)
			if requestID == "" {
				requestID = res.Header().Get(echo.HeaderXRequestID)
			}
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
				zap.String("request_id", requestID),
				zap.String("resource", c.Param("resource")),
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

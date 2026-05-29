package custommiddleware

import (
    "net/http"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

// RequestLogger logs request details using zap.
func RequestLogger(logger *zap.Logger) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            req := c.Request()
            res := c.Response()
            logger.Info("request",
                zap.String("method", req.Method),
                zap.String("uri", req.RequestURI),
                zap.Int("status", res.Status),
                zap.String("remote_ip", c.RealIP()),
            )
            return next(c)
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

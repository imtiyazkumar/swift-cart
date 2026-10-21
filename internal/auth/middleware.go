package auth

import (
    "net/http"
    "strings"

    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

// JWTMiddleware validates the Authorization header and sets user info in the context.
func JWTMiddleware(secret string, logger *zap.Logger, pools ...*pgxpool.Pool) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            authHeader := c.Request().Header.Get("Authorization")
            if authHeader == "" {
                return echo.NewHTTPError(http.StatusUnauthorized, "missing Authorization header")
            }
            parts := strings.SplitN(authHeader, " ", 2)
            if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
                return echo.NewHTTPError(http.StatusUnauthorized, "invalid Authorization header format")
            }
            tokenStr := parts[1]
            claims, err := ParseToken(secret, tokenStr)
            if err != nil || claims.TokenType != "access" {
                logger.Error("jwt parse error", zap.Error(err))
                return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
            }
            // Store user ID and role in the echo context for downstream handlers.
            c.Set("userID", claims.UserID)
            c.Set("role", claims.Role)
            if len(pools) > 0 {
                var currentRole string
                err := pools[0].QueryRow(c.Request().Context(), `SELECT role FROM public.users WHERE id=$1 AND deleted_at IS NULL AND disabled_at IS NULL`, claims.UserID).Scan(&currentRole)
                if err != nil {
                    return echo.NewHTTPError(http.StatusUnauthorized, "account is unavailable")
                }
                c.Set("role", currentRole)
            }
            return next(c)
        }
    }
}

// RBACMiddleware ensures the user has one of the allowed roles.
func RBACMiddleware(allowedRoles ...string) echo.MiddlewareFunc {
    roleSet := make(map[string]struct{}, len(allowedRoles))
    for _, r := range allowedRoles {
        roleSet[r] = struct{}{}
    }
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            role, ok := c.Get("role").(string)
            if !ok || role == "" {
                return echo.NewHTTPError(http.StatusForbidden, "role missing in token")
            }
            if _, ok := roleSet[role]; !ok {
                return echo.NewHTTPError(http.StatusForbidden, "insufficient role")
            }
            return next(c)
        }
    }
}

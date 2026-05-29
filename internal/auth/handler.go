package auth

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/yourorg/swiftkart/config"
	"go.uber.org/zap"
)

// RegisterRoutes registers auth endpoints.
func RegisterRoutes(e *echo.Echo, cfg *config.Config, log *zap.Logger, dbPool *pgxpool.Pool) {
	g := e.Group("/api/v1/auth")
	g.POST("/signup", SignUp(cfg, log, dbPool))
	g.POST("/login", Login(cfg, log, dbPool))
	g.POST("/refresh", Refresh(cfg, log))
	g.GET("/me", Me(log, dbPool))
}

// SignUp creates a new user.
func SignUp(cfg *config.Config, logger *zap.Logger, dbPool *pgxpool.Pool) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		if req.Email == "" || req.Password == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "email and password required")
		}
		user, err := NewUser(req.Email, req.Password, req.Role)
		if err != nil {
			logger.Error("user creation failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "could not create user")
		}
		if err := user.Insert(c.Request().Context(), dbPool); err != nil {
			logger.Error("db insert failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "could not save user")
		}
		return c.JSON(http.StatusCreated, map[string]string{"id": user.ID, "email": user.Email, "role": user.Role})
	}
}

// Login authenticates and returns JWT tokens.
func Login(cfg *config.Config, logger *zap.Logger, dbPool *pgxpool.Pool) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		user, err := FindByEmail(c.Request().Context(), dbPool, req.Email)
		if err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
		}
		if err := user.VerifyPassword(req.Password); err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
		}
		access, err := GenerateAccessToken(cfg.JWTSecret, user.ID, user.Role)
		if err != nil {
			logger.Error("access token error", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "could not generate token")
		}
		refresh, err := GenerateRefreshToken(cfg.JWTSecret, user.ID, user.Role)
		if err != nil {
			logger.Error("refresh token error", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "could not generate token")
		}
		return c.JSON(http.StatusOK, map[string]string{"access_token": access, "refresh_token": refresh})
	}
}

// Refresh issues a new access token using a refresh token.
func Refresh(cfg *config.Config, logger *zap.Logger) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		claims, err := ParseToken(cfg.JWTSecret, req.RefreshToken)
		if err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid refresh token")
		}
		newAccess, err := GenerateAccessToken(cfg.JWTSecret, claims.UserID, claims.Role)
		if err != nil {
			logger.Error("new access token error", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "could not generate token")
		}
		return c.JSON(http.StatusOK, map[string]string{"access_token": newAccess})
	}
}

// Me returns the authenticated user's profile.
func Me(logger *zap.Logger, dbPool *pgxpool.Pool) echo.HandlerFunc {
	return func(c echo.Context) error {
		userID, ok := c.Get("userID").(string)
		if !ok || userID == "" {
			return echo.NewHTTPError(http.StatusUnauthorized, "user not in context")
		}
		u := &User{}
		query := `SELECT id, email, role, created_at FROM users WHERE id=$1`
		err := dbPool.QueryRow(c.Request().Context(), query, userID).Scan(&u.ID, &u.Email, &u.Role, &u.CreatedAt)
		if err != nil {
			logger.Error("fetch user failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "could not load user")
		}
		return c.JSON(http.StatusOK, u)
	}
}

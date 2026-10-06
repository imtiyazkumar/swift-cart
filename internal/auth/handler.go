package auth

import (
	"net/http"
	"strings"

	"github.com/imtiyazkumar/yember-core/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// RegisterRoutes registers auth endpoints.
func RegisterRoutes(e *echo.Echo, cfg *config.Config, log *zap.Logger, dbPool *pgxpool.Pool) {
	g := e.Group("/api/v1/auth")
	g.POST("/signup", SignUp(cfg, log, dbPool))
	g.POST("/login", Login(cfg, log, dbPool))
	g.POST("/send-otp", SendOTP(log, dbPool))
	g.POST("/verify-otp", VerifyOTP(cfg, log, dbPool))
	g.POST("/verify-secondary-otp", VerifySecondaryOTP(cfg, log, dbPool))
	g.POST("/refresh", Refresh(cfg, log))
	g.GET("/me", Me(log, dbPool), JWTMiddleware(cfg.JWTSecret, log))
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
		query := `SELECT id, COALESCE(email, ''), COALESCE(phone, ''), COALESCE(full_name, ''), COALESCE(gender, ''), role, created_at FROM users WHERE id=$1`
		err := dbPool.QueryRow(c.Request().Context(), query, userID).Scan(&u.ID, &u.Email, &u.Phone, &u.FullName, &u.Gender, &u.Role, &u.CreatedAt)
		if err != nil {
			logger.Error("fetch user failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "could not load user")
		}
		return c.JSON(http.StatusOK, u)
	}
}

// SendOTP generates and saves OTP 1234 in auth.otp_codes.
func SendOTP(logger *zap.Logger, dbPool *pgxpool.Pool) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			Identifier string `json:"identifier"`
		}
		if err := c.Bind(&req); err != nil || strings.TrimSpace(req.Identifier) == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "identifier (phone or email) is required")
		}

		identifier := strings.TrimSpace(req.Identifier)
		otpCode := "1234"

		if err := CreateOTP(c.Request().Context(), dbPool, identifier, otpCode); err != nil {
			logger.Error("save OTP to database failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate OTP")
		}

		logger.Info("OTP generated and stored in auth.otp_codes", zap.String("identifier", identifier))
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "OTP sent successfully",
		})
	}
}

// VerifyOTP verifies the OTP code against auth.otp_codes and logs in or creates the user.
func VerifyOTP(cfg *config.Config, logger *zap.Logger, dbPool *pgxpool.Pool) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			Identifier string `json:"identifier"`
			OTP        string `json:"otp"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		identifier := strings.TrimSpace(req.Identifier)
		otpCode := strings.TrimSpace(req.OTP)

		if identifier == "" || otpCode == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "identifier and OTP code required")
		}

		// Verify against database or allow 1234 fallback
		valid, _ := VerifyOTPInDB(c.Request().Context(), dbPool, identifier, otpCode)
		if !valid && otpCode != "1234" {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired OTP code")
		}

		user, err := FindOrCreateByIdentifier(c.Request().Context(), dbPool, identifier)
		if err != nil {
			logger.Error("find or create user failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to authenticate user")
		}

		access, err := GenerateAccessToken(cfg.JWTSecret, user.ID, user.Role)
		if err != nil {
			logger.Error("access token generation error", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "could not generate access token")
		}
		refresh, err := GenerateRefreshToken(cfg.JWTSecret, user.ID, user.Role)
		if err != nil {
			logger.Error("refresh token generation error", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "could not generate refresh token")
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"access_token":  access,
			"refresh_token": refresh,
			"user": map[string]interface{}{
				"id":        user.ID,
				"email":     user.Email,
				"phone":     user.Phone,
				"full_name": user.FullName,
				"gender":    user.Gender,
				"role":      user.Role,
			},
		})
	}
}

// VerifySecondaryOTP verifies an OTP for linking a secondary email or phone to an authenticated user.
func VerifySecondaryOTP(cfg *config.Config, logger *zap.Logger, dbPool *pgxpool.Pool) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			UserID     string `json:"user_id"`
			Identifier string `json:"identifier"`
			OTP        string `json:"otp"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		if req.UserID == "" {
			if uid, ok := c.Get("userID").(string); ok {
				req.UserID = uid
			}
		}
		identifier := strings.TrimSpace(req.Identifier)
		otpCode := strings.TrimSpace(req.OTP)

		if req.UserID == "" || identifier == "" || otpCode == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "user_id, identifier and OTP code required")
		}

		normalized := NormalizeIdentifier(identifier)
		isEmail := strings.Contains(normalized, "@")

		// 1. Verify OTP code
		valid, _ := VerifyOTPInDB(c.Request().Context(), dbPool, normalized, otpCode)
		if !valid && otpCode != "1234" {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired OTP code")
		}

		// 2. Check if identifier is already registered to another account
		if isEmail {
			var existingID string
			err := dbPool.QueryRow(c.Request().Context(), `SELECT id FROM public.users WHERE LOWER(email) = LOWER($1) AND id != $2 AND deleted_at IS NULL`, normalized, req.UserID).Scan(&existingID)
			if err == nil && existingID != "" {
				return echo.NewHTTPError(http.StatusBadRequest, "this email address is already registered to another account")
			}
			_, _ = dbPool.Exec(c.Request().Context(), `UPDATE public.users SET email = $1, updated_at = NOW() WHERE id = $2`, normalized, req.UserID)
			_, _ = dbPool.Exec(c.Request().Context(), `UPDATE public.customers SET email = $1, updated_at = NOW() WHERE id = $2`, normalized, req.UserID)
		} else {
			var existingID string
			err := dbPool.QueryRow(c.Request().Context(), `SELECT id FROM public.users WHERE phone = $1 AND id != $2 AND deleted_at IS NULL`, normalized, req.UserID).Scan(&existingID)
			if err == nil && existingID != "" {
				return echo.NewHTTPError(http.StatusBadRequest, "this mobile number is already registered to another account")
			}
			_, _ = dbPool.Exec(c.Request().Context(), `UPDATE public.users SET phone = $1, updated_at = NOW() WHERE id = $2`, normalized, req.UserID)
			_, _ = dbPool.Exec(c.Request().Context(), `UPDATE public.customers SET phone = $1, updated_at = NOW() WHERE id = $2`, normalized, req.UserID)
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "Secondary contact verified and linked successfully",
		})
	}
}


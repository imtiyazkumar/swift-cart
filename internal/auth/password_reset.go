package auth

import (
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    "net/url"
    "os"
    "strings"
    "time"

    "github.com/imtiyazkumar/yember-core/config"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
    "golang.org/x/crypto/bcrypt"
)

const passwordResetResponse = "If an account exists for that email, a password reset link will be sent."

// ForgotPassword sends a short-lived, single-use reset link without revealing whether an email is registered.
func ForgotPassword(cfg *config.Config, logger *zap.Logger, dbPool *pgxpool.Pool) echo.HandlerFunc {
    return func(c echo.Context) error {
        var req struct {
            Email string `json:"email"`
        }
        if err := c.Bind(&req); err != nil || strings.TrimSpace(req.Email) == "" {
            return echo.NewHTTPError(400, "email is required")
        }
        if cfg.ResendAPIKey == "" || cfg.EmailFrom == "" {
            return echo.NewHTTPError(503, "password reset email is not configured")
        }

        email := strings.ToLower(strings.TrimSpace(req.Email))
        var userID, role string
        err := dbPool.QueryRow(c.Request().Context(),
            `SELECT id,role FROM public.users WHERE LOWER(email)=LOWER($1) AND deleted_at IS NULL`, email,
        ).Scan(&userID, &role)
        if err == pgx.ErrNoRows {
            return c.JSON(200, map[string]string{"message": passwordResetResponse})
        }
        if err != nil {
            logger.Error("password reset lookup failed", zap.Error(err))
            return echo.NewHTTPError(500, "could not process password reset")
        }

        rawToken := make([]byte, 32)
        if _, err := rand.Read(rawToken); err != nil {
            logger.Error("password reset token generation failed", zap.Error(err))
            return echo.NewHTTPError(500, "could not process password reset")
        }
        token := hex.EncodeToString(rawToken)
        tokenHash := sha256.Sum256([]byte(token))
        tx, err := dbPool.Begin(c.Request().Context())
        if err != nil {
            return echo.NewHTTPError(500, "could not process password reset")
        }
        defer tx.Rollback(c.Request().Context())
        _, err = tx.Exec(c.Request().Context(),
            `DELETE FROM public.password_reset_tokens WHERE user_id=$1 AND consumed_at IS NULL`, userID,
        )
        if err != nil {
            logger.Error("old password reset token cleanup failed", zap.Error(err))
            return echo.NewHTTPError(500, "could not process password reset")
        }
        _, err = tx.Exec(c.Request().Context(),
            `INSERT INTO public.password_reset_tokens (user_id, token_hash, expires_at) VALUES ($1,$2,$3)`,
            userID, hex.EncodeToString(tokenHash[:]), time.Now().UTC().Add(30*time.Minute),
        )
        if err != nil {
            logger.Error("password reset token save failed", zap.Error(err))
            return echo.NewHTTPError(500, "could not process password reset")
        }

        targetURL := cfg.PasswordResetURL
        if role == "MERCHANT_OWNER" && os.Getenv("RESTAURANT_PASSWORD_RESET_URL") != "" {
            targetURL = os.Getenv("RESTAURANT_PASSWORD_RESET_URL")
        }
        if role == "DELIVERY_PARTNER" && os.Getenv("DELIVERY_PASSWORD_RESET_URL") != "" {
            targetURL = os.Getenv("DELIVERY_PASSWORD_RESET_URL")
        }
        resetURL, err := url.Parse(targetURL)
        if err != nil {
            logger.Error("password reset URL is invalid", zap.Error(err))
            return echo.NewHTTPError(500, "could not process password reset")
        }
        query := resetURL.Query()
        query.Set("token", token)
        resetURL.RawQuery = query.Encode()
        if _, err = tx.Exec(c.Request().Context(), `INSERT INTO notifications.email_outbox(recipient,subject,body) VALUES($1,'Reset your Yember password',$2)`, email, "Use this single-use link within 30 minutes to reset your password: "+resetURL.String()); err != nil {
            return echo.NewHTTPError(500, "could not queue password reset")
        }
        if err = tx.Commit(c.Request().Context()); err != nil {
            return echo.NewHTTPError(500, "could not queue password reset")
        }
        return c.JSON(200, map[string]string{"message": passwordResetResponse})
    }
}

// ResetPassword consumes a reset token and replaces the user's bcrypt password hash.
func ResetPassword(logger *zap.Logger, dbPool *pgxpool.Pool) echo.HandlerFunc {
    return func(c echo.Context) error {
        var req struct {
            Token    string `json:"token"`
            Password string `json:"password"`
        }
        if err := c.Bind(&req); err != nil || strings.TrimSpace(req.Token) == "" {
            return echo.NewHTTPError(400, "reset token is required")
        }
        if len(req.Password) < 10 || len(req.Password)>72 {
            return echo.NewHTTPError(400, "password must contain 10 to 72 bytes")
        }
        hash := sha256.Sum256([]byte(req.Token))
        tx, err := dbPool.Begin(c.Request().Context())
        if err != nil {
            return echo.NewHTTPError(500, "could not reset password")
        }
        defer tx.Rollback(c.Request().Context())

        var userID string
        err = tx.QueryRow(c.Request().Context(), `
            SELECT user_id FROM public.password_reset_tokens
            WHERE token_hash=$1 AND expires_at > now() AND consumed_at IS NULL
            FOR UPDATE`, hex.EncodeToString(hash[:])).Scan(&userID)
        if err == pgx.ErrNoRows {
            return echo.NewHTTPError(400, "reset link is invalid or expired")
        }
        if err != nil {
            logger.Error("password reset token lookup failed", zap.Error(err))
            return echo.NewHTTPError(500, "could not reset password")
        }
        passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
        if err != nil {
            return echo.NewHTTPError(500, "could not reset password")
        }
        if _, err = tx.Exec(c.Request().Context(), `UPDATE public.users SET password_hash=$1, tokens_invalid_before=date_trunc('milliseconds',clock_timestamp()), updated_at=now() WHERE id=$2`, string(passwordHash), userID); err != nil {
            logger.Error("password update failed", zap.Error(err))
            return echo.NewHTTPError(500, "could not reset password")
        }
        if _, err = tx.Exec(c.Request().Context(), `UPDATE public.password_reset_tokens SET consumed_at=now() WHERE token_hash=$1`, hex.EncodeToString(hash[:])); err != nil {
            return echo.NewHTTPError(500, "could not reset password")
        }
        if _, err = tx.Exec(c.Request().Context(), `DELETE FROM public.password_reset_tokens WHERE user_id=$1 AND token_hash<>$2`, userID, hex.EncodeToString(hash[:])); err != nil {
            return echo.NewHTTPError(500, "could not reset password")
        }
        if _, err = tx.Exec(c.Request().Context(), `UPDATE auth.refresh_sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, userID); err != nil {
            return echo.NewHTTPError(500, "could not revoke old sessions")
        }
        if err = tx.Commit(c.Request().Context()); err != nil {
            logger.Error("password reset transaction commit failed", zap.Error(err))
            return echo.NewHTTPError(500, "could not reset password")
        }
        return c.JSON(200, map[string]string{"message": "Password reset successfully. You can now sign in."})
    }
}

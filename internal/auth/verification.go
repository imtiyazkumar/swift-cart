package auth

import (
    "crypto/rand"
    "encoding/hex"
    "net/mail"
    "net/url"
    "os"
    "strings"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/labstack/echo/v4"
    "golang.org/x/crypto/bcrypt"
)

func PartnerSignup(pool *pgxpool.Pool) echo.HandlerFunc {
    return func(c echo.Context) error {
        var req struct {
            Email    string `json:"email"`
            Password string `json:"password"`
            Name     string `json:"name"`
            Kind     string `json:"kind"`
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(400, "invalid request")
        }
        req.Email = strings.ToLower(strings.TrimSpace(req.Email))
        parsed, err := mail.ParseAddress(req.Email)
        if err != nil || parsed.Address != req.Email || len(req.Password) < 10 || len(req.Password) > 72 || strings.TrimSpace(req.Name) == "" {
            return echo.NewHTTPError(400, "name, valid email and password of 10 to 72 bytes are required")
        }
        role := ""
        switch req.Kind {
        case "restaurant":
            role = "MERCHANT_OWNER"
        case "delivery":
            role = "DELIVERY_PARTNER"
        default:
            return echo.NewHTTPError(400, "invalid partner kind")
        }
        hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
        if err != nil {
            return echo.NewHTTPError(500, "could not create account")
        }
        var id string
        err = pool.QueryRow(c.Request().Context(), `INSERT INTO public.users(email,password_hash,full_name,role) VALUES($1,$2,$3,$4) RETURNING id`, req.Email, string(hash), req.Name, role).Scan(&id)
        if err != nil {
            return echo.NewHTTPError(409, "could not create account; the email may already be registered")
        }
        return c.JSON(201, map[string]string{"id": id, "message": "Account created. Sign in and request email verification."})
    }
}

func RequestVerification(pool *pgxpool.Pool) echo.HandlerFunc {
    return func(c echo.Context) error {
        id, _ := c.Get("userID").(string)
        raw := make([]byte, 32)
        if _, err := rand.Read(raw); err != nil {
            return echo.NewHTTPError(500, "could not generate verification")
        }
        token := hex.EncodeToString(raw)
        verificationURL := os.Getenv("EMAIL_VERIFICATION_URL")
        if verificationURL == "" {
            return echo.NewHTTPError(503, "EMAIL_VERIFICATION_URL is not configured")
        }
        link, err := url.Parse(verificationURL)
        if err != nil {
            return echo.NewHTTPError(503, "email verification URL is invalid")
        }
        query := link.Query()
        query.Set("token", token)
        link.RawQuery = query.Encode()
        tx, err := pool.Begin(c.Request().Context())
        if err != nil {
            return echo.NewHTTPError(500, "could not queue verification")
        }
        defer tx.Rollback(c.Request().Context())
        var email string
        if err = tx.QueryRow(c.Request().Context(), `SELECT COALESCE(email,'') FROM public.users WHERE id=$1 AND disabled_at IS NULL AND deleted_at IS NULL FOR UPDATE`, id).Scan(&email); err != nil || email == "" {
            return echo.NewHTTPError(400, "email required")
        }
        if _, err = tx.Exec(c.Request().Context(), `DELETE FROM auth.email_verification_tokens WHERE user_id=$1 AND consumed_at IS NULL`, id); err != nil {
            return echo.NewHTTPError(500, "could not queue verification")
        }
        if _, err = tx.Exec(c.Request().Context(), `INSERT INTO auth.email_verification_tokens(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, tokenHash(token), id); err != nil {
            return echo.NewHTTPError(500, "could not queue verification")
        }
        if _, err = tx.Exec(c.Request().Context(), `INSERT INTO notifications.email_outbox(recipient,subject,body) VALUES($1,'Verify your Yember email',$2)`, email, "Verify your email within one hour: "+link.String()); err != nil {
            return echo.NewHTTPError(500, "could not queue verification")
        }
        if err = tx.Commit(c.Request().Context()); err != nil {
            return echo.NewHTTPError(500, "could not queue verification")
        }
        return c.JSON(202, map[string]string{"message": "Verification email queued. Delivery requires configured email service."})
    }
}

func VerifyEmail(pool *pgxpool.Pool) echo.HandlerFunc {
    return func(c echo.Context) error {
        var req struct {
            Token string `json:"token"`
        }
        if err := c.Bind(&req); err != nil || req.Token == "" {
            return echo.NewHTTPError(400, "token required")
        }
        tx, err := pool.Begin(c.Request().Context())
        if err != nil {
            return echo.NewHTTPError(500, "verification unavailable")
        }
        defer tx.Rollback(c.Request().Context())
        var userID string
        if err = tx.QueryRow(c.Request().Context(), `UPDATE auth.email_verification_tokens SET consumed_at=now() WHERE token_hash=$1 AND consumed_at IS NULL AND expires_at>now() RETURNING user_id`, tokenHash(req.Token)).Scan(&userID); err != nil {
            return echo.NewHTTPError(400, "verification link is invalid or expired")
        }
        if _, err = tx.Exec(c.Request().Context(), `UPDATE public.users SET email_verified_at=now() WHERE id=$1`, userID); err != nil {
            return echo.NewHTTPError(500, "verification unavailable")
        }
        if err = tx.Commit(c.Request().Context()); err != nil {
            return echo.NewHTTPError(500, "verification unavailable")
        }
        return c.JSON(200, map[string]string{"message": "Email verified"})
    }
}

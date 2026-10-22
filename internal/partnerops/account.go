package partnerops

import (
    "strings"

    "github.com/labstack/echo/v4"
    "golang.org/x/crypto/bcrypt"
)

func registerAccount(g *echo.Group, r *Repository) {
    g.GET("/account", func(c echo.Context) error {
        var data []byte
        err := r.pool.QueryRow(c.Request().Context(), `SELECT jsonb_build_object('id',id,'name',COALESCE(full_name,''),'email',COALESCE(email,''),'phone',COALESCE(phone,''),'role',role,'email_verified',email_verified_at IS NOT NULL,'created_at',created_at) FROM public.users WHERE id=$1 AND disabled_at IS NULL AND deleted_at IS NULL`, c.Get("userID")).Scan(&data)
        if err != nil {
            return echo.NewHTTPError(500, "could not load account")
        }
        return c.Blob(200, "application/json", data)
    })
    g.PATCH("/account", func(c echo.Context) error {
        var input struct {
            Name  string `json:"name"`
            Phone string `json:"phone"`
        }
        if c.Bind(&input) != nil || len(strings.TrimSpace(input.Name)) < 2 || len(input.Name) > 200 || len(input.Phone) < 7 || len(input.Phone) > 30 {
            return echo.NewHTTPError(400, "valid name and contact phone required")
        }
        ctx := c.Request().Context()
        tx, err := r.pool.Begin(ctx)
        if err != nil {
            return echo.NewHTTPError(503, "account service unavailable")
        }
        defer tx.Rollback(ctx)
        if _, err = tx.Exec(ctx, `UPDATE public.users SET full_name=$2,phone=$3,updated_at=now() WHERE id=$1`, c.Get("userID"), strings.TrimSpace(input.Name), strings.TrimSpace(input.Phone)); err != nil {
            return echo.NewHTTPError(400, "contact details could not be saved")
        }
        if c.Get("role") == "DELIVERY_PARTNER" {
            if _, err = tx.Exec(ctx, `UPDATE delivery.delivery_partners SET name=$2,phone=$3,updated_at=now() WHERE user_id=$1 AND deleted_at IS NULL`, c.Get("userID"), strings.TrimSpace(input.Name), strings.TrimSpace(input.Phone)); err != nil {
                return echo.NewHTTPError(500, "could not update delivery profile")
            }
        }
        if err = tx.Commit(ctx); err != nil {
            return echo.NewHTTPError(500, "could not save account")
        }
        return c.NoContent(204)
    })
    g.POST("/password", func(c echo.Context) error {
        var input struct {
            Current string `json:"current_password"`
            Next    string `json:"new_password"`
        }
        if c.Bind(&input) != nil || len(input.Current) > 72 || len(input.Next) < 12 || len(input.Next) > 72 || input.Current == input.Next {
            return echo.NewHTTPError(400, "use a different password with 12-72 bytes")
        }
        ctx := c.Request().Context()
        tx, err := r.pool.Begin(ctx)
        if err != nil {
            return echo.NewHTTPError(503, "account service unavailable")
        }
        defer tx.Rollback(ctx)
        var hash string
        if err = tx.QueryRow(ctx, `SELECT password_hash FROM public.users WHERE id=$1 FOR UPDATE`, c.Get("userID")).Scan(&hash); err != nil {
            return echo.NewHTTPError(500, "could not load account")
        }
        if bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Current)) != nil {
            return echo.NewHTTPError(400, "current password is incorrect")
        }
        next, err := bcrypt.GenerateFromPassword([]byte(input.Next), bcrypt.DefaultCost)
        if err != nil {
            return echo.NewHTTPError(500, "could not update password")
        }
        if _, err = tx.Exec(ctx, `UPDATE public.users SET password_hash=$2,updated_at=now() WHERE id=$1`, c.Get("userID"), string(next)); err != nil {
            return echo.NewHTTPError(500, "could not update password")
        }
        if _, err = tx.Exec(ctx, `UPDATE auth.refresh_sessions SET revoked_at=now() WHERE user_id=$1`, c.Get("userID")); err != nil {
            return echo.NewHTTPError(500, "could not revoke sessions")
        }
        if err = tx.Commit(ctx); err != nil {
            return echo.NewHTTPError(500, "could not update password")
        }
        return c.NoContent(204)
    })
}

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

func tokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func SaveRefreshSession(ctx context.Context, pool *pgxpool.Pool, userID, token string) error {
	_, err := pool.Exec(ctx, `INSERT INTO public.refresh_tokens(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '7 days')`, tokenHash(token), userID)
	return err
}

func Logout(pool *pgxpool.Pool) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := c.Bind(&req); err != nil || req.RefreshToken == "" {
			return echo.NewHTTPError(400, "refresh_token required")
		}
		if _, err := pool.Exec(c.Request().Context(), `WITH revoked AS (UPDATE public.refresh_tokens SET revoked_at=now() WHERE family_id IN (SELECT family_id FROM public.refresh_tokens WHERE token_hash=$1) AND revoked_at IS NULL RETURNING user_id) UPDATE public.users SET tokens_invalid_before=date_trunc('milliseconds',clock_timestamp()) WHERE id IN (SELECT user_id FROM revoked)`, tokenHash(req.RefreshToken)); err != nil {
			return echo.NewHTTPError(500, "could not revoke session")
		}
		return c.NoContent(204)
	}
}

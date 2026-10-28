package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/imtiyazkumar/yember-core/internal/legacy/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

const adminCookieName = "yember_admin_refresh"

// Refresh credentials stay in an HttpOnly cookie; browser access tokens remain
// in memory. SameSite plus a custom header and an origin check protect mutations.
func registerAdminSessions(g *echo.Group, cfg *config.Config, pool *pgxpool.Pool) {
	admin := g.Group("/admin")
	admin.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().Header.Get("X-Yember-Admin") != "1" {
				return echo.NewHTTPError(403, "admin request header required")
			}
			if origin := c.Request().Header.Get("Origin"); origin != "" {
				allowed := false
				for _, candidate := range strings.Split(cfg.CORSAllowedOrigins, ",") {
					if strings.TrimSpace(candidate) == origin {
						allowed = true
						break
					}
				}
				if !allowed {
					return echo.NewHTTPError(403, "origin is not allowed")
				}
			}
			return next(c)
		}
	})
	setCookie := func(c echo.Context, token string, age int) {
		c.SetCookie(&http.Cookie{Name: adminCookieName, Value: token, Path: "/api/v1/auth/admin", HttpOnly: true, Secure: cfg.AdminCookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: age, Expires: time.Now().Add(time.Duration(age) * time.Second)})
	}
	respond := func(c echo.Context, user *User, refresh string) error {
		access, err := GenerateAccessToken(cfg.JWTSecret, user.ID, user.Role)
		if err != nil {
			return echo.NewHTTPError(500, "could not start administrator session")
		}
		setCookie(c, refresh, 7*24*60*60)
		c.Response().Header().Set("Cache-Control", "no-store")
		return c.JSON(200, map[string]interface{}{"access_token": access, "user": user})
	}
	admin.POST("/login", func(c echo.Context) error {
		var input struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := c.Bind(&input); err != nil || len(input.Password) > 72 {
			return echo.NewHTTPError(400, "invalid credentials")
		}
		user, err := FindByEmail(c.Request().Context(), pool, strings.TrimSpace(input.Email))
		if err != nil || user.VerifyPassword(input.Password) != nil {
			return echo.NewHTTPError(401, "invalid credentials")
		}
		if user.Role != "ADMIN" {
			return echo.NewHTTPError(403, "administrator access required")
		}
		refresh, err := GenerateRefreshToken(cfg.JWTSecret, user.ID, user.Role)
		if err != nil {
			return echo.NewHTTPError(500, "could not start session")
		}
		if err = SaveRefreshSession(c.Request().Context(), pool, user.ID, refresh); err != nil {
			return echo.NewHTTPError(503, "session service unavailable")
		}
		return respond(c, user, refresh)
	})
	admin.POST("/session", func(c echo.Context) error {
		cookie, err := c.Cookie(adminCookieName)
		if err != nil {
			return echo.NewHTTPError(401, "sign in required")
		}
		claims, err := ParseToken(cfg.JWTSecret, cookie.Value)
		if err != nil || claims.TokenType != "refresh" {
			setCookie(c, "", -1)
			return echo.NewHTTPError(401, "session expired")
		}
		tx, err := pool.Begin(c.Request().Context())
		if err != nil {
			return echo.NewHTTPError(503, "session service unavailable")
		}
		defer tx.Rollback(c.Request().Context())
		family, absolute, err := consumeRefresh(c.Request().Context(), tx, cookie.Value, claims.UserID)
		if err != nil {
			setCookie(c, "", -1)
			return echo.NewHTTPError(401, "session expired or replayed")
		}
		user := &User{}
		if err = tx.QueryRow(c.Request().Context(), `SELECT id,COALESCE(email,''),COALESCE(full_name,''),role FROM public.users WHERE id=$1 AND role='ADMIN' AND disabled_at IS NULL AND deleted_at IS NULL`, claims.UserID).Scan(&user.ID, &user.Email, &user.FullName, &user.Role); err != nil {
			setCookie(c, "", -1)
			return echo.NewHTTPError(401, "administrator access is no longer available")
		}
		refresh, err := GenerateRefreshToken(cfg.JWTSecret, user.ID, user.Role)
		if err != nil {
			return echo.NewHTTPError(500, "could not rotate session")
		}
		if _, err = tx.Exec(c.Request().Context(), `INSERT INTO public.refresh_tokens(token_hash,user_id,expires_at,family_id,family_expires_at,parent_hash) VALUES($1,$2,LEAST(now()+interval '7 days',$4::timestamptz),$3,$4,$5)`, tokenHash(refresh), user.ID, family, absolute, tokenHash(cookie.Value)); err != nil {
			return echo.NewHTTPError(503, "session service unavailable")
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return echo.NewHTTPError(503, "session service unavailable")
		}
		return respond(c, user, refresh)
	})
	admin.POST("/logout", func(c echo.Context) error {
		cookie, err := c.Cookie(adminCookieName)
		setCookie(c, "", -1)
		if err == nil {
			if _, err = pool.Exec(c.Request().Context(), `WITH revoked AS (UPDATE public.refresh_tokens SET revoked_at=now() WHERE token_hash=$1 AND revoked_at IS NULL RETURNING user_id) UPDATE public.users SET tokens_invalid_before=date_trunc('milliseconds',clock_timestamp()) WHERE id IN(SELECT user_id FROM revoked)`, tokenHash(cookie.Value)); err != nil {
				return echo.NewHTTPError(503, "session could not be revoked; please retry")
			}
		}
		return c.NoContent(204)
	})
}

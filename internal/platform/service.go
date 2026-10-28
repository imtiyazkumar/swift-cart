// Package platform implements the document contract using numeric internal
// relationships. Public routes accept and return UUIDs only.
package platform

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"
	"time"
)

type Service struct {
	DB             *pgxpool.Pool
	JWTSecret      string
	CookieSecure   bool
	AllowedOrigins []string
	SendOTP        func(context.Context, string, string) error
	Gateway        *Gateway
}
type identity struct {
	ID    int64  `json:"-"`
	UUID  string `json:"id"`
	Name  string `json:"full_name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
	Role  string `json:"role"`
}

func person(c echo.Context) *identity { u, _ := c.Get("identity").(*identity); return u }
func publicID(value string) error {
	if _, err := uuid.Parse(value); err != nil {
		return echo.NewHTTPError(400, "a public UUID is required")
	}
	return nil
}
func bad(message string) error { return echo.NewHTTPError(http.StatusBadRequest, message) }
func unavailable(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return echo.NewHTTPError(404, "record not found")
	}
	return echo.NewHTTPError(409, "record could not be updated").SetInternal(err)
}
func (s *Service) begin(c echo.Context, action string) (pgx.Tx, error) {
	tx, err := s.DB.Begin(c.Request().Context())
	if err != nil {
		return nil, err
	}
	actor := ""
	if u := person(c); u != nil {
		actor = fmtID(u.ID)
	}
	if _, err = tx.Exec(c.Request().Context(), `SELECT set_config('yember.actor_id',$1,true),set_config('yember.action',$2,true),set_config('yember.request_id',$3,true),set_config('yember.ip_address',$4,true)`, actor, action, c.Response().Header().Get(echo.HeaderXRequestID), c.RealIP()); err != nil {
		tx.Rollback(c.Request().Context())
		return nil, err
	}
	return tx, nil
}
func (s *Service) one(c echo.Context, query string, args ...any) error {
	var data json.RawMessage
	if err := s.DB.QueryRow(c.Request().Context(), query, args...).Scan(&data); err != nil {
		return unavailable(err)
	}
	return c.JSON(200, data)
}
func (s *Service) list(c echo.Context, query string, args ...any) error {
	return s.one(c, `SELECT COALESCE(jsonb_agg(row_data),'[]'::jsonb) FROM (`+query+`) AS rows(row_data)`, args...)
}
func commit(c echo.Context, tx pgx.Tx) error {
	if err := tx.Commit(c.Request().Context()); err != nil {
		return unavailable(err)
	}
	return c.NoContent(204)
}
func roles(allowed ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			u := person(c)
			if u == nil {
				return echo.NewHTTPError(401, "sign in required")
			}
			for _, role := range allowed {
				if u.Role == role {
					return next(c)
				}
			}
			return echo.NewHTTPError(403, "this role cannot access the resource")
		}
	}
}
func noStore(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error { c.Response().Header().Set("Cache-Control", "no-store"); return next(c) }
}
func (s *Service) Register(e *echo.Echo) {
	api := e.Group("/api/v1")
	s.registerAuth(api)
	s.registerWebhook(api)
	s.registerCheckoutPage(api)
	s.registerImages(api)
	protected := api.Group("", s.authenticate, noStore)
	s.registerAdmin(protected)
	s.registerBusiness(protected)
	s.registerCart(protected)
	s.registerOrders(protected)
	s.registerCash(protected)
	s.registerPayments(protected)
	s.registerBrowse(protected)
	s.registerBanks(protected)
	s.registerDocuments(protected)
	s.registerPayouts(protected)
	s.registerRefunds(protected)
	s.registerAccounts(protected)
	protected.GET("/auth/me", func(c echo.Context) error { return c.JSON(200, person(c)) })
	protected.POST("/auth/logout-all", func(c echo.Context) error {
		_, err := s.DB.Exec(c.Request().Context(), `UPDATE refresh_tokens SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, person(c).ID)
		if err != nil {
			return unavailable(err)
		}
		return c.NoContent(204)
	})
}
func validName(s string, max int) bool { return strings.TrimSpace(s) != "" && len(s) <= max }
func nowUTC() time.Time                { return time.Now().UTC() }

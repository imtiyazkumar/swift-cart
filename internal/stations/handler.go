package stations

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/imtiyazkumar/yember-core/config"
	"github.com/imtiyazkumar/yember-core/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func adminLock(c echo.Context, tx pgx.Tx) error {
	var id string
	return tx.QueryRow(c.Request().Context(), `SELECT id FROM public.users WHERE id=$1 AND role='ADMIN' AND disabled_at IS NULL AND deleted_at IS NULL FOR SHARE`, c.Get("userID")).Scan(&id)
}
func audit(c echo.Context, tx pgx.Tx, action, target string, details any) error {
	data, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = tx.Exec(c.Request().Context(), `INSERT INTO public.admin_audit_logs(admin_id,action,target_type,target_id,details) VALUES($1,$2,'station',$3,$4)`, c.Get("userID"), action, target, data)
	return err
}
func reasonValid(s string) bool { return len(strings.TrimSpace(s)) >= 3 && len(s) <= 2000 }

func Register(api *echo.Group, pool *pgxpool.Pool, cfg *config.Config, log *zap.Logger) {
	secured := api.Group("", auth.JWTMiddleware(cfg.JWTSecret, log, pool))
	list := func(all bool) echo.HandlerFunc {
		return func(c echo.Context) error {
			var data []byte
			err := pool.QueryRow(c.Request().Context(), `SELECT COALESCE(jsonb_agg(to_jsonb(s) ORDER BY s.name,s.id),'[]') FROM public.stations s WHERE active OR $1`, all).Scan(&data)
			if err != nil {
				return echo.NewHTTPError(500, "could not load stations")
			}
			return c.Blob(200, "application/json", data)
		}
	}
	secured.GET("/stations", list(false))
	admin := secured.Group("/admin", auth.RBACMiddleware("ADMIN"))
	admin.GET("/stations", list(true))
	save := func(c echo.Context) error {
		var input struct {
			Name      string   `json:"name"`
			Latitude  *float64 `json:"latitude"`
			Longitude *float64 `json:"longitude"`
			Radius    float64  `json:"radius_m"`
			Active    bool     `json:"active"`
			Reason    string   `json:"reason"`
		}
		if c.Bind(&input) != nil || Coordinates(input.Latitude, input.Longitude) != nil || len(strings.TrimSpace(input.Name)) < 2 || len(input.Name) > 160 || !(input.Radius > 0 && input.Radius <= 100000) || !reasonValid(input.Reason) {
			return echo.NewHTTPError(400, "name, valid coordinates, radius (1–100000 metres) and an audit reason are required")
		}
		ctx := c.Request().Context()
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if err = adminLock(c, tx); err != nil {
			return echo.NewHTTPError(403, "active administrator required")
		}
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(894215)`); err != nil {
			return err
		}
		id := c.Param("id")
		if id == "" {
			id = uuid.NewString()
			_, err = tx.Exec(ctx, `INSERT INTO public.stations(id,name,latitude,longitude,radius_m,active) VALUES($1,$2,$3,$4,$5,$6)`, id, strings.TrimSpace(input.Name), *input.Latitude, *input.Longitude, input.Radius, input.Active)
		} else {
			if _, e := uuid.Parse(id); e != nil {
				return echo.NewHTTPError(400, "invalid station ID")
			}
			var found string
			err = tx.QueryRow(ctx, `UPDATE public.stations SET name=$2,latitude=$3,longitude=$4,radius_m=$5,active=$6,updated_at=now() WHERE id=$1 RETURNING id`, id, strings.TrimSpace(input.Name), *input.Latitude, *input.Longitude, input.Radius, input.Active).Scan(&found)
		}
		if err != nil {
			return echo.NewHTTPError(400, "station could not be saved")
		}
		// Coverage edits stop new work without changing accepted deliveries.
		if _, err = tx.Exec(ctx, `UPDATE public.merchants SET is_active=false WHERE is_active AND NOT COALESCE(public.station_covers(station_id,latitude,longitude),false)`); err != nil {
			return err
		}
		if err = ExpireUnavailable(ctx, tx); err != nil {
			return err
		}
		if err = audit(c, tx, "STATION_SAVED", id, input); err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return c.JSON(200, map[string]string{"id": id})
	}
	admin.POST("/stations", save)
	admin.PUT("/stations/:id", save)
	request := func(c echo.Context) error {
		var input struct {
			Kind      string   `json:"kind"`
			Resource  string   `json:"resource_id"`
			Station   string   `json:"station_id"`
			Latitude  *float64 `json:"latitude"`
			Longitude *float64 `json:"longitude"`
			Reason    string   `json:"reason"`
		}
		if c.Bind(&input) != nil || !reasonValid(input.Reason) || (input.Kind != "restaurant" && input.Kind != "delivery") {
			return echo.NewHTTPError(400, "kind and station-change reason are required")
		}
		if _, err := uuid.Parse(input.Resource); err != nil {
			return echo.NewHTTPError(400, "valid resource ID required")
		}
		if _, err := uuid.Parse(input.Station); err != nil {
			return echo.NewHTTPError(400, "valid target station required")
		}
		ctx := c.Request().Context()
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if err = LockCoverage(ctx, tx); err != nil {
			return err
		}
		var owner string
		table, column := "public.merchants", "owner_id"
		if input.Kind == "delivery" {
			table, column = "delivery.delivery_partners", "user_id"
		}
		if err = tx.QueryRow(ctx, `SELECT `+column+` FROM `+table+` WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, input.Resource).Scan(&owner); err != nil {
			return echo.NewHTTPError(404, "partner profile not found")
		}
		if c.Get("role") != "ADMIN" && owner != c.Get("userID") {
			return echo.NewHTTPError(403, "own partner profile required")
		}
		if input.Kind == "restaurant" {
			if Coordinates(input.Latitude, input.Longitude) != nil {
				return echo.NewHTTPError(400, "restaurant coordinates are required")
			}
			var covers bool
			if err = tx.QueryRow(ctx, `SELECT public.station_covers($1,$2,$3)`, input.Station, *input.Latitude, *input.Longitude).Scan(&covers); err != nil {
				return err
			}
			if !covers {
				return echo.NewHTTPError(400, Unserved)
			}
		} else {
			var active bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.stations WHERE id=$1 AND active)`, input.Station).Scan(&active); err != nil {
				return err
			}
			if !active {
				return echo.NewHTTPError(400, "target station is inactive")
			}
		}
		var id string
		if err = tx.QueryRow(ctx, `INSERT INTO public.station_change_requests(kind,resource_id,requested_by,station_id,latitude,longitude,reason) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, input.Kind, input.Resource, c.Get("userID"), input.Station, input.Latitude, input.Longitude, strings.TrimSpace(input.Reason)).Scan(&id); err != nil {
			return echo.NewHTTPError(409, "a pending station change already exists or request is invalid")
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return c.JSON(201, map[string]string{"id": id})
	}
	secured.POST("/station-changes", request, auth.RBACMiddleware("MERCHANT_OWNER", "DELIVERY_PARTNER", "ADMIN"))
	secured.GET("/station-changes", func(c echo.Context) error {
		var data []byte
		err := pool.QueryRow(c.Request().Context(), `SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY created_at DESC),'[]') FROM public.station_change_requests r WHERE requested_by=$1 OR $2='ADMIN'`, c.Get("userID"), c.Get("role")).Scan(&data)
		if err != nil {
			return err
		}
		return c.Blob(200, "application/json", data)
	})
	admin.POST("/station-changes/:id/decision", func(c echo.Context) error {
		var input struct {
			Approve bool   `json:"approve"`
			Reason  string `json:"reason"`
		}
		if c.Bind(&input) != nil || !reasonValid(input.Reason) {
			return echo.NewHTTPError(400, "review reason required")
		}
		if _, err := uuid.Parse(c.Param("id")); err != nil {
			return echo.NewHTTPError(400, "invalid request ID")
		}
		ctx := c.Request().Context()
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if err = adminLock(c, tx); err != nil {
			return echo.NewHTTPError(403, "active administrator required")
		}
		if err = LockCoverage(ctx, tx); err != nil {
			return err
		}
		var kind, resource, station, status, requestedBy string
		var lat, lon *float64
		if err = tx.QueryRow(ctx, `SELECT kind,resource_id,station_id,status,latitude,longitude,requested_by FROM public.station_change_requests WHERE id=$1 FOR UPDATE`, c.Param("id")).Scan(&kind, &resource, &station, &status, &lat, &lon, &requestedBy); err != nil {
			return echo.NewHTTPError(404, "station change not found")
		}
		if status != "PENDING" {
			return echo.NewHTTPError(409, "station change already reviewed")
		}
		if input.Approve {
			if _, err = tx.Exec(ctx, `UPDATE public.station_change_requests SET reviewed_by=$2 WHERE id=$1`, c.Param("id"), c.Get("userID")); err != nil {
				return err
			}
			if requestedBy == c.Get("userID") {
				return echo.NewHTTPError(403, "another administrator must approve this station change")
			}
			var active bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.stations WHERE id=$1 AND active)`, station).Scan(&active); err != nil {
				return err
			}
			if !active {
				return echo.NewHTTPError(409, "target station is inactive")
			}
			if kind == "restaurant" {
				var valid bool
				if err = tx.QueryRow(ctx, `SELECT public.station_covers($1,$2,$3)`, station, lat, lon).Scan(&valid); err != nil {
					return err
				}
				if !valid {
					return echo.NewHTTPError(409, Unserved)
				}
				// Lock both profiles and orders consistently; no station switching with active work.
				var locked string
				if err = tx.QueryRow(ctx, `SELECT id FROM public.merchants WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, resource).Scan(&locked); err != nil {
					return err
				}
				var busy bool
				if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.orders WHERE merchant_id=$1 AND status NOT IN ('DELIVERED','CANCELLED','REJECTED','REFUNDED'))`, resource).Scan(&busy); err != nil {
					return err
				}
				if busy {
					return echo.NewHTTPError(409, "finish active orders before changing station")
				}
				_, err = tx.Exec(ctx, `UPDATE public.merchants SET station_id=$2,latitude=$3,longitude=$4,is_active=false,updated_at=now() WHERE id=$1`, resource, station, lat, lon)
				if err == nil {
					_, err = tx.Exec(ctx, `UPDATE merchants.merchant_branches SET latitude=$2,longitude=$3 WHERE id=(SELECT id FROM merchants.merchant_branches WHERE merchant_id=$1 AND deleted_at IS NULL ORDER BY created_at,id LIMIT 1)`, resource, lat, lon)
				}
			} else {
				var locked string
				if err = tx.QueryRow(ctx, `SELECT id FROM delivery.delivery_partners WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, resource).Scan(&locked); err != nil {
					return err
				}
				var busy bool
				if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM delivery.delivery_assignments WHERE partner_id=$1 AND status IN ('ASSIGNED','PICKED_UP','OUT_FOR_DELIVERY'))`, resource).Scan(&busy); err != nil {
					return err
				}
				if busy {
					return echo.NewHTTPError(409, "finish the accepted delivery before changing station")
				}
				_, err = tx.Exec(ctx, `UPDATE delivery.delivery_partners SET station_id=$2,status='OFFLINE',updated_at=now() WHERE id=$1`, resource, station)
				if err == nil {
					_, err = tx.Exec(ctx, `UPDATE delivery.offers SET status='EXPIRED',updated_at=now() WHERE partner_id=$1 AND status='PENDING'`, resource)
				}
			}
			if err != nil {
				return err
			}
			status = "APPROVED"
		} else {
			status = "REJECTED"
		}
		if _, err = tx.Exec(ctx, `UPDATE public.station_change_requests SET status=$2,reviewed_by=$3,review_reason=$4,reviewed_at=now() WHERE id=$1`, c.Param("id"), status, c.Get("userID"), input.Reason); err != nil {
			return err
		}
		if err = audit(c, tx, "STATION_CHANGE_"+status, resource, map[string]any{"request_id": c.Param("id"), "station_id": station, "reason": input.Reason}); err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return c.NoContent(204)
	})
}

// AddressStation resolves only an owned, persisted delivery address.
func AddressStation(ctx echo.Context, pool *pgxpool.Pool, id string) (string, error) {
	var lat, lon *float64
	if _, err := uuid.Parse(id); err != nil {
		return "", errors.New("select a valid delivery address")
	}
	if err := pool.QueryRow(ctx.Request().Context(), `SELECT latitude,longitude FROM users.customer_addresses WHERE id=$1 AND customer_id=$2 AND deleted_at IS NULL`, id, ctx.Get("userID")).Scan(&lat, &lon); err != nil {
		return "", errors.New("delivery address not found")
	}
	station, err := Resolve(ctx.Request().Context(), pool, lat, lon)
	if err != nil {
		return "", errors.New(DeliveryUnavailable)
	}
	return station, nil
}

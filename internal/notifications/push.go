package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/imtiyazkumar/yember-core/config"
	"github.com/imtiyazkumar/yember-core/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"
)

var pushToken = regexp.MustCompile(`^(ExpoPushToken|ExponentPushToken)\[[A-Za-z0-9_-]+\]$`)

func RegisterPush(api *echo.Group, pool *pgxpool.Pool, cfg *config.Config, log *zap.Logger) {
	group := api.Group("/notifications", auth.JWTMiddleware(cfg.JWTSecret, log, pool))
	group.PUT("/device", func(c echo.Context) error {
		var in struct {
			Token string `json:"token"`
			App   string `json:"app"`
		}
		if c.Bind(&in) != nil || len(in.Token) > 250 || !pushToken.MatchString(in.Token) {
			return echo.NewHTTPError(400, "valid device token required")
		}
		roles := map[string]string{"customer": "CUSTOMER", "restaurant": "MERCHANT_OWNER", "delivery": "DELIVERY_PARTNER"}
		if roles[in.App] != c.Get("role") {
			return echo.NewHTTPError(403, "app role mismatch")
		}
		_, err := pool.Exec(c.Request().Context(), `INSERT INTO notifications.devices(token,user_id,app) VALUES($1,$2,$3) ON CONFLICT(token) DO UPDATE SET user_id=EXCLUDED.user_id,app=EXCLUDED.app,updated_at=now(),disabled_at=NULL`, in.Token, c.Get("userID"), in.App)
		if err != nil {
			return echo.NewHTTPError(503, "device registration unavailable")
		}
		return c.NoContent(204)
	})
	group.DELETE("/device", func(c echo.Context) error {
		_, err := pool.Exec(c.Request().Context(), `UPDATE notifications.devices SET disabled_at=now() WHERE user_id=$1`, c.Get("userID"))
		if err != nil {
			return echo.NewHTTPError(503, "device removal unavailable")
		}
		return c.NoContent(204)
	})
	group.GET("/preferences", func(c echo.Context) error {
		var raw json.RawMessage
		err := pool.QueryRow(c.Request().Context(), `SELECT COALESCE((SELECT jsonb_build_object('order_updates',order_updates,'marketing',marketing) FROM users.notification_preferences WHERE user_id=$1),' {"order_updates":true,"marketing":false}'::jsonb)`, c.Get("userID")).Scan(&raw)
		if err != nil {
			return echo.NewHTTPError(503, "preferences unavailable")
		}
		return c.JSON(200, raw)
	})
	group.PUT("/preferences", func(c echo.Context) error {
		var in struct {
			Order     bool `json:"order_updates"`
			Marketing bool `json:"marketing"`
		}
		if c.Bind(&in) != nil {
			return echo.NewHTTPError(400, "invalid preferences")
		}
		_, err := pool.Exec(c.Request().Context(), `INSERT INTO users.notification_preferences(user_id,order_updates,marketing) VALUES($1,$2,$3) ON CONFLICT(user_id) DO UPDATE SET order_updates=$2,marketing=$3,updated_at=now()`, c.Get("userID"), in.Order, in.Marketing)
		if err != nil {
			return echo.NewHTTPError(503, "preferences unavailable")
		}
		return c.NoContent(204)
	})
}
func expoRequest(ctx context.Context, path string, body, out any) error {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://exp.host/--/api/v2/push/"+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := os.Getenv("EXPO_PUSH_ACCESS_TOKEN"); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return errors.New("push transport unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return errors.New("push provider rejected request")
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}
func RunPush(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger) {
	if os.Getenv("PUSH_DELIVERY_ENABLED") != "true" {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := pushOne(ctx, pool); err != nil {
				log.Warn("push queue attempt failed", zap.Error(err))
			}
			if err := pushReceipt(ctx, pool); err != nil {
				log.Warn("push receipt check failed", zap.Error(err))
			}
		}
	}
}
func pushOne(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id, user, title, body string
	var data json.RawMessage
	var attempts int
	err = tx.QueryRow(ctx, `SELECT id,user_id,title,body,data,attempts FROM notifications.push_outbox WHERE status IN ('PENDING','SENDING') AND next_attempt_at<=now() ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id, &user, &title, &body, &data, &attempts)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT d.token FROM notifications.devices d JOIN public.users u ON u.id=d.user_id WHERE d.user_id=$1 AND d.disabled_at IS NULL AND u.disabled_at IS NULL AND u.deleted_at IS NULL AND COALESCE((SELECT order_updates FROM users.notification_preferences WHERE user_id=$1),true) ORDER BY d.updated_at DESC LIMIT 20`, user)
	if err != nil {
		return err
	}
	tokens := []string{}
	for rows.Next() {
		var token string
		if err = rows.Scan(&token); err != nil {
			rows.Close()
			return err
		}
		tokens = append(tokens, token)
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}
	if _, err = tx.Exec(ctx, `UPDATE notifications.push_outbox SET status='SENDING',attempts=attempts+1,next_attempt_at=now()+interval '2 minutes' WHERE id=$1`, id); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	messages := []map[string]any{}
	for _, token := range tokens {
		messages = append(messages, map[string]any{"to": token, "title": title, "body": body, "data": data, "sound": "default", "channelId": "orders"})
	}
	var response struct {
		Data []struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Details struct {
				Error string `json:"error"`
			} `json:"details"`
		} `json:"data"`
	}
	if len(messages) > 0 {
		err = expoRequest(ctx, "send", messages, &response)
		if err == nil && len(response.Data) != len(messages) {
			err = errors.New("invalid push response")
		}
	}
	if err == nil {
		for i, ticket := range response.Data {
			if ticket.Details.Error == "DeviceNotRegistered" {
				_, _ = pool.Exec(ctx, `UPDATE notifications.devices SET disabled_at=now() WHERE token=$1`, tokens[i])
			} else if ticket.Status != "ok" {
				err = errors.New("push ticket rejected")
			} else if ticket.ID != "" {
				_, err = pool.Exec(ctx, `INSERT INTO notifications.push_tickets(ticket_id,token,outbox_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, ticket.ID, tokens[i], id)
				if err != nil {
					break
				}
			}
		}
	}
	state := "SENT"
	if err != nil {
		state = "PENDING"
		if attempts >= 7 {
			state = "FAILED"
		}
	}
	_, updateErr := pool.Exec(ctx, `UPDATE notifications.push_outbox SET status=$2,next_attempt_at=now()+interval '2 minutes',last_error=CASE WHEN $2='SENT' THEN NULL ELSE 'Push provider delivery requires retry' END WHERE id=$1`, id, state)
	if updateErr != nil {
		return updateErr
	}
	return err
}
func pushReceipt(ctx context.Context, pool *pgxpool.Pool) error {
	var id, token string
	err := pool.QueryRow(ctx, `SELECT ticket_id,token FROM notifications.push_tickets WHERE checked_at IS NULL AND created_at<now()-interval '15 minutes' ORDER BY created_at LIMIT 1`).Scan(&id, &token)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var res struct {
		Data map[string]struct {
			Status  string `json:"status"`
			Details struct {
				Error string `json:"error"`
			} `json:"details"`
		} `json:"data"`
	}
	if err = expoRequest(ctx, "getReceipts", map[string]any{"ids": []string{id}}, &res); err != nil {
		return err
	}
	receipt, ok := res.Data[id]
	if !ok {
		return nil
	}
	if receipt.Details.Error == "DeviceNotRegistered" {
		_, err = pool.Exec(ctx, `UPDATE notifications.devices SET disabled_at=now() WHERE token=$1`, token)
		if err != nil {
			return err
		}
	}
	_, err = pool.Exec(ctx, `UPDATE notifications.push_tickets SET checked_at=now() WHERE ticket_id=$1`, id)
	return err
}

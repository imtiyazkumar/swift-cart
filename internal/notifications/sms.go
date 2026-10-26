package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type TextBee struct {
	Client  *http.Client
	BaseURL string
	Key     string
	Device  string
}

func (t TextBee) Send(ctx context.Context, recipient, body string) (string, int, error) {
	if t.Key == "" {
		return "", 0, errors.New("TextBee key unavailable")
	}
	base := t.BaseURL
	if base == "" {
		base = "https://api.textbee.dev/api/v1"
	}
	data := map[string]any{"recipients": []string{recipient}, "message": body}
	if t.Device != "" {
		data["deviceId"] = t.Device
	}
	raw, _ := json.Marshal(data)
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(base, "/")+"/gateway/send-sms", bytes.NewReader(raw))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", t.Key)
	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return "", 0, errors.New("SMS acceptance unknown")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", res.StatusCode, errors.New("SMS provider rejected request")
	}
	var result struct {
		Data struct {
			Success bool   `json:"success"`
			ID      string `json:"smsBatchId"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&result) != nil || !result.Data.Success || result.Data.ID == "" {
		return "", 200, errors.New("SMS acceptance unknown")
	}
	return result.Data.ID, 200, nil
}
func RunSMS(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger) {
	if os.Getenv("TEXTBEE_DELIVERY_ENABLED") != "true" {
		return
	}
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := sendSMS(ctx, pool); err != nil {
				log.Warn("SMS delivery requires review", zap.Error(err))
			}
		}
	}
}
func sendSMS(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `UPDATE notifications.sms_outbox SET status='EXPIRED',body='' WHERE expires_at<=now() AND status NOT IN ('DELIVERED','EXPIRED')`); err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id, recipient, body string
	err = tx.QueryRow(ctx, `SELECT id,recipient,body FROM notifications.sms_outbox WHERE status='PENDING' AND next_attempt_at<=now() AND expires_at>now() ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id, &recipient, &body)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE notifications.sms_outbox SET status='SENDING' WHERE id=$1`, id); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	reference, status, sendErr := (TextBee{Key: os.Getenv("TEXTBEE_API_KEY"), Device: os.Getenv("TEXTBEE_DEVICE_ID"), BaseURL: os.Getenv("TEXTBEE_BASE_URL")}).Send(ctx, recipient, body)
	state := "QUEUED"
	if sendErr != nil {
		state = "UNKNOWN"
		if status == 429 {
			state = "PENDING"
		}
		if status == 400 || status == 401 {
			state = "FAILED"
		}
	}
	_, err = pool.Exec(ctx, `UPDATE notifications.sms_outbox SET status=$2,provider_reference=NULLIF($3,''),body=CASE WHEN $2 IN ('QUEUED','FAILED') THEN '' ELSE body END,next_attempt_at=now()+interval '1 minute' WHERE id=$1`, id, state, reference)
	if err != nil {
		return err
	}
	return sendErr
}

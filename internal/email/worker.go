package email

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"

    "github.com/imtiyazkumar/yember-core/config"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "go.uber.org/zap"
)

type Message struct {
    From    string   `json:"from"`
    To      []string `json:"to"`
    Subject string   `json:"subject"`
    Text    string   `json:"text"`
}

type Resend struct {
    Key      string
    Client   *http.Client
    Endpoint string
}

func (r Resend) Send(ctx context.Context, id string, message Message) error {
    if r.Key == "" {
        return fmt.Errorf("Resend is not configured")
    }
    body, err := json.Marshal(message)
    if err != nil {
        return err
    }
    endpoint := r.Endpoint
    if endpoint == "" {
        endpoint = "https://api.resend.com/emails"
    }
    request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
    if err != nil {
        return err
    }
    request.Header.Set("Authorization", "Bearer "+r.Key)
    request.Header.Set("Content-Type", "application/json")
    request.Header.Set("Idempotency-Key", "yember-email/"+id)
    client := r.Client
    if client == nil {
        client = &http.Client{Timeout: 15 * time.Second}
    }
    response, err := client.Do(request)
    if err != nil {
        return fmt.Errorf("email provider connection failed")
    }
    defer response.Body.Close()
    _, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 65536))
    if response.StatusCode < 200 || response.StatusCode >= 300 {
        return fmt.Errorf("email provider returned HTTP %d", response.StatusCode)
    }
    return nil
}

// Run is explicitly opt-in: local tests must never send live transactional email.
func Run(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, log *zap.Logger) {
    if !cfg.EmailDeliveryEnabled || cfg.ResendAPIKey == "" || cfg.EmailFrom == "" {
        log.Info("email delivery disabled; queued messages remain visible to administrators")
        return
    }
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if err := deliverOne(ctx, pool, cfg); err != nil {
                log.Warn("email delivery attempt failed", zap.Error(err))
            }
        }
    }
}

func deliverOne(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config) error {
    tx, err := pool.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    var id, to, subject, body string
    var attempts int
    err = tx.QueryRow(ctx, `SELECT id,recipient,subject,body,attempts FROM notifications.email_outbox WHERE status IN ('PENDING','SENDING') AND next_attempt_at<=now() ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &to, &subject, &body, &attempts)
    if err == pgx.ErrNoRows {
        return nil
    }
    if err != nil {
        return err
    }
    // Resend idempotency keys expire after 24 hours. The retry window stays below it.
    _, err = tx.Exec(ctx, `UPDATE notifications.email_outbox SET status='SENDING',attempts=attempts+1,next_attempt_at=now()+interval '1 minute' WHERE id=$1`, id)
    if err != nil {
        return err
    }
    if err = tx.Commit(ctx); err != nil {
        return err
    }
    sendErr := (Resend{Key: cfg.ResendAPIKey}).Send(ctx, id, Message{From: cfg.EmailFrom, To: []string{to}, Subject: subject, Text: body})
    if sendErr == nil {
        _, err = pool.Exec(ctx, `UPDATE notifications.email_outbox SET status='SENT',sent_at=now(),body='',last_error='' WHERE id=$1`, id)
        return err
    }
    state := "PENDING"
    if attempts+1 >= 8 {
        state = "FAILED"
    }
    _, err = pool.Exec(ctx, `UPDATE notifications.email_outbox SET status=$2,last_error=$3,next_attempt_at=now()+make_interval(secs => $4) WHERE id=$1`, id, state, sendErr.Error(), (1<<min(attempts, 7))*30)
    if err != nil {
        return err
    }
    return sendErr
}

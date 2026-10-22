package fees

import (
    "context"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "github.com/imtiyazkumar/yember-core/internal/stations"
    "io"
    "net/http"
    "net/url"
    "os"
    "strconv"
    "strings"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

type Policy struct {
    Amount   int64  `json:"amount"`
    Currency string `json:"currency"`
    Terms    string `json:"terms"`
}

func (p Policy) Validate() error {
    if p.Amount <= 0 || p.Amount > 100000000 || len(p.Currency) != 3 || p.Currency != strings.ToUpper(p.Currency) || strings.TrimSpace(p.Terms) == "" || len(p.Terms) > 2000 {
        return errors.New("positive amount in minor currency units, uppercase 3-letter currency and payment/tax terms required")
    }
    for _, ch := range p.Currency {
        if ch < 'A' || ch > 'Z' {
            return errors.New("invalid currency")
        }
    }
    return nil
}

type Checkout struct {
    URL    string `json:"url"`
    Status string `json:"status"`
}
type Service struct {
    pool     *pgxpool.Pool
    Client   *http.Client
    Endpoint string
}

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) Policy(ctx context.Context) (*Policy, error) {
    var raw []byte
    if err := s.pool.QueryRow(ctx, `SELECT value FROM public.platform_settings WHERE key='onboarding_fee'`).Scan(&raw); err != nil {
        return nil, errors.New("onboarding fee is not configured")
    }
    var p Policy
    if err := json.Unmarshal(raw, &p); err != nil {
        return nil, err
    }
    return &p, p.Validate()
}

func (s *Service) SetPolicy(ctx context.Context, actor string, p Policy) error {
    if err := p.Validate(); err != nil {
        return err
    }
    tx, err := s.pool.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    raw, err := json.Marshal(p)
    if err != nil {
        return err
    }
    if _, err = tx.Exec(ctx, `INSERT INTO public.platform_settings(key,value,updated_by) VALUES('onboarding_fee',$1,$2) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_by=EXCLUDED.updated_by,updated_at=now()`, raw, actor); err != nil {
        return err
    }
    if _, err = tx.Exec(ctx, `INSERT INTO public.admin_audit_logs(admin_id,action,target_type,details) VALUES($1,'ONBOARDING_FEE_UPDATED','platform_settings',$2)`, actor, raw); err != nil {
        return err
    }
    return tx.Commit(ctx)
}

func (s *Service) CreateCheckout(ctx context.Context, owner string) (*Checkout, error) {
    key := os.Getenv("STRIPE_SECRET_KEY")
    successURL, cancelURL := os.Getenv("PAYMENT_SUCCESS_URL"), os.Getenv("PAYMENT_CANCEL_URL")
    if key == "" || successURL == "" || cancelURL == "" {
        return nil, errors.New("Stripe checkout is not configured")
    }
    policy, err := s.Policy(ctx)
    if err != nil {
        return nil, err
    }
    tx, err := s.pool.Begin(ctx)
    if err != nil {
        return nil, err
    }
    defer tx.Rollback(ctx)
    if err = stations.LockCoverage(ctx, tx); err != nil {
        return nil, err
    }
    var merchant, approval, email string
    if err = tx.QueryRow(ctx, `SELECT m.id,m.approval_status,u.email FROM public.merchants m JOIN public.users u ON u.id=m.owner_id WHERE m.owner_id=$1 AND m.deleted_at IS NULL ORDER BY m.created_at LIMIT 1 FOR UPDATE OF m`, owner).Scan(&merchant, &approval, &email); err != nil {
        return nil, errors.New("restaurant not found")
    }
    if approval != "APPROVED" {
        return nil, errors.New("restaurant approval required before payment")
    }
    if err = stations.ValidateRestaurant(ctx, tx, merchant); err != nil {
        return nil, err
    }
    var existing Checkout
    err = tx.QueryRow(ctx, `SELECT checkout_url,status FROM payments.onboarding_payments WHERE merchant_id=$1`, merchant).Scan(&existing.URL, &existing.Status)
    if err == nil {
        return &existing, nil
    }
    if err != pgx.ErrNoRows {
        return nil, err
    }
    form := url.Values{"mode": {"payment"}, "success_url": {successURL}, "cancel_url": {cancelURL}, "client_reference_id": {merchant}, "customer_email": {email}, "line_items[0][quantity]": {"1"}, "line_items[0][price_data][currency]": {strings.ToLower(policy.Currency)}, "line_items[0][price_data][unit_amount]": {strconv.FormatInt(policy.Amount, 10)}, "line_items[0][price_data][product_data][name]": {"Yember restaurant onboarding"}, "line_items[0][price_data][product_data][description]": {policy.Terms}, "metadata[merchant_id]": {merchant}}
    endpoint := s.Endpoint
    if endpoint == "" {
        endpoint = "https://api.stripe.com/v1/checkout/sessions"
    }
    request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
    if err != nil {
        return nil, err
    }
    request.SetBasicAuth(key, "")
    request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    request.Header.Set("Idempotency-Key", "yember-onboarding-"+merchant)
    client := s.Client
    if client == nil {
        client = &http.Client{Timeout: 15 * time.Second}
    }
    response, err := client.Do(request)
    if err != nil {
        return nil, errors.New("payment provider unavailable; no payment has been confirmed")
    }
    defer response.Body.Close()
    if response.StatusCode < 200 || response.StatusCode >= 300 {
        return nil, fmt.Errorf("payment provider returned HTTP %d", response.StatusCode)
    }
    var session struct {
        ID  string `json:"id"`
        URL string `json:"url"`
    }
    if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&session); err != nil || session.ID == "" || session.URL == "" {
        return nil, errors.New("invalid payment provider response")
    }
    if _, err = tx.Exec(ctx, `INSERT INTO payments.onboarding_payments(merchant_id,session_id,checkout_url,amount,currency) VALUES($1,$2,$3,$4,$5)`, merchant, session.ID, session.URL, policy.Amount, policy.Currency); err != nil {
        return nil, err
    }
    if err = tx.Commit(ctx); err != nil {
        return nil, err
    }
    return &Checkout{URL: session.URL, Status: "PENDING"}, nil
}

func VerifySignature(body []byte, header, secret string, now time.Time) bool {
    if secret == "" {
        return false
    }
    var timestamp string
    signatures := []string{}
    for _, part := range strings.Split(header, ",") {
        key, value, ok := strings.Cut(part, "=")
        if !ok {
            continue
        }
        if key == "t" {
            timestamp = value
        }
        if key == "v1" {
            signatures = append(signatures, value)
        }
    }
    seconds, err := strconv.ParseInt(timestamp, 10, 64)
    if err != nil {
        return false
    }
    age := now.Sub(time.Unix(seconds, 0))
    if age > 5*time.Minute || age < -5*time.Minute {
        return false
    }
    mac := hmac.New(sha256.New, []byte(secret))
    _, _ = mac.Write([]byte(timestamp + "."))
    _, _ = mac.Write(body)
    expected := mac.Sum(nil)
    for _, signature := range signatures {
        actual, err := hex.DecodeString(signature)
        if err == nil && hmac.Equal(actual, expected) {
            return true
        }
    }
    return false
}

func (s *Service) ApplyEvent(ctx context.Context, body []byte) error {
    var event struct {
        ID   string `json:"id"`
        Type string `json:"type"`
        Data struct {
            Object struct {
                ID            string `json:"id"`
                PaymentStatus string `json:"payment_status"`
                Amount        int64  `json:"amount_total"`
                Currency      string `json:"currency"`
                Reference     string `json:"client_reference_id"`
            } `json:"object"`
        } `json:"data"`
    }
    if err := json.Unmarshal(body, &event); err != nil || event.ID == "" {
        return errors.New("invalid payment event")
    }
    if event.Type != "checkout.session.completed" && event.Type != "checkout.session.async_payment_succeeded" {
        return nil
    }
    session := event.Data.Object
    if session.PaymentStatus != "paid" {
        return nil
    }
    tx, err := s.pool.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    if err = stations.LockCoverage(ctx, tx); err != nil {
        return err
    }
    tag, err := tx.Exec(ctx, `INSERT INTO payments.onboarding_events(event_id) VALUES($1) ON CONFLICT DO NOTHING`, event.ID)
    if err != nil {
        return err
    }
    if tag.RowsAffected() == 0 {
        return nil
    }
    var merchant, status string
    var amount int64
    var currency string
    if err = tx.QueryRow(ctx, `SELECT merchant_id,status,amount,currency FROM payments.onboarding_payments WHERE session_id=$1 FOR UPDATE`, session.ID).Scan(&merchant, &status, &amount, &currency); err != nil {
        return errors.New("payment session not found")
    }
    if merchant != session.Reference || amount != session.Amount || !strings.EqualFold(currency, session.Currency) {
        return errors.New("payment amount, currency or merchant mismatch")
    }
    if status == "PAID" {
        return tx.Commit(ctx)
    }
    if _, err = tx.Exec(ctx, `UPDATE payments.onboarding_payments SET status='PAID',paid_at=now() WHERE merchant_id=$1`, merchant); err != nil {
        return err
    }
    if _, err = tx.Exec(ctx, `UPDATE public.merchants SET fee_payment_status='PAID',onboarding_status='COMPLETED',is_active=(approval_status='APPROVED' AND COALESCE(public.station_covers(station_id,latitude,longitude),false)),updated_at=now() WHERE id=$1`, merchant); err != nil {
        return err
    }
    if _, err = tx.Exec(ctx, `INSERT INTO notifications.email_outbox(recipient,subject,body) SELECT email,'Yember onboarding payment confirmed','Your onboarding payment has been verified. Opening also requires approval and valid active station coverage.' FROM public.merchants WHERE id=$1`, merchant); err != nil {
        return err
    }
    return tx.Commit(ctx)
}

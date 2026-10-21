package fees

import (
    "context"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "net/http"
    "net/http/httptest"
    "strconv"
    "strings"
    "sync/atomic"
    "testing"
    "time"

    "github.com/imtiyazkumar/yember-core/internal/testdb"
)

func TestWebhookSignatureRejectsTamperingAndReplay(t *testing.T) {
    body := []byte(`{"id":"evt_test"}`)
    now := time.Now()
    ts := strconv.FormatInt(now.Unix(), 10)
    mac := hmac.New(sha256.New, []byte("test-secret"))
    _, _ = mac.Write([]byte(ts + "."))
    _, _ = mac.Write(body)
    header := "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
    if !VerifySignature(body, header, "test-secret", now) {
        t.Fatal("valid signature rejected")
    }
    if VerifySignature([]byte("tampered"), header, "test-secret", now) {
        t.Fatal("tampered payload accepted")
    }
    if VerifySignature(body, header, "test-secret", now.Add(6*time.Minute)) {
        t.Fatal("stale signature accepted")
    }
    if VerifySignature(body, header, "", now) {
        t.Fatal("unconfigured webhook accepted")
    }
}

func TestCheckoutAndVerifiedActivationAreIdempotent(t *testing.T) {
    pool := testdb.New(t)
    ctx := context.Background()
    var owner, admin, merchant string
    if err := pool.QueryRow(ctx, `INSERT INTO public.users(email,password_hash,role) VALUES('owner@test.invalid','hash','MERCHANT_OWNER') RETURNING id`).Scan(&owner); err != nil {
        t.Fatal(err)
    }
    if err := pool.QueryRow(ctx, `INSERT INTO public.users(email,password_hash,role) VALUES('admin@test.invalid','hash','ADMIN') RETURNING id`).Scan(&admin); err != nil {
        t.Fatal(err)
    }
    if err := pool.QueryRow(ctx, `INSERT INTO public.merchants(name,email,owner_id,is_active,approval_status) VALUES('Kitchen','k@test.invalid',$1,false,'APPROVED') RETURNING id`, owner).Scan(&merchant); err != nil {
        t.Fatal(err)
    }
    var requests atomic.Int32
    provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        requests.Add(1)
        if r.Header.Get("Idempotency-Key") != "yember-onboarding-"+merchant {
            t.Error("missing payment idempotency key")
        }
        _ = r.ParseForm()
        if r.Form.Get("line_items[0][price_data][unit_amount]") != "25000" {
            t.Error("client controlled payment amount")
        }
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"id":"cs_isolated_test","url":"https://checkout.stripe.com/test"}`))
    }))
    defer provider.Close()
    t.Setenv("STRIPE_SECRET_KEY", "sk_test_local")
    t.Setenv("PAYMENT_SUCCESS_URL", "https://example.invalid/success")
    t.Setenv("PAYMENT_CANCEL_URL", "https://example.invalid/cancel")
    service := New(pool)
    service.Endpoint = provider.URL
    if err := service.SetPolicy(ctx, admin, Policy{Amount: 25000, Currency: "INR", Terms: "Test policy only"}); err != nil {
        t.Fatal(err)
    }
    for i := 0; i < 2; i++ {
        checkout, err := service.CreateCheckout(ctx, owner)
        if err != nil || checkout.Status != "PENDING" {
            t.Fatalf("checkout failed: %v", err)
        }
    }
    if requests.Load() != 1 {
        t.Fatal("duplicate provider checkout created")
    }
    payload := fmt.Sprintf(`{"id":"evt_isolated_test","type":"checkout.session.completed","data":{"object":{"id":"cs_isolated_test","payment_status":"paid","amount_total":25000,"currency":"inr","client_reference_id":"%s"}}}`, merchant)
    invalid := strings.Replace(payload, `"amount_total":25000`, `"amount_total":1`, 1)
    if err := service.ApplyEvent(ctx, []byte(invalid)); err == nil {
        t.Fatal("wrong payment amount accepted")
    }
    for i := 0; i < 2; i++ {
        if err := service.ApplyEvent(ctx, []byte(payload)); err != nil {
            t.Fatal(err)
        }
    }
    var active bool
    if err := pool.QueryRow(ctx, `SELECT is_active FROM public.merchants WHERE id=$1`, merchant).Scan(&active); err != nil || !active {
        t.Fatal("verified payment did not activate approved restaurant")
    }
    var count int
    if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications.email_outbox`).Scan(&count); err != nil || count != 1 {
        t.Fatal("payment notification duplicated")
    }
}

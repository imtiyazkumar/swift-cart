package email

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestResendIdempotencyAndProviderFailures(t *testing.T) {
    status := http.StatusOK
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Idempotency-Key") != "yember-email/event-1" {
            t.Error("missing authentication or idempotency key")
        }
        w.WriteHeader(status)
    }))
    defer server.Close()
    sender := Resend{Key: "test-key", Endpoint: server.URL}
    if err := sender.Send(context.Background(), "event-1", Message{From: "test@test.invalid", To: []string{"recipient@test.invalid"}, Text: "Test"}); err != nil {
        t.Fatal(err)
    }
    status = http.StatusTooManyRequests
    if err := sender.Send(context.Background(), "event-1", Message{}); err == nil {
        t.Fatal("provider failure reported as success")
    }
}

package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTextBeeAcceptanceIsQueuedNotDelivered(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gateway/send-sms" || r.Header.Get("x-api-key") != "test-secret" {
			t.Error("invalid TextBee request")
		}
		var body struct {
			Recipients []string `json:"recipients"`
			Message    string   `json:"message"`
			Device     string   `json:"deviceId"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Recipients) != 1 || body.Recipients[0] != "+919876543210" || body.Device != "fixture-device" {
			t.Error("incorrect TextBee body")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"success":true,"smsBatchId":"batch-fixture"}}`))
	}))
	defer server.Close()
	id, status, err := (TextBee{BaseURL: server.URL, Key: "test-secret", Device: "fixture-device"}).Send(context.Background(), "+919876543210", "Code 123456")
	if err != nil || id != "batch-fixture" || status != 200 {
		t.Fatal(id, status, err)
	}
}

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func SendOTPFromEnv() func(context.Context, string, string) error {
	key, device := os.Getenv("TEXTBEE_API_KEY"), os.Getenv("TEXTBEE_DEVICE_ID")
	if os.Getenv("TEXTBEE_DELIVERY_ENABLED") != "true" || key == "" || device == "" {
		return nil
	}
	base := strings.TrimRight(os.Getenv("TEXTBEE_BASE_URL"), "/")
	if base == "" {
		base = "https://api.textbee.dev/api/v1"
	}
	client := &http.Client{Timeout: 12 * time.Second}
	return func(ctx context.Context, phone, code string) error {
		if !strings.HasPrefix(phone, "+") {
			phone = "+91" + phone
		}
		body, _ := json.Marshal(map[string]any{"recipients": []string{phone}, "message": "Your Yember verification code is " + code + ". It expires in 10 minutes. Never share this code."})
		request, err := http.NewRequestWithContext(ctx, "POST", base+"/gateway/devices/"+url.PathEscape(device)+"/send-sms", bytes.NewReader(body))
		if err != nil {
			return err
		}
		request.Header.Set("x-api-key", key)
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("SMS delivery unavailable")
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("SMS delivery unavailable")
		}
		return nil
	}
}

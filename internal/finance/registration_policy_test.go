package finance

import (
	"context"
	"encoding/json"
	"github.com/imtiyazkumar/yember-core/internal/devseed"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRegistrationAmountsAndSeparateApproval(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if err := devseed.Seed(ctx, p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RAZORPAY_KEY_ID", "rzp_test_registration")
	t.Setenv("RAZORPAY_KEY_SECRET", "test-secret")
	if _, err := p.Exec(ctx, `UPDATE public.restaurants SET fee_payment_status='UNPAID',approval_status='PENDING',is_active=false WHERE id=$1`, devseed.RestaurantID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		user   string
		amount int64
	}{{devseed.OwnerID, 20000}, {devseed.DeliveryUserID, 15000}} {
		var requested map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" || r.URL.Path != "/payment_links" {
				t.Error("unexpected provider request")
			}
			if err := json.NewDecoder(r.Body).Decode(&requested); err != nil {
				t.Error(err)
			}
			json.NewEncoder(w).Encode(PaymentLink{ID: "plink_" + tc.user, URL: "https://rzp.io/test", Expires: time.Now().Add(30 * time.Minute).Unix()})
		}))
		s := New(p)
		s.Gateway.BaseURL = server.URL
		result, err := s.OnboardingCheckout(ctx, tc.user)
		if err != nil {
			server.Close()
			t.Fatal(err)
		}
		if result.Status != "PENDING" || requested["amount"] != float64(tc.amount) {
			t.Fatalf("registration price mismatch: %+v", requested)
		}
		id := requested["reference_id"].(string)
		if err = s.captureOnboarding(ctx, PaymentLink{ID: "plink_" + tc.user, Reference: id, Currency: "INR"}, "pay_"+tc.user, tc.amount, "INR"); err != nil {
			server.Close()
			t.Fatal(err)
		}
		if err = s.Refund(ctx, devseed.AdminID, id, "registration-refund", "Refund registration", tc.amount); err == nil {
			t.Fatal("registration refund accepted")
		}
		server.Close()
	}
	var approval string
	var active bool
	if err := p.QueryRow(ctx, `SELECT approval_status,is_active FROM public.restaurants WHERE id=$1`, devseed.RestaurantID).Scan(&approval, &active); err != nil || approval != "PENDING" || active {
		t.Fatal("payment changed business approval", err)
	}
}

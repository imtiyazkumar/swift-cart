package finance

import (
	"context"
	"encoding/json"
	"github.com/imtiyazkumar/yember-core/internal/legacy/auth"
	"github.com/imtiyazkumar/yember-core/internal/legacy/config"
	"github.com/imtiyazkumar/yember-core/internal/legacy/devseed"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSettlementReservationApprovalAndConfirmedPayment(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if err := devseed.Seed(ctx, p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	s := New(p)
	var a, b, user, bank string
	if err := p.QueryRow(ctx, `INSERT INTO public.users(email,password_hash,role) VALUES('finance-a@test.invalid','hash','ADMIN') RETURNING id`).Scan(&a); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO public.users(email,password_hash,role) VALUES('finance-b@test.invalid','hash','ADMIN') RETURNING id`).Scan(&b); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `SELECT owner_id FROM public.restaurants WHERE id=$1`, devseed.RestaurantID).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO finance.bank_accounts(user_id,ciphertext,key_id,last_four,status) VALUES($1,'data','test','1234','VERIFIED') RETURNING id`, user).Scan(&bank); err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = post(ctx, tx, "opening/one", "ADJUSTMENT", "", "verified-opening", "Opening balance", a, Line{"ADJUSTMENT", "", "adjustment", 85000}, Line{"PARTNER_PAYABLE", user, "adjustment", -85000}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	batch, err := s.Generate(ctx, a, time.Now().Add(31*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Generate(ctx, a, time.Now().Add(31*24*time.Hour))
	if err != nil || replay != batch {
		t.Fatal("batch replay", err)
	}
	if err = s.Approve(ctx, a, batch, "Self approval must fail"); err == nil {
		t.Fatal("self approval allowed")
	}
	if err = s.Approve(ctx, b, batch, "Reviewed eligible earnings"); err != nil {
		t.Fatal(err)
	}
	var payout string
	if err = p.QueryRow(ctx, `SELECT id FROM finance.payouts WHERE batch_id=$1`, batch).Scan(&payout); err != nil {
		t.Fatal(err)
	}
	if err = s.Initiate(ctx, a, payout, "Initiate bank instruction"); err != nil {
		t.Fatal(err)
	}
	if err = s.Confirm(ctx, a, payout, "SUCCEEDED", "UTR-SELF-INVALID"); err == nil {
		t.Fatal("self reconciliation allowed")
	}
	// A lost response must retain its original provider request key/body. Manual
	// failure claims must never unlock a fresh RazorpayX payout attempt.
	t.Setenv("FINANCE_PAYOUT_PROVIDER", "RAZORPAYX")
	t.Setenv("FINANCE_LIVE_APPROVED", "true")
	t.Setenv("RAZORPAYX_ACCOUNT_NUMBER", "source-fixture")
	t.Setenv("RAZORPAYX_KEY_ID", "rzp_test_fixture")
	t.Setenv("RAZORPAYX_KEY_SECRET", "test-secret")
	if _, err = p.Exec(ctx, `UPDATE finance.bank_accounts SET provider_fund_account='fa_fixture' WHERE id=$1`, bank); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, `UPDATE finance.payouts SET provider='RAZORPAYX' WHERE id=$1`, payout); err != nil {
		t.Fatal(err)
	}
	var firstKey, firstBody string
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			calls++
			body, _ := io.ReadAll(r.Body)
			if calls == 1 {
				firstKey = r.Header.Get("X-Payout-Idempotency")
				firstBody = string(body)
				w.WriteHeader(503)
				return
			}
			if firstKey == "" || r.Header.Get("X-Payout-Idempotency") != firstKey || string(body) != firstBody {
				t.Error("uncertain payout key/body changed")
			}
			_, _ = w.Write([]byte(`{"id":"pout_fixture","amount":85000,"status":"processing"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"pout_fixture","amount":85000,"currency":"INR","status":"failed","reference_id":"` + payout + `"}`))
	}))
	defer server.Close()
	s.Gateway.BaseURL = server.URL
	if err = s.SubmitRazorpayX(ctx, payout); err == nil {
		t.Fatal("uncertain payout accepted")
	}
	if err = s.Confirm(ctx, b, payout, "FAILED", "MANUAL-UNVERIFIED"); err == nil {
		t.Fatal("manual provider failure accepted")
	}
	if err = s.Initiate(ctx, a, payout, "Must not retry an uncertain payment"); err == nil {
		t.Fatal("uncertain payout generated new attempt")
	}
	if err = s.SubmitRazorpayX(ctx, payout); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("unexpected provider calls", calls)
	}
	if err = s.ReconcileRazorpayX(ctx, b, payout); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FINANCE_PAYOUT_PROVIDER", "BANK_RECONCILIATION")
	// Confirmed failure is idempotent; manual mode is used for the next attempt.
	if _, err = p.Exec(ctx, `UPDATE finance.payouts SET provider='BANK_RECONCILIATION' WHERE id=$1`, payout); err != nil {
		t.Fatal(err)
	}
	if err = s.Confirm(ctx, b, payout, "FAILED", "BANK-FAILURE-001"); err != nil {
		t.Fatal(err)
	}
	raw, err := s.Summary(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	var summary map[string]any
	_ = json.Unmarshal(raw, &summary)
	if summary["pending"] != float64(85000) {
		t.Fatal("failed payout removed liability", summary)
	}
	if err = s.Initiate(ctx, a, payout, "Retry confirmed failed bank instruction"); err != nil {
		t.Fatal(err)
	}
	if err = s.Confirm(ctx, b, payout, "SUCCEEDED", "UTR-CONFIRMED-001"); err != nil {
		t.Fatal(err)
	}
	if err = s.Confirm(ctx, b, payout, "SUCCEEDED", "UTR-CONFIRMED-001"); err != nil {
		t.Fatal(err)
	}
	raw, err = s.Summary(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(raw, &summary)
	if summary["pending"] != float64(0) || summary["paid"] != float64(85000) {
		t.Fatal("incorrect settlement", summary)
	}
	var count int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM finance.journals WHERE kind='PAYOUT'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate payout", count, err)
	}
}

func TestFinancePermissionsAndOwnerIsolation(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if err := devseed.Seed(ctx, p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{JWTSecret: "finance-isolation-secret-at-least-32-characters"}
	e := echo.New()
	Register(e.Group("/api/v1"), p, cfg, zap.NewNop())
	token, _ := auth.GenerateAccessToken(cfg.JWTSecret, devseed.CustomerID, "CUSTOMER")
	for _, path := range []string{"/api/v1/partner/finance/ledger", "/api/v1/admin/finance/balances"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != 403 {
			t.Fatal(path, rec.Code)
		}
	}
	token, _ = auth.GenerateAccessToken(cfg.JWTSecret, devseed.AdminID, "ADMIN")
	req := httptest.NewRequest("GET", "/api/v1/admin/finance/balances", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatal("ungranted admin allowed", rec.Code)
	}
	token, _ = auth.GenerateAccessToken(cfg.JWTSecret, devseed.OwnerID, "RESTAURANT_OWNER")
	req = httptest.NewRequest("GET", "/api/v1/partner/finance/bank", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "ciphertext") {
		t.Fatal(rec.Code, rec.Body.String())
	}
}

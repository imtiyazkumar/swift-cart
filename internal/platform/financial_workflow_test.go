package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func (f *workflow) captureForTest(t *testing.T, order string) string {
	t.Helper()
	ctx := context.Background()
	var intent, payment string
	if err := f.p.QueryRow(ctx, `UPDATE payment_intents SET provider_order_id='order_finance',status='PENDING' WHERE order_id=(SELECT id FROM orders WHERE uuid=$1) RETURNING uuid`, order).Scan(&intent); err != nil {
		t.Fatal(err)
	}
	c := f.e.NewContext(httptest.NewRequest("POST", "/", nil), httptest.NewRecorder())
	c.Set("identity", f.users["CUSTOMER"])
	tx, err := f.s.begin(c, "PAY")
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = f.s.capture(c, tx, providerPayment{ID: "pay_finance", OrderID: "order_finance", Amount: 100000, Currency: "INR", Status: "captured", Method: "upi"}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.p.QueryRow(ctx, `SELECT uuid FROM payments WHERE provider_payment_id='pay_finance'`).Scan(&payment); err != nil {
		t.Fatal(err)
	}
	return payment
}
func TestRefundIsProviderConfirmedBoundedAndIdempotent(t *testing.T) {
	f := newWorkflow(t)
	order := f.checkout(t, "RAZORPAY")
	payment := f.captureForTest(t, order)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && r.URL.Path == "/payments/pay_finance/refund" {
			calls++
			fmt.Fprint(w, `{"id":"rfnd_confirmed","payment_id":"pay_finance","amount":100000,"currency":"INR","status":"processed"}`)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()
	f.s.Gateway = &Gateway{BaseURL: server.URL, KeyID: "test-key", Secret: "test-secret", Client: server.Client()}
	body := `{"amount_paise":100000,"reason":"Cancelled before preparation","idempotency_key":"stable-refund-key"}`
	f.call(t, "ADMIN", "POST", "/admin/payments/"+payment+"/refunds", body, 201)
	f.call(t, "ADMIN", "POST", "/admin/payments/"+payment+"/refunds", body, 200)
	if calls != 1 {
		t.Fatal("refund retry called provider twice", calls)
	}
	f.call(t, "ADMIN", "POST", "/admin/payments/"+payment+"/refunds", `{"amount_paise":1,"reason":"Cannot over-refund","idempotency_key":"another-refund-key"}`, 400)
	var refunds, postings int
	var balance int64
	var status string
	if err := f.p.QueryRow(context.Background(), `SELECT count(*) FROM refunds WHERE status='COMPLETED'`).Scan(&refunds); err != nil || refunds != 1 {
		t.Fatal("refund not recorded exactly once", refunds, err)
	}
	if err := f.p.QueryRow(context.Background(), `SELECT count(*) FROM financial_transactions WHERE transaction_type='REFUND'`).Scan(&postings); err != nil || postings != 2 {
		t.Fatal("refund postings duplicated", postings, err)
	}
	if err := f.p.QueryRow(context.Background(), `SELECT COALESCE(sum(CASE direction WHEN 'DEBIT' THEN amount_paise ELSE -amount_paise END),0)::bigint FROM financial_transactions WHERE account_code='YEMBER_BANK'`).Scan(&balance); err != nil || balance != 0 {
		t.Fatal("bank balance changed unexpectedly", balance, err)
	}
	if err := f.p.QueryRow(context.Background(), `SELECT status FROM orders WHERE uuid=$1`, order).Scan(&status); err != nil || status != "REFUNDED" {
		t.Fatal("full refund did not update order", status, err)
	}
}
func TestBankPayoutAllocatesEarningsAndRequiresIndependentConfirmation(t *testing.T) {
	t.Setenv("RESTAURANT_COMMISSION_BPS", "1000")
	keys, _ := json.Marshal(map[string]string{"test": base64.StdEncoding.EncodeToString(make([]byte, 32))})
	t.Setenv("FINANCE_BANK_KEYS", string(keys))
	t.Setenv("FINANCE_BANK_ACTIVE_KEY", "test")
	f := newWorkflow(t)
	order := f.checkout(t, "RAZORPAY")
	f.captureForTest(t, order)
	ctx := context.Background()
	if _, err := f.p.Exec(ctx, `UPDATE orders SET status='DELIVERED' WHERE uuid=$1`, order); err != nil {
		t.Fatal(err)
	}
	c := f.e.NewContext(httptest.NewRequest("POST", "/", nil), httptest.NewRecorder())
	c.Set("identity", f.users["ADMIN"])
	tx, err := f.s.begin(c, "PAY")
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var internal int64
	if err = tx.QueryRow(ctx, `SELECT id FROM orders WHERE uuid=$1`, order).Scan(&internal); err != nil {
		t.Fatal(err)
	}
	if err = f.s.recognize(c, tx, internal); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	f.call(t, "RESTAURANT_OWNER", "PUT", "/partner/finance/bank", `{"holder":"Restaurant owner","account_number":"123456789012","routing_code":"SBIN0001234"}`, 204)
	var bank, earning string
	if err = f.p.QueryRow(ctx, `SELECT uuid FROM restaurant_bank_accounts WHERE deleted_at IS NULL`).Scan(&bank); err != nil {
		t.Fatal(err)
	}
	if err = f.p.QueryRow(ctx, `SELECT uuid FROM restaurant_earnings`).Scan(&earning); err != nil {
		t.Fatal(err)
	}
	f.call(t, "ADMIN", "POST", "/admin/manage/restaurant-bank-accounts/"+bank+"/verify-bank", `{"status":"VERIFIED","reason":"Verified original bank evidence"}`, 204)
	payout := f.call(t, "ADMIN", "POST", "/admin/payouts/restaurant", fmt.Sprintf(`{"bank_id":%q,"earning_ids":[%q],"reason":"Pay eligible restaurant earnings"}`, bank, earning), 201)["id"].(string)
	body := `{"status":"PAID","provider_transfer_id":"UTR-CONFIRMED-1234","reason":"Verified bank statement transfer"}`
	f.call(t, "ADMIN", "POST", "/admin/payouts/restaurant/"+payout+"/confirm", body, 400)
	u := &identity{Role: "ADMIN"}
	if err = f.p.QueryRow(ctx, `INSERT INTO users(full_name,email,role,status) VALUES('Independent admin','independent@test.invalid','ADMIN','ACTIVE') RETURNING id,uuid`).Scan(&u.ID, &u.UUID); err != nil {
		t.Fatal(err)
	}
	f.users["SECOND_ADMIN"] = u
	f.token["SECOND_ADMIN"], _ = f.s.access(u)
	for i := 0; i < 2; i++ {
		f.call(t, "SECOND_ADMIN", "POST", "/admin/payouts/restaurant/"+payout+"/confirm", body, 204)
	}
	for account, want := range map[string]int64{"YEMBER_BANK": 10000, "RESTAURANT_PAYABLE": 0} {
		var got int64
		if err = f.p.QueryRow(ctx, `SELECT COALESCE(sum(CASE direction WHEN 'DEBIT' THEN amount_paise ELSE -amount_paise END),0)::bigint FROM financial_transactions WHERE account_code=$1`, account).Scan(&got); err != nil || got != want {
			t.Fatalf("%s got %d want %d: %v", account, got, want, err)
		}
	}
}
func TestUnknownProviderOrderIsNeverAutomaticallyCreatedTwice(t *testing.T) {
	f := newWorkflow(t)
	order := f.checkout(t, "RAZORPAY")
	var intent string
	if err := f.p.QueryRow(context.Background(), `SELECT uuid FROM payment_intents WHERE order_id=(SELECT id FROM orders WHERE uuid=$1)`, order).Scan(&intent); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(503) }))
	defer server.Close()
	f.s.Gateway = &Gateway{BaseURL: server.URL, KeyID: "test-key", Secret: "test-secret", Client: server.Client()}
	f.call(t, "CUSTOMER", "POST", "/payments/intents/"+intent+"/order", "{}", 503)
	f.call(t, "CUSTOMER", "POST", "/payments/intents/"+intent+"/order", "{}", 400)
	if calls != 1 {
		t.Fatal("uncertain provider request was replayed", calls)
	}
}
func TestEveryAdminWorkspaceUsesOnlyContractTables(t *testing.T) {
	f := newWorkflow(t)
	for name := range tables {
		resource := strings.ReplaceAll(name, "_", "-")
		f.call(t, "ADMIN", "GET", "/admin/manage/"+resource+"?limit=1", "", 200)
	}
	f.call(t, "CUSTOMER", "GET", "/admin/contract", "", 403)
}

func TestAdminOnlyCouponLifecycleRetainsUsage(t *testing.T) {
	f := newWorkflow(t)
	ctx := context.Background()
	create := `{"code":"SAVE100","discount_type":"FLAT","discount_value":10000,"is_active":true,"usage_limit":1,"reason":"Launch bounded promotion"}`
	f.call(t, "CUSTOMER", "POST", "/admin/manage/coupons", create, 403)
	uid := f.call(t, "ADMIN", "POST", "/admin/manage/coupons", create, 200)["id"].(string)
	f.call(t, "CUSTOMER", "POST", "/customer/cart/items", fmt.Sprintf(`{"menu_item_id":%q,"address_id":%q,"quantity":1}`, f.item, f.address), 204)
	f.call(t, "CUSTOMER", "POST", "/customer/checkout", fmt.Sprintf(`{"address_id":%q,"idempotency_key":"coupon-checkout-key","payment_method":"PAY_ON_DELIVERY","coupon_code":"SAVE100"}`, f.address), 201)
	f.call(t, "ADMIN", "PUT", "/admin/manage/coupons/"+uid, `{"is_active":false,"reason":"Pause promotion"}`, 200)
	var code string
	var discount int64
	var uses int
	if err := f.p.QueryRow(ctx, `SELECT c.code,count(u.id),max(u.discount_amount_paise) FROM coupons c JOIN coupon_usages u ON u.coupon_id=c.id WHERE c.uuid=$1 GROUP BY c.code`, uid).Scan(&code, &uses, &discount); err != nil || code != "SAVE100" || uses != 1 || discount != 10000 {
		t.Fatal("activation changed coupon code or usage", code, uses, discount, err)
	}
	f.call(t, "ADMIN", "DELETE", "/admin/manage/coupons/"+uid, `{"reason":"Retire promotion while keeping redemptions"}`, 204)
	if err := f.p.QueryRow(ctx, `SELECT count(*) FROM coupon_usages`).Scan(&uses); err != nil || uses != 1 {
		t.Fatal("coupon deletion removed redemption history", uses, err)
	}
	if _, err := f.p.Exec(ctx, `DELETE FROM coupon_usages`); err == nil {
		t.Fatal("redemption history could be erased")
	}
}

func TestCashDifferenceRetainsActualReceiptAndHoldsSettlement(t *testing.T) {
	t.Setenv("DELIVERY_FEE_PAISE", "0")
	f := newWorkflow(t)
	order := f.checkout(t, "PAY_ON_DELIVERY")
	for _, status := range []string{"ACCEPTED", "PREPARING", "READY_FOR_PICKUP"} {
		f.call(t, "RESTAURANT_OWNER", "PATCH", "/restaurant/orders/"+order+"/status", fmt.Sprintf(`{"status":%q}`, status), 204)
	}
	for _, status := range []string{"ACCEPTED", "PICKED_UP"} {
		f.call(t, "DELIVERY_PARTNER", "PATCH", "/rider/deliveries/"+order+"/status", fmt.Sprintf(`{"status":%q}`, status), 204)
	}
	f.call(t, "DELIVERY_PARTNER", "PATCH", "/rider/deliveries/"+order+"/status", `{"status":"DELIVERED","cash_collected":true,"amount_collected_paise":90000}`, 204)
	f.call(t, "DELIVERY_PARTNER", "POST", "/partner/finance/cash-handovers/"+order, `{"amount_paise":90000}`, 204)
	f.call(t, "RESTAURANT_OWNER", "POST", "/partner/finance/cash-handovers/"+order+"/confirm", `{"amount_paise":90000,"accept":true}`, 400)
	var correct bool
	if err := f.p.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM cash_collections WHERE amount_due_paise=100000 AND amount_collected_paise=90000 AND status='PENDING_REVIEW') AND EXISTS(SELECT 1 FROM reconciliation_runs WHERE difference_paise=-10000 AND status='EXCEPTIONS') AND NOT EXISTS(SELECT 1 FROM restaurant_earnings) AND EXISTS(SELECT 1 FROM restaurant_payouts WHERE amount_paise=90000 AND status='PENDING_REVIEW')`).Scan(&correct); err != nil || !correct {
		t.Fatal("cash discrepancy was lost or settled", correct, err)
	}
}

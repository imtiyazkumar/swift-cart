package platform

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/imtiyazkumar/yember-core/internal/contractschema"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

func testSignature(secret, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

type workflow struct {
	p                                         *pgxpool.Pool
	s                                         *Service
	e                                         *echo.Echo
	users                                     map[string]*identity
	token                                     map[string]string
	restaurant, rider, item, variant, address string
}

func newWorkflow(t *testing.T) *workflow {
	t.Helper()
	p := testdb.Empty(t)
	ctx := context.Background()
	if err := contractschema.Install(ctx, p); err != nil {
		t.Fatal(err)
	}
	f := &workflow{p: p, e: echo.New(), users: map[string]*identity{}, token: map[string]string{}}
	defaultErrorHandler := f.e.HTTPErrorHandler
	f.e.HTTPErrorHandler = func(err error, c echo.Context) { t.Logf("request error: %v", err); defaultErrorHandler(err, c) }
	f.s = &Service{DB: p, JWTSecret: "a-test-secret-with-more-than-thirty-two-characters"}
	for i, role := range []string{"ADMIN", "CUSTOMER", "RESTAURANT_OWNER", "DELIVERY_PARTNER"} {
		u := &identity{Role: role}
		if err := p.QueryRow(ctx, `INSERT INTO users(full_name,email,phone,role,status) VALUES($1,$2,$3,$4,'ACTIVE') RETURNING id,uuid`, role, strings.ToLower(role)+"@test.invalid", fmt.Sprintf("900000000%d", i), role).Scan(&u.ID, &u.UUID); err != nil {
			t.Fatal(err)
		}
		f.users[role] = u
		f.token[role], _ = f.s.access(u)
	}
	var station, restaurant, rider, category, item, address int64
	if err := p.QueryRow(ctx, `INSERT INTO stations(name,latitude,longitude,radius_km,status) VALUES('Station',34,74,5,'ACTIVE') RETURNING id`).Scan(&station); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO restaurants(owner_user_id,station_id,name,address,latitude,longitude,status,is_open) VALUES($1,$2,'Restaurant','Pickup',34,74,'APPROVED',true) RETURNING id,uuid`, f.users["RESTAURANT_OWNER"].ID, station).Scan(&restaurant, &f.restaurant); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO riders(user_id,station_id,verification_status,availability_status) VALUES($1,$2,'APPROVED','ONLINE') RETURNING id,uuid`, f.users["DELIVERY_PARTNER"].ID, station).Scan(&rider, &f.rider); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO registration_fees(restaurant_id,amount_paise,currency,status) VALUES($1,20000,'INR','PAID');`, restaurant); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO registration_fees(delivery_agent_id,amount_paise,currency,status) VALUES($1,15000,'INR','PAID');`, rider); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO menu_categories(name,is_active) VALUES('Meals',true) RETURNING id`).Scan(&category); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO menu_items(restaurant_id,category_id,name,base_price,is_available) VALUES($1,$2,'Meal',1000,true) RETURNING id,uuid`, restaurant, category).Scan(&item, &f.item); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO user_addresses(user_id,label,address_line1,city,latitude,longitude) VALUES($1,'Home','Delivery','Srinagar',34,74) RETURNING id,uuid`, f.users["CUSTOMER"].ID).Scan(&address, &f.address); err != nil {
		t.Fatal(err)
	}
	f.s.Register(f.e)
	return f
}
func (f *workflow) call(t *testing.T, role, method, path, body string, want int) map[string]any {
	t.Helper()
	r := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if role != "" {
		r.Header.Set("Authorization", "Bearer "+f.token[role])
	}
	w := httptest.NewRecorder()
	f.e.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body)
	}
	result := map[string]any{}
	if len(w.Body.Bytes()) > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &result)
	}
	return result
}
func (f *workflow) checkout(t *testing.T, method string) string {
	f.call(t, "CUSTOMER", "POST", "/customer/cart/items", fmt.Sprintf(`{"menu_item_id":%q,"address_id":%q,"quantity":1}`, f.item, f.address), 204)
	out := f.call(t, "CUSTOMER", "POST", "/customer/checkout", fmt.Sprintf(`{"address_id":%q,"idempotency_key":"workflow-checkout","payment_method":%q}`, f.address, method), 201)
	return out["id"].(string)
}
func TestDocumentSchemaCashDeliveryAndGrossHandover(t *testing.T) {
	t.Setenv("RESTAURANT_COMMISSION_BPS", "1000")
	t.Setenv("DELIVERY_FEE_PAISE", "4000")
	f := newWorkflow(t)
	order := f.checkout(t, "PAY_ON_DELIVERY")
	for _, status := range []string{"ACCEPTED", "PREPARING", "READY_FOR_PICKUP"} {
		f.call(t, "RESTAURANT_OWNER", "PATCH", "/restaurant/orders/"+order+"/status", fmt.Sprintf(`{"status":%q}`, status), 204)
	}
	f.call(t, "CUSTOMER", "PATCH", "/restaurant/orders/"+order+"/status", `{"status":"ACCEPTED"}`, 403)
	for _, status := range []string{"ACCEPTED", "PICKED_UP"} {
		f.call(t, "DELIVERY_PARTNER", "PATCH", "/rider/deliveries/"+order+"/status", fmt.Sprintf(`{"status":%q}`, status), 204)
	}
	f.call(t, "DELIVERY_PARTNER", "PATCH", "/rider/deliveries/"+order+"/status", `{"status":"DELIVERED","cash_collected":true,"amount_collected_paise":104000}`, 204)
	f.call(t, "DELIVERY_PARTNER", "POST", "/partner/finance/cash-handovers/"+order, `{"amount_paise":104000}`, 204)
	for i := 0; i < 2; i++ {
		f.call(t, "RESTAURANT_OWNER", "POST", "/partner/finance/cash-handovers/"+order+"/confirm", `{"amount_paise":104000,"accept":true}`, 204)
	}
	for account, want := range map[string]int64{"YEMBER_BANK": 0, "CASH_IN_TRANSIT": 0, "RESTAURANT_PAYABLE": 0, "RESTAURANT_FEE_RECEIVABLE": 14000, "AGENT_PAYABLE": -4000} {
		var got int64
		if err := f.p.QueryRow(context.Background(), `SELECT COALESCE(sum(CASE direction WHEN 'DEBIT' THEN amount_paise ELSE -amount_paise END),0)::bigint FROM financial_transactions WHERE account_code=$1`, account).Scan(&got); err != nil || got != want {
			t.Fatalf("%s balance=%d want=%d: %v", account, got, want, err)
		}
	}
	if err := contractschema.Validate(context.Background(), f.p); err != nil {
		t.Fatal(err)
	}
}
func TestRazorpayOrdersCaptureAndDuplicateVerification(t *testing.T) {
	f := newWorkflow(t)
	order := f.checkout(t, "RAZORPAY")
	var uid string
	if err := f.p.QueryRow(context.Background(), `SELECT i.uuid FROM payment_intents i JOIN orders o ON o.id=i.order_id WHERE o.uuid=$1`, order).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && r.URL.Path == "/orders" {
			calls++
			fmt.Fprint(w, `{"id":"order_verified","amount":100000,"currency":"INR"}`)
			return
		}
		if r.Method == "GET" && r.URL.Path == "/payments/pay_verified" {
			fmt.Fprint(w, `{"id":"pay_verified","order_id":"order_verified","amount":100000,"currency":"INR","status":"captured","method":"upi"}`)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()
	f.s.Gateway = &Gateway{BaseURL: server.URL, KeyID: "rzp_test_key", Secret: "server-secret", Client: server.Client()}
	for i := 0; i < 2; i++ {
		f.call(t, "CUSTOMER", "POST", "/payments/intents/"+uid+"/order", "{}", 200)
	}
	macSignature := testSignature("server-secret", "order_verified|pay_verified")
	for i := 0; i < 2; i++ {
		f.call(t, "CUSTOMER", "POST", "/payments/intents/"+uid+"/verify", fmt.Sprintf(`{"razorpay_payment_id":"pay_verified","razorpay_signature":%q}`, macSignature), 204)
	}
	if calls != 1 {
		t.Fatal("retry created another provider order", calls)
	}
	var count int
	var amount int64
	var status string
	if err := f.p.QueryRow(context.Background(), `SELECT count(*),sum(amount_paise)::bigint FROM payments`).Scan(&count, &amount); err != nil || count != 1 || amount != 100000 {
		t.Fatal("capture duplicated or changed", count, amount, err)
	}
	if err := f.p.QueryRow(context.Background(), `SELECT status FROM orders WHERE uuid=$1`, order).Scan(&status); err != nil || status != "CONFIRMED" {
		t.Fatal("verified checkout did not confirm order", status, err)
	}
	if err := contractschema.Validate(context.Background(), f.p); err != nil {
		t.Fatal(err)
	}
}

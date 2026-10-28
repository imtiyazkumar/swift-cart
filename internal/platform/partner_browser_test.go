package platform

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/crypto/bcrypt"
)

// Opt-in browser suite: the real API runs against a disposable database, never
// the developer's seed data or an external payment service.
func TestRestaurantBrowser(t *testing.T) {
	if os.Getenv("YEMBER_RESTAURANT_E2E") != "1" {
		t.Skip("set YEMBER_INTEGRATION=1 and YEMBER_RESTAURANT_E2E=1 to run restaurant Playwright tests")
	}
	f := newBrowserWorkflow(t)
	ctx := context.Background()
	// More than one page, including an exact 25-record boundary after filtering.
	for i := 1; i <= 30; i++ {
		f.call(t, "CUSTOMER", "POST", "/customer/cart/items", fmt.Sprintf(`{"menu_item_id":%q,"address_id":%q,"quantity":2}`, f.item, f.address), 204)
		out := f.call(t, "CUSTOMER", "POST", "/customer/checkout", fmt.Sprintf(`{"address_id":%q,"idempotency_key":"browser-history-%02d","payment_method":"PAY_ON_DELIVERY"}`, f.address, i), 201)
		f.call(t, "RESTAURANT_OWNER", "PATCH", "/restaurant/orders/"+out["id"].(string)+"/status", `{"status":"REJECTED","reason":"Synthetic browser history fixture"}`, 204)
	}
	if _, err := f.p.Exec(ctx, `UPDATE orders SET order_number='BROWSER-BOUNDARY-'||id WHERE id<=25`); err != nil {
		t.Fatal(err)
	}
	runPartnerBrowser(t, f, "restaurant", 8085)
}

func newBrowserWorkflow(t *testing.T) *workflow {
	t.Helper()
	f := newWorkflow(t)
	t.Setenv("DOCUMENT_LOCAL_DIR", t.TempDir())
	t.Setenv("FINANCE_BANK_ACTIVE_KEY", "browser")
	t.Setenv("FINANCE_BANK_KEYS", `{"browser":"`+base64.StdEncoding.EncodeToString(make([]byte, 32))+`"}`)
	ctx := context.Background()
	hash, err := bcrypt.GenerateFromPassword([]byte("RestaurantE2E!2026"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.p.Exec(ctx, `UPDATE users SET password_hash=$1`, string(hash)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.p.Exec(ctx, `UPDATE menu_items SET name='Saffron rice',image_url='https://images.pexels.com/photos/1640777/pexels-photo-1640777.jpeg?auto=compress&cs=tinysrgb&w=640',preparation_minutes=20; INSERT INTO item_variants(menu_item_id,name,price_delta,is_available) SELECT id,'Family portion',250,true FROM menu_items`); err != nil {
		t.Fatal(err)
	}
	return f
}

func runPartnerBrowser(t *testing.T, f *workflow, kind string, port int) {
	t.Helper()
	f.e.Use(middleware.CORSWithConfig(middleware.CORSConfig{AllowOrigins: []string{fmt.Sprintf("http://localhost:%d", port)}}))
	server := httptest.NewServer(f.e)
	defer server.Close()
	_, source, _, _ := runtime.Caller(0)
	appRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../yember-app"))
	command := exec.Command("npm", "run", "test:e2e:"+kind)
	if runtime.GOOS == "windows" {
		command = exec.Command("cmd.exe", "/c", "npm.cmd run test:e2e:"+kind)
	}
	command.Dir = appRoot
	command.Env = append(os.Environ(), "YEMBER_E2E_APP="+kind, "YEMBER_E2E_API_URL="+server.URL+"/api/v1", "EXPO_PUBLIC_API_URL="+server.URL+"/api/v1", "EXPO_NO_DOTENV=1", "YEMBER_E2E_PASSWORD=RestaurantE2E!2026")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		t.Fatal(kind+" browser suite:", err)
	}
}

func TestDeliveryBrowser(t *testing.T) {
	if os.Getenv("YEMBER_DELIVERY_E2E") != "1" {
		t.Skip("set YEMBER_INTEGRATION=1 and YEMBER_DELIVERY_E2E=1 to run delivery Playwright tests")
	}
	f := newBrowserWorkflow(t)
	t.Setenv("DELIVERY_FEE_PAISE", "4000")
	// History fixtures are deliberately non-financial; cash tests create fresh
	// orders and exercise collection/recognition through the actual endpoints.
	_, err := f.p.Exec(context.Background(), `INSERT INTO orders(order_number,user_id,restaurant_id,address_id,status,subtotal,delivery_fee,tax_amount,discount_amount,total_amount,currency,placed_at)
 SELECT 'DELIVERY-HISTORY-'||n,u.id,r.id,a.id,'DELIVERED',1000,40,0,0,1040,'INR',now()-n*interval '1 hour'
 FROM generate_series(1,30)n CROSS JOIN users u CROSS JOIN restaurants r CROSS JOIN user_addresses a WHERE u.uuid=$1 AND r.uuid=$2 AND a.uuid=$3;
 `, f.users["CUSTOMER"].UUID, f.restaurant, f.address)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.p.Exec(context.Background(), `WITH assignments AS (INSERT INTO delivery_assignments(order_id,rider_id,status,assigned_at,accepted_at,picked_up_at,delivered_at)
 SELECT o.id,r.id,'DELIVERED',o.created_at,o.created_at,o.created_at,o.created_at FROM orders o CROSS JOIN riders r WHERE r.uuid=$1
 RETURNING order_id) INSERT INTO order_items(order_id,menu_item_id,quantity,unit_price) SELECT a.order_id,i.id,1,1000 FROM assignments a CROSS JOIN menu_items i WHERE i.uuid=$2`, f.rider, f.item)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DELIVERY_FEE_PAISE", "0")
	prepaid := f.checkout(t, "RAZORPAY")
	f.captureForTest(t, prepaid)
	if _, err = f.p.Exec(context.Background(), `UPDATE orders SET order_number='BROWSER-PREPAID' WHERE uuid=$1`, prepaid); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DELIVERY_FEE_PAISE", "4000")
	runPartnerBrowser(t, f, "delivery", 8086)
}

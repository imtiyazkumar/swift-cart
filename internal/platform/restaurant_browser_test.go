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
	// More than one page, including an exact 25-record boundary after filtering.
	for i := 1; i <= 30; i++ {
		f.call(t, "CUSTOMER", "POST", "/customer/cart/items", fmt.Sprintf(`{"menu_item_id":%q,"address_id":%q,"quantity":2}`, f.item, f.address), 204)
		out := f.call(t, "CUSTOMER", "POST", "/customer/checkout", fmt.Sprintf(`{"address_id":%q,"idempotency_key":"browser-history-%02d","payment_method":"PAY_ON_DELIVERY"}`, f.address, i), 201)
		f.call(t, "RESTAURANT_OWNER", "PATCH", "/restaurant/orders/"+out["id"].(string)+"/status", `{"status":"REJECTED","reason":"Synthetic browser history fixture"}`, 204)
	}
	if _, err = f.p.Exec(ctx, `UPDATE orders SET order_number='BROWSER-BOUNDARY-'||id WHERE id<=25`); err != nil {
		t.Fatal(err)
	}
	f.e.Use(middleware.CORSWithConfig(middleware.CORSConfig{AllowOrigins: []string{"http://localhost:8085"}}))
	server := httptest.NewServer(f.e)
	defer server.Close()
	_, source, _, _ := runtime.Caller(0)
	appRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../yember-app"))
	command := exec.Command("npm", "run", "test:e2e:restaurant")
	if runtime.GOOS == "windows" {
		command = exec.Command("cmd.exe", "/c", "npm.cmd run test:e2e:restaurant")
	}
	command.Dir = appRoot
	command.Env = append(os.Environ(), "YEMBER_E2E_API_URL="+server.URL+"/api/v1", "EXPO_PUBLIC_API_URL="+server.URL+"/api/v1", "EXPO_NO_DOTENV=1", "YEMBER_E2E_PASSWORD=RestaurantE2E!2026")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err = command.Run(); err != nil {
		t.Fatal("restaurant browser suite:", err)
	}
}

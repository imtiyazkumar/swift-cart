package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/imtiyazkumar/yember-core/internal/contractschema"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"github.com/labstack/echo/v4"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizedVariantCheckoutAndAddressHistory(t *testing.T) {
	p := testdb.Empty(t)
	ctx := context.Background()
	if _, err := p.Exec(ctx, contractschema.SQL); err != nil {
		t.Fatal(err)
	}
	var owner, customer, station, restaurant, category, item, variant, address int64
	var userUUID, itemUUID, variantUUID, addressUUID string
	for _, u := range []struct {
		role string
		id   *int64
		uuid *string
	}{{"RESTAURANT_OWNER", &owner, new(string)}, {"CUSTOMER", &customer, &userUUID}} {
		if err := p.QueryRow(ctx, `INSERT INTO users(full_name,role,status) VALUES($1,$1,'ACTIVE') RETURNING id,uuid`, u.role).Scan(u.id, u.uuid); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.QueryRow(ctx, `INSERT INTO stations(name,latitude,longitude,radius_km,status,created_by) VALUES('Central',34,74,5,'ACTIVE',$1) RETURNING id`, owner).Scan(&station); err != nil {
		t.Fatal(err)
	}
	var outside bool
	if err := p.QueryRow(ctx, `SELECT station_covers($1,35,75)`, station).Scan(&outside); err != nil || outside {
		t.Fatal("station coverage accepts distant coordinates", err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO restaurants(owner_user_id,station_id,name,address,latitude,longitude,status,is_open) VALUES($1,$2,'Kitchen','Pickup',34,74,'APPROVED',true) RETURNING id`, owner, station).Scan(&restaurant); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO menu_categories(name,sort_order,is_active) VALUES('Meals',0,true) RETURNING id`).Scan(&category); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO menu_items(restaurant_id,category_id,name,base_price,is_available) VALUES($1,$2,'Meal',100,true) RETURNING id,uuid`, restaurant, category).Scan(&item, &itemUUID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO item_variants(menu_item_id,name,price_delta,is_available) VALUES($1,'Large',25,true) RETURNING id,uuid`, item).Scan(&variant, &variantUUID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO user_addresses(user_id,label,contact_name,address_line1,city,latitude,longitude,is_default) VALUES($1,'Home','Customer','Dropoff','City',34,74,true) RETURNING id,uuid`, customer).Scan(&address, &addressUUID); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	s := &Service{DB: p, JWTSecret: "a-test-secret-with-more-than-thirty-two-characters"}
	s.Register(e)
	token, err := s.access(&identity{ID: customer, UUID: userUUID, Role: "CUSTOMER"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/v1/customer"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		e.ServeHTTP(w, r)
		return w
	}
	line := call("/cart/items", fmt.Sprintf(`{"menu_item_id":%q,"variant_id":%q,"address_id":%q,"quantity":2}`, itemUUID, variantUUID, addressUUID))
	if line.Code != 204 {
		t.Fatal(line.Code, line.Body)
	}
	body := fmt.Sprintf(`{"address_id":%q,"idempotency_key":"numeric-checkout-key","payment_method":"PAY_ON_DELIVERY"}`, addressUUID)
	response := call("/checkout", body)
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body)
	}
	var order struct {
		UUID  string `json:"id"`
		Total int64  `json:"subtotal"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &order); err != nil || order.Total != 25000 {
		t.Fatal("variant delta was not priced authoritatively", response.Body, err)
	}
	replay := call("/checkout", body)
	if replay.Code != 200 || !strings.Contains(replay.Body.String(), order.UUID) {
		t.Fatal("checkout retry created a duplicate", replay.Code, replay.Body)
	}
	var internalOrder, storedItem, storedVariant int64
	var unit string
	if err = p.QueryRow(ctx, `SELECT o.id,i.menu_item_id,i.variant_id,i.unit_price::text FROM orders o JOIN order_items i ON i.order_id=o.id WHERE o.uuid=$1`, order.UUID).Scan(&internalOrder, &storedItem, &storedVariant, &unit); err != nil || storedItem != item || storedVariant != variant || unit != "125.00" {
		t.Fatal("normalized checkout references/charged price incorrect", err)
	}
	if _, err = p.Exec(ctx, `UPDATE user_addresses SET address_line1='Changed delivery address' WHERE id=$1`, address); err == nil {
		t.Fatal("historical address mutated")
	}
	if _, err = p.Exec(ctx, `UPDATE menu_items SET name='Renamed meal',base_price=200 WHERE id=$1`, item); err != nil {
		t.Fatal(err)
	}
	if err = p.QueryRow(ctx, `SELECT unit_price::text FROM order_items WHERE order_id=$1`, internalOrder).Scan(&unit); err != nil || unit != "125.00" {
		t.Fatal("charged price changed with menu", err)
	}
	var history int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM order_status_history WHERE order_id=$1`, internalOrder).Scan(&history); err != nil || history != 1 {
		t.Fatal("initial status history missing", err)
	}
}

package stations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/imtiyazkumar/yember-core/config"
	"github.com/imtiyazkumar/yember-core/internal/auth"
	"github.com/imtiyazkumar/yember-core/internal/checkout"
	"github.com/imtiyazkumar/yember-core/internal/customer"
	"github.com/imtiyazkumar/yember-core/internal/devseed"
	"github.com/imtiyazkumar/yember-core/internal/dispatch"
	"github.com/imtiyazkumar/yember-core/internal/onboarding"
	"github.com/imtiyazkumar/yember-core/internal/rider"
	"github.com/imtiyazkumar/yember-core/internal/stations"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func ptr(v float64) *float64 { return &v }
func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}
func seed(t *testing.T) *pgxpool.Pool {
	t.Helper()
	p := testdb.New(t)
	if err := devseed.Seed(context.Background(), p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	return p
}

const stationA = "10000000-0000-4000-a000-000000000080"
const stationB = "20000000-0000-4000-a000-000000000080"

func TestCoordinates(t *testing.T) {
	for _, c := range [][2]*float64{{nil, ptr(0)}, {ptr(0), nil}, {ptr(91), ptr(0)}, {ptr(0), ptr(-181)}, {ptr(math.NaN()), ptr(0)}, {ptr(0), ptr(math.Inf(1))}} {
		if stations.Coordinates(c[0], c[1]) == nil {
			t.Fatal("invalid coordinates accepted")
		}
	}
	if err := stations.Coordinates(ptr(0), ptr(0)); err != nil {
		t.Fatal("zero coordinates are valid", err)
	}
}
func TestResolutionNearestTieBoundaryAndInactive(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if _, err := stations.Resolve(ctx, p, ptr(34), ptr(74)); err == nil {
		t.Fatal("no coverage must fail closed")
	}
	exec(t, p, `INSERT INTO public.stations(id,name,latitude,longitude,radius_m) VALUES($1,'First',34,74,5000),($2,'Second',34,74,5000)`, stationA, stationB)
	for i := 0; i < 5; i++ {
		id, err := stations.Resolve(ctx, p, ptr(34), ptr(74))
		if err != nil || id != stationA {
			t.Fatal("tie was not resolved by stable ID", id, err)
		}
	}
	exec(t, p, `UPDATE public.stations SET latitude=34.02 WHERE id=$1`, stationB)
	id, err := stations.Resolve(ctx, p, ptr(34.02), ptr(74))
	if err != nil || id != stationB {
		t.Fatal("nearest covering station was not selected", id, err)
	}
	exec(t, p, `UPDATE public.stations SET active=false WHERE id=$1`, stationB)
	id, err = stations.Resolve(ctx, p, ptr(34.02), ptr(74))
	if err != nil || id != stationA {
		t.Fatal("inactive station was selected", err)
	}
	exec(t, p, `UPDATE public.stations SET radius_m=public.station_distance_m(34,74,34.03,74) WHERE id=$1`, stationA)
	if _, err = stations.Resolve(ctx, p, ptr(34.03), ptr(74)); err != nil {
		t.Fatal("exact boundary should be covered", err)
	}
	if _, err = stations.Resolve(ctx, p, ptr(34.031), ptr(74)); err == nil {
		t.Fatal("outside boundary was covered")
	}
}

func TestRegistrationRequiresCoverageAndIgnoresChosenRestaurantStation(t *testing.T) {
	p := seed(t)
	ctx := context.Background()
	exec(t, p, `INSERT INTO public.stations(id,name,latitude,longitude,radius_m) VALUES($1,'Nearer',34.02,74,5000)`, stationB)
	var owner, doc string
	if err := p.QueryRow(ctx, `INSERT INTO public.users(email,password_hash,role,email_verified_at) VALUES('newowner@test.invalid','hash','MERCHANT_OWNER',now()) RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO public.private_documents(id,user_id,document_type,content_type,byte_size,backend) VALUES(gen_random_uuid(),$1,'LICENSE','application/pdf',10,'local') RETURNING id`, owner).Scan(&doc); err != nil {
		t.Fatal(err)
	}
	app := onboarding.Application{Name: "Station Kitchen", Phone: "1234567890", Address: "Test address", Description: "Test kitchen", LicenseNumber: "license", Cuisine: []string{"Test"}, DocumentIDs: []string{doc}, Latitude: ptr(0), Longitude: ptr(0), StationID: stationA}
	if _, err := onboarding.New(p).Submit(ctx, owner, "restaurant", app); err == nil || !strings.Contains(err.Error(), "does not serve") {
		t.Fatal("outside registration was not rejected clearly", err)
	}
	app.Latitude = nil
	if _, err := onboarding.New(p).Submit(ctx, owner, "restaurant", app); err == nil {
		t.Fatal("missing coordinates accepted")
	}
	app.Latitude = ptr(34.02)
	app.Longitude = ptr(74)
	id, err := onboarding.New(p).Submit(ctx, owner, "restaurant", app)
	if err != nil {
		t.Fatal(err)
	}
	var assigned string
	if err = p.QueryRow(ctx, `SELECT station_id FROM public.merchants WHERE id=$1`, id).Scan(&assigned); err != nil || assigned != stationB {
		t.Fatal("restaurant was not assigned to nearest station", err)
	}
	var recorded string
	if err = p.QueryRow(ctx, `SELECT data->>'station_id' FROM public.partner_applications WHERE resource_id=$1`, id).Scan(&recorded); err != nil || recorded != stationB {
		t.Fatal("application retained an untrusted station selection", err)
	}
	exec(t, p, `UPDATE public.stations SET active=false WHERE id=$1`, stationB)
	if _, err = p.Exec(ctx, `UPDATE public.merchants SET approval_status='APPROVED' WHERE id=$1`, id); err == nil {
		t.Fatal("approval did not revalidate station status")
	}
	if _, err = p.Exec(ctx, `UPDATE public.merchants SET station_id=$2 WHERE id=$1`, id, stationA); err == nil {
		t.Fatal("station changed without approval")
	}
}

func TestAddressDiscoveryCartAndCheckoutIsolation(t *testing.T) {
	p := seed(t)
	ctx := context.Background()
	t.Setenv("CUSTOMER_PAYMENT_METHOD", "CASH_ON_DELIVERY")
	exec(t, p, `INSERT INTO public.stations(id,name,latitude,longitude,radius_m) VALUES($1,'Overlap',34.02,74,5000)`, stationB)
	service := checkout.New(p)
	a, err := service.AddAddress(ctx, devseed.CustomerID, "A", "Street A", "", "City", "State", "123456", ptr(34), ptr(74))
	if err != nil {
		t.Fatal(err)
	}
	b, err := service.AddAddress(ctx, devseed.CustomerID, "B", "Street B", "", "City", "State", "123456", ptr(34.02), ptr(74))
	if err != nil {
		t.Fatal(err)
	}
	outside, err := service.AddAddress(ctx, devseed.CustomerID, "Outside", "Street C", "", "City", "State", "123456", ptr(0), ptr(0))
	if err != nil {
		t.Fatal(err)
	}
	item := "10000000-0000-4000-a000-000000000041"
	if err = service.Add(ctx, devseed.CustomerID, item, 1, a); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Checkout(ctx, devseed.CustomerID, b, "wrong-station"); err == nil {
		t.Fatal("cross-station checkout allowed")
	}
	_, cleared, err := service.SelectAddress(ctx, devseed.CustomerID, b)
	if err != nil || !cleared {
		t.Fatal("changing station did not clear incompatible cart", err)
	}
	cart, err := service.Get(ctx, devseed.CustomerID)
	if err != nil || len(cart.Items) != 0 {
		t.Fatal("cart retained incompatible items", err)
	}
	if err = service.Add(ctx, devseed.CustomerID, item, 1, b); err == nil {
		t.Fatal("cross-station cart insert allowed")
	}
	if err = service.Add(ctx, devseed.CustomerID, item, 1, a); err != nil {
		t.Fatal(err)
	}
	_, cleared, err = service.SelectAddress(ctx, devseed.CustomerID, outside)
	if err == nil || !cleared {
		t.Fatal("unserved address must fail and remove incompatible items")
	}
	if _, err = service.Checkout(ctx, devseed.CustomerID, outside, "outside"); err == nil {
		t.Fatal("unserved checkout allowed")
	}
	if _, _, err = service.SelectAddress(ctx, devseed.OwnerID, a); err == nil {
		t.Fatal("another customer's address was accepted")
	}
	if err = service.Add(ctx, devseed.CustomerID, item, 1, a); err != nil {
		t.Fatal(err)
	}
	result, err := service.Checkout(ctx, devseed.CustomerID, a, "covered")
	if err != nil {
		t.Fatal(err)
	}
	exec(t, p, `UPDATE public.stations SET active=false WHERE id=$1`, stationA)
	replay, err := service.Checkout(ctx, devseed.CustomerID, a, "covered")
	if err != nil || replay.ID != result.ID {
		t.Fatal("coverage changes must not break an already created order's idempotent replay", err)
	}
	cfg := &config.Config{JWTSecret: "station-test-secret-at-least-32-characters"}
	e := echo.New()
	group := e.Group("/api/v1/customer", auth.JWTMiddleware(cfg.JWTSecret, zap.NewNop(), p), auth.RBACMiddleware("CUSTOMER"))
	customer.RegisterRoutes(group, cfg, zap.NewNop(), customer.NewService(customer.NewPostgresRepo(p, zap.NewNop())))
	token, _ := auth.GenerateAccessToken(cfg.JWTSecret, devseed.CustomerID, "CUSTOMER")
	call := func(address string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/v1/customer/restaurants?address_id="+address, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	if rec := call(outside); rec.Code != 400 || !strings.Contains(rec.Body.String(), "unavailable") {
		t.Fatal("unserved discovery was not blocked", rec.Code, rec.Body.String())
	}
	exec(t, p, `UPDATE public.stations SET active=true WHERE id=$1`, stationA)
	if rec := call(b); rec.Code != 200 || strings.Contains(rec.Body.String(), devseed.RestaurantID) {
		t.Fatal("discovery leaked a restaurant from another resolved station", rec.Code, rec.Body.String())
	}
	if rec := call(a); rec.Code != 200 || !strings.Contains(rec.Body.String(), devseed.RestaurantID) {
		t.Fatal("covered restaurant not discoverable", rec.Code, rec.Body.String())
	}
}

func TestPartnerAvailabilityFreshnessAndAcceptedDeliverySurvivesBoundary(t *testing.T) {
	p := seed(t)
	ctx := context.Background()
	repo := rider.NewPostgresRepo(p, zap.NewNop())
	exec(t, p, `UPDATE delivery.live_locations SET observed_at=now()-interval '3 minutes' WHERE partner_id=$1`, devseed.PartnerID)
	if err := repo.UpdateStatus(ctx, devseed.PartnerID, "ONLINE"); err == nil {
		t.Fatal("stale location allowed activation")
	}
	fix := stations.Location{Latitude: ptr(34), Longitude: ptr(74), Accuracy: ptr(5), ObservedAt: time.Now(), Permission: true}
	if err := stations.RecordLocation(ctx, p, devseed.PartnerID, fix); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateStatus(ctx, devseed.PartnerID, "ONLINE"); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []stations.Location{
		{Latitude: ptr(34), Longitude: ptr(74), Accuracy: ptr(5), ObservedAt: time.Now(), Permission: false},
		{Latitude: ptr(34), Longitude: ptr(74), Accuracy: ptr(5), ObservedAt: time.Now(), Permission: true, Mocked: true},
		{Latitude: ptr(34), Longitude: ptr(74), Accuracy: ptr(101), ObservedAt: time.Now(), Permission: true},
		{Latitude: ptr(34), Longitude: ptr(74), Accuracy: ptr(5), ObservedAt: time.Now().Add(-3 * time.Minute), Permission: true},
		{Latitude: ptr(34), Longitude: ptr(74), Accuracy: ptr(5), ObservedAt: time.Now().Add(time.Minute), Permission: true},
	} {
		if err := stations.RecordLocation(ctx, p, devseed.PartnerID, invalid); err == nil {
			t.Fatal("unverified location accepted")
		}
	}
	fix.ObservedAt = time.Now()
	if err := stations.RecordLocation(ctx, p, devseed.PartnerID, fix); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateStatus(ctx, devseed.PartnerID, "ONLINE"); err != nil {
		t.Fatal(err)
	}
	exec(t, p, `INSERT INTO delivery.offers(order_id,partner_id,expires_at) VALUES('10000000-0000-4000-a000-000000000107',$1,now()+interval '5 minutes')`, devseed.PartnerID)
	exec(t, p, `UPDATE delivery.live_locations SET observed_at=now()-interval '110 seconds' WHERE partner_id=$1`, devseed.PartnerID)
	outside := stations.Location{Latitude: ptr(34.05), Longitude: ptr(74), Accuracy: ptr(5), ObservedAt: time.Now(), Permission: true}
	if err := stations.RecordLocation(ctx, p, devseed.PartnerID, outside); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateStatus(ctx, devseed.PartnerID, "ONLINE"); err == nil {
		t.Fatal("outside assigned station allowed activation")
	}
	exec(t, p, `INSERT INTO public.stations(id,name,latitude,longitude,radius_m) VALUES($1,'Other coverage',34.05,74,2000)`, stationB)
	if err := repo.UpdateStatus(ctx, devseed.PartnerID, "ONLINE"); err == nil {
		t.Fatal("entering another station allowed activation")
	}
	var pending int
	var status string
	if err := p.QueryRow(ctx, `SELECT count(*) FROM delivery.offers WHERE partner_id=$1 AND status='PENDING'`, devseed.PartnerID).Scan(&pending); err != nil || pending != 0 {
		t.Fatal("outside location left pending offers", err)
	}
	if err := p.QueryRow(ctx, `SELECT status FROM delivery.delivery_partners WHERE id=$1`, devseed.PartnerID).Scan(&status); err != nil || status != "OFFLINE" {
		t.Fatal("outside partner not safely offline", err)
	}
	if err := dispatch.Accept(ctx, p, devseed.PartnerID, "10000000-0000-4000-a000-000000000107"); err == nil {
		t.Fatal("outside partner accepted an offer")
	}
	order := "10000000-0000-4000-a000-000000000108"
	for _, next := range []string{"PICKED_UP", "OUT_FOR_DELIVERY", "DELIVERED"} {
		if err := repo.UpdateJobStatus(ctx, devseed.PartnerID, order, next); err != nil {
			t.Fatal("boundary blocked accepted delivery completion", next, err)
		}
	}
}

func TestStationAdminApprovalOwnershipAndRevalidation(t *testing.T) {
	p := seed(t)
	ctx := context.Background()
	cfg := &config.Config{JWTSecret: "station-test-secret-at-least-32-characters"}
	e := echo.New()
	stations.Register(e.Group("/api/v1"), p, cfg, zap.NewNop())
	var reviewer string
	if err := p.QueryRow(ctx, `INSERT INTO public.users(email,password_hash,role) VALUES('reviewer@test.invalid','hash','ADMIN') RETURNING id`).Scan(&reviewer); err != nil {
		t.Fatal(err)
	}
	call := func(user, role, method, path string, body any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		token, _ := auth.GenerateAccessToken(cfg.JWTSecret, user, role)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	body := map[string]any{"name": "New station", "latitude": 34.01, "longitude": 74, "radius_m": 6000, "active": true, "reason": "Expand local coverage"}
	if rec := call(devseed.CustomerID, "CUSTOMER", "POST", "/admin/stations", body); rec.Code != 403 {
		t.Fatal("customer managed stations", rec.Code)
	}
	rec := call(devseed.AdminID, "ADMIN", "POST", "/admin/stations", body)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var created map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	request := map[string]any{"kind": "delivery", "resource_id": devseed.PartnerID, "station_id": created["id"], "reason": "Transfer operational station"}
	if rec = call(devseed.OwnerID, "MERCHANT_OWNER", "POST", "/station-changes", request); rec.Code != 403 {
		t.Fatal("another partner requested a station change", rec.Code)
	}
	rec = call(devseed.AdminID, "ADMIN", "POST", "/station-changes", request)
	if rec.Code != 201 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var change map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &change)
	path := "/admin/station-changes/" + change["id"] + "/decision"
	decision := map[string]any{"approve": true, "reason": "Verified target station"}
	if rec = call(devseed.AdminID, "ADMIN", "POST", path, decision); rec.Code != 403 {
		t.Fatal("requester approved own transfer", rec.Code, rec.Body.String())
	}
	if rec = call(reviewer, "ADMIN", "POST", path, decision); rec.Code != 409 {
		t.Fatal("transfer with active accepted delivery allowed", rec.Code, rec.Body.String())
	}
	exec(t, p, `UPDATE delivery.delivery_assignments SET status='DELIVERED' WHERE partner_id=$1`, devseed.PartnerID)
	exec(t, p, `UPDATE public.stations SET active=false WHERE id=$1`, created["id"])
	if rec = call(reviewer, "ADMIN", "POST", path, decision); rec.Code != 409 {
		t.Fatal("inactive target passed review", rec.Code, rec.Body.String())
	}
	exec(t, p, `UPDATE public.stations SET active=true WHERE id=$1`, created["id"])
	if rec = call(reviewer, "ADMIN", "POST", path, decision); rec.Code != 204 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if rec = call(reviewer, "ADMIN", "POST", path, decision); rec.Code != 409 {
		t.Fatal("approval replay succeeded", rec.Code)
	}
	var assigned, status string
	if err := p.QueryRow(ctx, `SELECT station_id,status FROM delivery.delivery_partners WHERE id=$1`, devseed.PartnerID).Scan(&assigned, &status); err != nil || assigned != created["id"] || status != "OFFLINE" {
		t.Fatal("transfer did not set station and require explicit reactivation", err)
	}
	var audits int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM public.admin_audit_logs WHERE action='STATION_CHANGE_APPROVED' AND target_id=$1`, devseed.PartnerID).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("station approval audit missing", err)
	}
}

func TestDispatchRevalidatesLocationBeforeOfferingAndAccepting(t *testing.T) {
	p := seed(t)
	ctx := context.Background()
	exec(t, p, `UPDATE delivery.delivery_assignments SET status='DELIVERED' WHERE partner_id=$1`, devseed.PartnerID)
	exec(t, p, `UPDATE delivery.delivery_partners SET status='ONLINE' WHERE id=$1`, devseed.PartnerID)
	if err := dispatch.Tick(ctx, p); err != nil {
		t.Fatal(err)
	}
	var order string
	if err := p.QueryRow(ctx, `SELECT order_id FROM delivery.offers WHERE partner_id=$1 AND status='PENDING'`, devseed.PartnerID).Scan(&order); err != nil {
		t.Fatal("eligible partner received no offer", err)
	}
	exec(t, p, `UPDATE delivery.live_locations SET observed_at=now()-interval '3 minutes' WHERE partner_id=$1`, devseed.PartnerID)
	if err := dispatch.Accept(ctx, p, devseed.PartnerID, order); err == nil {
		t.Fatal("stale partner accepted an outstanding offer")
	}
	if err := dispatch.Tick(ctx, p); err != nil {
		t.Fatal(err)
	}
	var count int
	var status string
	if err := p.QueryRow(ctx, `SELECT count(*) FROM delivery.offers WHERE partner_id=$1 AND status='PENDING'`, devseed.PartnerID).Scan(&count); err != nil || count != 0 {
		t.Fatal("stale offers were not expired", err)
	}
	if err := p.QueryRow(ctx, `SELECT status FROM delivery.delivery_partners WHERE id=$1`, devseed.PartnerID).Scan(&status); err != nil || status != "OFFLINE" {
		t.Fatal("stale available partner was not taken offline", err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO delivery.delivery_assignments(order_id,partner_id,status) VALUES($1,$2,'ASSIGNED')`, order, devseed.PartnerID); err == nil {
		t.Fatal("alternate assignment path bypassed geofence")
	}
}

func TestLocationReplayAndImpossibleMovementFailClosed(t *testing.T) {
	p := seed(t)
	ctx := context.Background()
	var observed time.Time
	if err := p.QueryRow(ctx, `SELECT observed_at FROM delivery.live_locations WHERE partner_id=$1 ORDER BY recorded_at DESC LIMIT 1`, devseed.PartnerID).Scan(&observed); err != nil {
		t.Fatal(err)
	}
	fix := stations.Location{Latitude: ptr(34), Longitude: ptr(74), Accuracy: ptr(5), ObservedAt: observed, Permission: true}
	if err := stations.RecordLocation(ctx, p, devseed.PartnerID, fix); err == nil {
		t.Fatal("replayed location accepted")
	}
	fix.ObservedAt = time.Now()
	fix.Latitude = ptr(35)
	if err := stations.RecordLocation(ctx, p, devseed.PartnerID, fix); err == nil {
		t.Fatal("impossible GPS movement accepted")
	}
}

func TestCoverageDeactivationPreservesAcceptedDelivery(t *testing.T) {
	p := seed(t)
	ctx := context.Background()
	repo := rider.NewPostgresRepo(p, zap.NewNop())
	if err := repo.UpdateStatus(ctx, devseed.PartnerID, "ONLINE"); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{JWTSecret: "station-test-secret-at-least-32-characters"}
	e := echo.New()
	stations.Register(e.Group("/api/v1"), p, cfg, zap.NewNop())
	body, _ := json.Marshal(map[string]any{"name": "Paused coverage", "latitude": 34, "longitude": 74, "radius_m": 5000, "active": false, "reason": "Pause station operations"})
	req := httptest.NewRequest("PUT", "/api/v1/admin/stations/"+stationA, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	token, _ := auth.GenerateAccessToken(cfg.JWTSecret, devseed.AdminID, "ADMIN")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var open bool
	var status string
	if err := p.QueryRow(ctx, `SELECT is_active FROM public.merchants WHERE id=$1`, devseed.RestaurantID).Scan(&open); err != nil || open {
		t.Fatal("station deactivation left restaurant open", err)
	}
	if err := p.QueryRow(ctx, `SELECT status FROM delivery.delivery_partners WHERE id=$1`, devseed.PartnerID).Scan(&status); err != nil || status != "OFFLINE" {
		t.Fatal("station deactivation left partner online", err)
	}
	order := "10000000-0000-4000-a000-000000000108"
	for _, next := range []string{"PICKED_UP", "OUT_FOR_DELIVERY", "DELIVERED"} {
		if err := repo.UpdateJobStatus(ctx, devseed.PartnerID, order, next); err != nil {
			t.Fatal("inactive station blocked accepted delivery", err)
		}
	}
}

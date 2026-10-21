package admin

import (
    "context"
    "encoding/json"
    "strings"
    "testing"

    "github.com/imtiyazkumar/yember-core/config"
    "github.com/imtiyazkumar/yember-core/internal/auth"
    "github.com/imtiyazkumar/yember-core/internal/testdb"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
    "net/http/httptest"
)

func TestConsoleManagementAndAudit(t *testing.T) {
    pool := testdb.New(t)
    ctx := context.Background()
    repo := &pgRepo{db: pool, log: zap.NewNop()}
    var actor, customer, owner, restaurant, category, item, partnerUser, partner string
    for _, user := range []struct {
        email, role string
        dest        *string
    }{{"admin@test.invalid", "ADMIN", &actor}, {"customer@test.invalid", "CUSTOMER", &customer}, {"owner@test.invalid", "MERCHANT_OWNER", &owner}, {"partner@test.invalid", "DELIVERY_PARTNER", &partnerUser}} {
        if err := pool.QueryRow(ctx, `INSERT INTO public.users(email,password_hash,role,full_name) VALUES($1,'never-serialize-this-hash',$2,$1) RETURNING id`, user.email, user.role).Scan(user.dest); err != nil {
            t.Fatal(err)
        }
    }
    if _, err := pool.Exec(ctx, `INSERT INTO public.customers(id,user_id,email,name) VALUES($1,$1,'customer@test.invalid','Customer')`, customer); err != nil {
        t.Fatal(err)
    }
    if err := pool.QueryRow(ctx, `INSERT INTO public.merchants(name,email,owner_id,is_active,approval_status) VALUES('Kitchen','kitchen@test.invalid',$1,false,'APPROVED') RETURNING id`, owner).Scan(&restaurant); err != nil {
        t.Fatal(err)
    }
    if err := pool.QueryRow(ctx, `INSERT INTO catalog.categories(merchant_id,name) VALUES($1,'Meals') RETURNING id`, restaurant).Scan(&category); err != nil {
        t.Fatal(err)
    }
    if err := pool.QueryRow(ctx, `INSERT INTO catalog.items(merchant_id,category_id,name,base_price) VALUES($1,$2,'Lunch',25000) RETURNING id`, restaurant, category).Scan(&item); err != nil {
        t.Fatal(err)
    }
    if err := pool.QueryRow(ctx, `INSERT INTO delivery.delivery_partners(user_id,name,phone,approval_status) VALUES($1,'Partner','1234567890','APPROVED') RETURNING id`, partnerUser).Scan(&partner); err != nil {
        t.Fatal(err)
    }
    if _, err := pool.Exec(ctx, `INSERT INTO delivery.vehicles(partner_id,vehicle_type,registration_number) VALUES($1,'BIKE','TEST')`, partner); err != nil {
        t.Fatal(err)
    }
    for resource := range consoleViews {
        page, err := repo.ConsoleList(ctx, resource, ListFilter{Page: 1, Limit: 25})
        if err != nil {
            t.Fatalf("list %s: %v", resource, err)
        }
        data, _ := json.Marshal(page)
        if strings.Contains(string(data), "password_hash") || strings.Contains(string(data), "never-serialize") {
            t.Fatal("credential field leaked")
        }
    }
    page, err := repo.ConsoleList(ctx, "customers", ListFilter{Search: "customer@test", Status: "ACTIVE", Page: 1, Limit: 25})
    if err != nil || page.Total != 1 {
        t.Fatalf("customer search failed: %v", err)
    }
    injection, err := repo.ConsoleList(ctx, "customers", ListFilter{Search: "' OR true --", Page: 1, Limit: 25})
    if err != nil || injection.Total != 0 {
        t.Fatal("search escaped its parameter boundary")
    }
    detail, err := repo.ConsoleDetail(ctx, "restaurants", restaurant)
    if err != nil {
        t.Fatal(err)
    }
    if len(detail["menu"].([]json.RawMessage)) != 1 {
        t.Fatal("menu not included")
    }
    if err = repo.ConsoleAvailability(ctx, actor, "restaurants", restaurant, true, "Open for business"); err == nil {
        t.Fatal("unpaid restaurant activated")
    }
    if _, err = pool.Exec(ctx, `UPDATE public.merchants SET fee_payment_status='PAID' WHERE id=$1`, restaurant); err != nil {
        t.Fatal(err)
    }
    if err = repo.ConsoleAvailability(ctx, actor, "restaurants", restaurant, true, "Payment verified"); err != nil {
        t.Fatal(err)
    }
    if err = repo.ConsoleMenu(ctx, actor, restaurant, item, MenuEdit{Name: "Updated lunch", Price: 30000, Available: false, Reason: "Sold out"}); err != nil {
        t.Fatal(err)
    }
    if err = repo.ConsoleEditProfile(ctx, customer, "customers", customer, ProfileEdit{Name: "Unauthorized", Reason: "Bad actor"}); err == nil {
        t.Fatal("non-admin edited profile")
    }
    if err = repo.ConsoleEditProfile(ctx, actor, "customers", customer, ProfileEdit{Name: "Updated customer", Phone: "1234567890", Reason: "Contact corrected"}); err != nil {
        t.Fatal(err)
    }
    if err = repo.ConsoleEditProfile(ctx, actor, "restaurants", restaurant, ProfileEdit{Name: "Kitchen", Address: "Primary address", Reason: "Record branch address"}); err != nil {
        t.Fatal(err)
    }
    var secondary string
    if err = pool.QueryRow(ctx, `INSERT INTO merchants.merchant_branches(merchant_id,name,address,created_at) VALUES($1,'Second branch','Keep this address',now()+interval '1 second') RETURNING id`, restaurant).Scan(&secondary); err != nil {
        t.Fatal(err)
    }
    if err = repo.ConsoleEditProfile(ctx, actor, "restaurants", restaurant, ProfileEdit{Name: "Kitchen", Address: "Corrected primary address", Reason: "Correct primary branch"}); err != nil {
        t.Fatal(err)
    }
    var address string
    if err = pool.QueryRow(ctx, `SELECT address FROM merchants.merchant_branches WHERE id=$1`, secondary).Scan(&address); err != nil || address != "Keep this address" {
        t.Fatal("primary branch edit changed another branch")
    }
    var review string
    if err = pool.QueryRow(ctx, `INSERT INTO merchants.restaurant_reviews(customer_id,merchant_id,rating,comment) VALUES($1,$2,4,'Review text') RETURNING id`, customer, restaurant).Scan(&review); err != nil {
        t.Fatal(err)
    }
    if err = repo.ConsoleOperation(ctx, actor, "reviews", review, false, "Moderation decision"); err != nil {
        t.Fatal(err)
    }
    hidden, err := repo.ConsoleList(ctx, "reviews", ListFilter{Status: "HIDDEN"})
    if err != nil || hidden.Total != 1 {
        t.Fatal("review moderation did not persist")
    }
    if err = repo.ConsoleOperation(ctx, actor, "reviews", review, true, "Appeal accepted"); err != nil {
        t.Fatal(err)
    }
    published, err := repo.ConsoleList(ctx, "reviews", ListFilter{Status: "PUBLISHED"})
    if err != nil || published.Total != 1 {
        t.Fatal("review was not restored")
    }
    if err = repo.ConsoleDisable(ctx, actor, actor, true, "Disable myself"); err == nil {
        t.Fatal("admin disabled self")
    }
    if err = repo.ConsoleDisable(ctx, actor, customer, true, ""); err == nil {
        t.Fatal("missing reason accepted")
    }
    if err = repo.ConsoleDisable(ctx, actor, customer, true, "Customer request"); err != nil {
        t.Fatal(err)
    }
    filtered, err := repo.ConsoleList(ctx, "customers", ListFilter{Status: "DISABLED"})
    if err != nil || filtered.Total != 1 {
        t.Fatal("account restriction did not persist")
    }
    if err = repo.ConsoleCoupon(ctx, actor, "", CouponInput{Code: "TEST20", DiscountType: "PERCENTAGE", DiscountValue: 20, IsActive: true}, "Launch promotion"); err != nil {
        t.Fatal(err)
    }
    coupons, err := repo.ConsoleList(ctx, "coupons", ListFilter{})
    if err != nil || coupons.Total != 1 {
        t.Fatal("coupon was not created")
    }
    var coupon struct {
        ID string `json:"id"`
    }
    _ = json.Unmarshal(coupons.Items[0], &coupon)
    if err = repo.ConsoleCoupon(ctx, actor, coupon.ID, CouponInput{Code: "TEST20", DiscountType: "PERCENTAGE", DiscountValue: 20, IsActive: false}, "Promotion ended"); err != nil {
        t.Fatal(err)
    }
    var email string
    if err = pool.QueryRow(ctx, `INSERT INTO notifications.email_outbox(recipient,subject,body,status) VALUES('customer@test.invalid','Test email','Private verification token','FAILED') RETURNING id`).Scan(&email); err != nil {
        t.Fatal(err)
    }
    if err = repo.ConsoleOperation(ctx, actor, "emails", email, true, "Retry transient failure"); err != nil {
        t.Fatal(err)
    }
    if err = repo.ConsoleOperation(ctx, actor, "emails", email, true, "Duplicate retry"); err == nil {
        t.Fatal("pending email retried again")
    }
    emails, err := repo.ConsoleList(ctx, "emails", ListFilter{})
    if err != nil {
        t.Fatal(err)
    }
    serialized, _ := json.Marshal(emails)
    if strings.Contains(string(serialized), "Private verification token") {
        t.Fatal("email verification token leaked in admin list")
    }
    if _, err = repo.ConsoleOverview(ctx); err != nil {
        t.Fatal(err)
    }
    cfg := &config.Config{JWTSecret: "isolated-administrator-console-test-secret"}
    e := echo.New()
    RegisterRoutes(e.Group("/api/v1/admin", auth.JWTMiddleware(cfg.JWTSecret, zap.NewNop(), pool), auth.RBACMiddleware("ADMIN")), cfg, zap.NewNop(), NewService(repo))
    customerToken, _ := auth.GenerateAccessToken(cfg.JWTSecret, owner, "MERCHANT_OWNER")
    request := httptest.NewRequest("GET", "/api/v1/admin/manage/customers", nil)
    request.Header.Set("Authorization", "Bearer "+customerToken)
    record := httptest.NewRecorder()
    e.ServeHTTP(record, request)
    if record.Code != 403 {
        t.Fatalf("non-admin console access returned %d", record.Code)
    }
    adminToken, _ := auth.GenerateAccessToken(cfg.JWTSecret, actor, "ADMIN")
    request = httptest.NewRequest("GET", "/api/v1/admin/manage/customers/not-a-uuid", nil)
    request.Header.Set("Authorization", "Bearer "+adminToken)
    record = httptest.NewRecorder()
    e.ServeHTTP(record, request)
    if record.Code != 400 {
        t.Fatalf("invalid ID returned %d", record.Code)
    }
    var count int
    if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.admin_audit_logs`).Scan(&count); err != nil || count < 6 {
        t.Fatal("management changes lack audit records")
    }
}

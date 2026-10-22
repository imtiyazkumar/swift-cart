package checkout_test

import (
    "bytes"
    "context"
    "encoding/json"
    "image"
    "image/png"
    "mime/multipart"
    "net/http/httptest"
    "testing"

    "github.com/imtiyazkumar/yember-core/config"
    "github.com/imtiyazkumar/yember-core/internal/auth"
    "github.com/imtiyazkumar/yember-core/internal/checkout"
    "github.com/imtiyazkumar/yember-core/internal/customer"
    "github.com/imtiyazkumar/yember-core/internal/dispatch"
    "github.com/imtiyazkumar/yember-core/internal/restaurant"
    "github.com/imtiyazkumar/yember-core/internal/rider"
    "github.com/imtiyazkumar/yember-core/internal/testdb"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

func TestCustomerToRestaurantToDeliveryAndRatings(t *testing.T) {
    t.Setenv("CUSTOMER_PAYMENT_METHOD", "CASH_ON_DELIVERY")
    pool := testdb.New(t)
    testdb.Station(t, pool)
    ctx := context.Background()
    log := zap.NewNop()
    cfg := &config.Config{JWTSecret: "isolated-full-workflow-secret-32-characters"}
    var customerID, owner, merchant, partnerUser, partner, category, item, address string
    for _, user := range []struct {
        email, role string
        id          *string
    }{{"customer@test.invalid", "CUSTOMER", &customerID}, {"owner@test.invalid", "MERCHANT_OWNER", &owner}, {"partner@test.invalid", "DELIVERY_PARTNER", &partnerUser}} {
        if err := pool.QueryRow(ctx, `INSERT INTO public.users(email,password_hash,role,email_verified_at) VALUES($1,'hash',$2,now()) RETURNING id`, user.email, user.role).Scan(user.id); err != nil {
            t.Fatal(err)
        }
    }
    if _, err := pool.Exec(ctx, `INSERT INTO public.customers(id,name,email) VALUES($1,'Customer','customer@test.invalid')`, customerID); err != nil {
        t.Fatal(err)
    }
    if err := pool.QueryRow(ctx, `INSERT INTO public.merchants(name,email,owner_id,is_active,approval_status,fee_payment_status,latitude,longitude) VALUES('Kitchen','kitchen@test.invalid',$1,true,'APPROVED','PAID',34,74) RETURNING id`, owner).Scan(&merchant); err != nil {
        t.Fatal(err)
    }
    if err := pool.QueryRow(ctx, `INSERT INTO catalog.categories(merchant_id,name) VALUES($1,'Meals') RETURNING id`, merchant).Scan(&category); err != nil {
        t.Fatal(err)
    }
    if err := pool.QueryRow(ctx, `INSERT INTO catalog.items(merchant_id,category_id,name,base_price) VALUES($1,$2,'Lunch',25000) RETURNING id`, merchant, category).Scan(&item); err != nil {
        t.Fatal(err)
    }
    if err := pool.QueryRow(ctx, `INSERT INTO delivery.delivery_partners(user_id,name,phone,approval_status,status,station_id) VALUES($1,'Partner','1234567890','APPROVED','OFFLINE','20000000-0000-4000-a000-000000000001') RETURNING id`, partnerUser).Scan(&partner); err != nil {
        t.Fatal(err)
    }
    if _, err := pool.Exec(ctx, `INSERT INTO delivery.vehicles(partner_id,vehicle_type,registration_number) VALUES($1,'BIKE','TEST')`, partner); err != nil {
        t.Fatal(err)
    }
    if err := pool.QueryRow(ctx, `INSERT INTO users.customer_addresses(customer_id,label,address_line1,city,state,postal_code,latitude,longitude) VALUES($1,'Home','Test Street','Test City','Test State','123456',34,74) RETURNING id`, customerID).Scan(&address); err != nil {
        t.Fatal(err)
    }
    testdb.PartnerLocation(t, pool, partner)
    service := checkout.New(pool)
    if err := service.Add(ctx, customerID, item, 2, address); err != nil {
        t.Fatal(err)
    }
    order, err := service.Checkout(ctx, customerID, address, "isolated-idempotency-key")
    if err != nil {
        t.Fatal(err)
    }
    replay, err := service.Checkout(ctx, customerID, address, "isolated-idempotency-key")
    if err != nil || replay.ID != order.ID {
        t.Fatal("checkout retry duplicated an order")
    }
    if order.Subtotal != 50000 {
        t.Fatal("authoritative order total is incorrect")
    }
    if _, err = pool.Exec(ctx, `UPDATE catalog.items SET name='Changed lunch',base_price=99999 WHERE id=$1`, item); err != nil {
        t.Fatal(err)
    }
    var snapshotName string
    var snapshotPrice int64
    if err = pool.QueryRow(ctx, `SELECT name_snapshot,unit_price FROM orders.order_items WHERE order_id=$1`, order.ID).Scan(&snapshotName, &snapshotPrice); err != nil || snapshotName != "Lunch" || snapshotPrice != 25000 {
        t.Fatal("order item snapshot changed with catalog")
    }
    e := echo.New()
    api := e.Group("/api/v1")
    restaurant.RegisterImages(api, pool, cfg, log)
    restaurant.RegisterRoutes(api.Group("/restaurant", auth.JWTMiddleware(cfg.JWTSecret, log, pool), auth.RBACMiddleware("MERCHANT_OWNER")), cfg, log, restaurant.NewService(restaurant.NewPostgresRepo(pool, log)))
    rider.RegisterRoutes(api.Group("/rider", auth.JWTMiddleware(cfg.JWTSecret, log, pool), auth.RBACMiddleware("DELIVERY_PARTNER")), cfg, log, rider.NewService(rider.NewPostgresRepo(pool, log)))
    ownerToken, _ := auth.GenerateAccessToken(cfg.JWTSecret, owner, "MERCHANT_OWNER")
    partnerToken, _ := auth.GenerateAccessToken(cfg.JWTSecret, partnerUser, "DELIVERY_PARTNER")
    t.Setenv("DOCUMENT_STORAGE_BACKEND", "local")
    t.Setenv("DOCUMENT_STORAGE_DIRECTORY", t.TempDir())
    t.Setenv("PUBLIC_API_BASE_URL", "http://example.invalid/api/v1")
    var imageBody bytes.Buffer
    writer := multipart.NewWriter(&imageBody)
    part, _ := writer.CreateFormFile("file", "meal.png")
    if err = png.Encode(part, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
        t.Fatal(err)
    }
    _ = writer.Close()
    imageRequest := httptest.NewRequest("POST", "/api/v1/restaurant/menu/items/"+item+"/image", &imageBody)
    imageRequest.Header.Set("Authorization", "Bearer "+ownerToken)
    imageRequest.Header.Set("Content-Type", writer.FormDataContentType())
    imageResponse := httptest.NewRecorder()
    e.ServeHTTP(imageResponse, imageRequest)
    if imageResponse.Code != 201 {
        t.Fatalf("menu image upload: %s", imageResponse.Body.String())
    }
    request := func(path, token string, body interface{}, expected int) {
        t.Helper()
        data, _ := json.Marshal(body)
        req := httptest.NewRequest("PATCH", path, bytes.NewReader(data))
        req.Header.Set("Content-Type", "application/json")
        req.Header.Set("Authorization", "Bearer "+token)
        rec := httptest.NewRecorder()
        e.ServeHTTP(rec, req)
        if rec.Code != expected {
            t.Fatalf("%s returned %d expected %d: %s", path, rec.Code, expected, rec.Body.String())
        }
    }
    orderPath := "/api/v1/restaurant/orders/" + order.ID + "/status"
    request(orderPath, ownerToken, map[string]string{"status": "READY_FOR_PICKUP"}, 400)
    request(orderPath, ownerToken, map[string]string{"status": "ACCEPTED", "merchant_id": "00000000-0000-4000-a000-000000000000"}, 200)
    reviews := customer.NewService(customer.NewPostgresRepo(pool, log))
    if err = reviews.SubmitReview(ctx, customerID, &customer.ReviewRequest{OrderID: order.ID, Rating: 5}); err == nil {
        t.Fatal("review accepted before delivery")
    }
    if err = dispatch.Tick(ctx, pool); err != nil {
        t.Fatal(err)
    }
    if err = dispatch.Accept(ctx, pool, partner, order.ID); err != nil {
        t.Fatal(err)
    }
    deliveryPath := "/api/v1/rider/deliveries/" + order.ID + "/status"
    request(deliveryPath, partnerToken, map[string]string{"status": "DELIVERED"}, 400)
    request(deliveryPath, partnerToken, map[string]string{"status": "PICKED_UP"}, 400)
    request(orderPath, ownerToken, map[string]string{"status": "PREPARING"}, 200)
    request(orderPath, ownerToken, map[string]string{"status": "READY_FOR_PICKUP"}, 200)
    request(deliveryPath, partnerToken, map[string]string{"status": "PICKED_UP"}, 200)
    request(deliveryPath, partnerToken, map[string]string{"status": "DELIVERED"}, 200)
    request(deliveryPath, partnerToken, map[string]string{"status": "DELIVERED"}, 400)
    if err = reviews.SubmitReview(ctx, customerID, &customer.ReviewRequest{OrderID: order.ID, Rating: 5, DeliveryRating: 4, Comment: "Test order"}); err != nil {
        t.Fatal(err)
    }
    if err = reviews.SubmitReview(ctx, customerID, &customer.ReviewRequest{OrderID: order.ID, Rating: 5}); err == nil {
        t.Fatal("duplicate review accepted")
    }
}

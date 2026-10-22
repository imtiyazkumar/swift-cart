package partnerops

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/imtiyazkumar/yember-core/config"
    "github.com/imtiyazkumar/yember-core/internal/admin"
    "github.com/imtiyazkumar/yember-core/internal/auth"
    "github.com/imtiyazkumar/yember-core/internal/devseed"
    "github.com/imtiyazkumar/yember-core/internal/testdb"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

func TestPartnerWorkspaceOwnershipHistoryAndSupport(t *testing.T) {
    pool := testdb.New(t)
    ctx := context.Background()
    const password = "LocalTestPassword!2026"
    if err := devseed.Seed(ctx, pool, password); err != nil {
        t.Fatal(err)
    }
    cfg := &config.Config{JWTSecret: "isolated-partner-workspace-test-secret-with-32-characters"}
    e := echo.New()
    Register(e.Group("/api/v1"), pool, cfg, zap.NewNop())
    admin.RegisterRoutes(e.Group("/api/v1/admin", auth.JWTMiddleware(cfg.JWTSecret, zap.NewNop(), pool), auth.RBACMiddleware("ADMIN")), cfg, zap.NewNop(), admin.NewService(admin.NewPostgresRepo(pool, zap.NewNop())))
    call := func(user, role, method, path string, body interface{}) *httptest.ResponseRecorder {
        t.Helper()
        var payload []byte
        if body != nil {
            payload, _ = json.Marshal(body)
        }
        request := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(payload))
        request.Header.Set("Content-Type", "application/json")
        if user != "" {
            token, _ := auth.GenerateAccessToken(cfg.JWTSecret, user, role)
            request.Header.Set("Authorization", "Bearer "+token)
        }
        result := httptest.NewRecorder()
        e.ServeHTTP(result, request)
        return result
    }
    expect := func(result *httptest.ResponseRecorder, status int) {
        t.Helper()
        if result.Code != status {
            t.Fatalf("got %d expected %d: %s", result.Code, status, result.Body.String())
        }
    }
    expect(call("", "", "GET", "/partner/overview", nil), 401)
    expect(call(devseed.CustomerID, "CUSTOMER", "GET", "/partner/overview", nil), 403)
    for _, user := range []struct{ id, role string }{{devseed.OwnerID, "MERCHANT_OWNER"}, {devseed.DeliveryUserID, "DELIVERY_PARTNER"}} {
        result := call(user.id, user.role, "GET", "/partner/overview?days=7", nil)
        expect(result, 200)
        var data map[string]interface{}
        _ = json.Unmarshal(result.Body.Bytes(), &data)
        if len(data["daily"].([]interface{})) != 7 {
            t.Fatal("overview must include zero-filled UTC days")
        }
        result = call(user.id, user.role, "GET", "/partner/orders?scope=history&limit=1&page=1", nil)
        expect(result, 200)
        var page Page
        _ = json.Unmarshal(result.Body.Bytes(), &page)
        if page.Total != 7 || len(page.Items) != 1 {
            t.Fatalf("history pagination invalid for %s: %s", user.role, result.Body.String())
        }
    }
    expect(call(devseed.DeliveryUserID, "DELIVERY_PARTNER", "GET", "/partner/earnings?days=30", nil), 200)
    expect(call(devseed.OwnerID, "MERCHANT_OWNER", "GET", "/partner/earnings", nil), 403)
    readyID := "10000000-0000-4000-a000-000000000108"
    deliveredID := "10000000-0000-4000-a000-000000000100"
    active := call(devseed.DeliveryUserID, "DELIVERY_PARTNER", "GET", "/partner/orders/"+readyID, nil)
    expect(active, 200)
    if !strings.Contains(active.Body.String(), "12 Local Test Street") || !strings.Contains(active.Body.String(), "+919000000002") {
        t.Fatal("active courier lacks delivery contact")
    }
    completed := call(devseed.DeliveryUserID, "DELIVERY_PARTNER", "GET", "/partner/orders/"+deliveredID, nil)
    expect(completed, 200)
    if strings.Contains(completed.Body.String(), "+919000000002") {
        t.Fatal("completed delivery leaked customer phone")
    }
    var stranger string
    if err := pool.QueryRow(ctx, `INSERT INTO public.users(email,password_hash,role,full_name) VALUES('other-courier@test.invalid','hash','DELIVERY_PARTNER','Other courier') RETURNING id`).Scan(&stranger); err != nil {
        t.Fatal(err)
    }
    expect(call(stranger, "DELIVERY_PARTNER", "GET", "/partner/orders/"+readyID, nil), 404)
    expect(call(devseed.OwnerID, "MERCHANT_OWNER", "GET", "/partner/orders/not-a-uuid", nil), 400)
    reviews := call(devseed.OwnerID, "MERCHANT_OWNER", "GET", "/partner/reviews", nil)
    expect(reviews, 200)
    expect(call(devseed.OwnerID, "MERCHANT_OWNER", "PUT", "/partner/reviews/10000000-0000-4000-a000-000000000060/reply", map[string]string{"reply": "Thank you for your feedback."}), 204)
    expect(call(devseed.DeliveryUserID, "DELIVERY_PARTNER", "PUT", "/partner/reviews/10000000-0000-4000-a000-000000000060/reply", map[string]string{"reply": "Wrong role"}), 403)
    input := TicketInput{Category: "ORDER", Subject: "Pickup assistance", Message: "Please help with this local delivery.", OrderID: readyID, RequestKey: "test-request-key-1"}
    created := call(devseed.DeliveryUserID, "DELIVERY_PARTNER", "POST", "/partner/support", input)
    expect(created, 201)
    var ticket struct {
        ID string `json:"id"`
    }
    _ = json.Unmarshal(created.Body.Bytes(), &ticket)
    repeat := call(devseed.DeliveryUserID, "DELIVERY_PARTNER", "POST", "/partner/support", input)
    expect(repeat, 201)
    var repeated struct {
        ID string `json:"id"`
    }
    _ = json.Unmarshal(repeat.Body.Bytes(), &repeated)
    if ticket.ID != repeated.ID {
        t.Fatal("idempotent ticket creation duplicated the case")
    }
    input.OrderID = deliveredID
    input.RequestKey = "unauthorized-order-key"
    expect(call(stranger, "DELIVERY_PARTNER", "POST", "/partner/support", input), 404)
    expect(call(devseed.OwnerID, "MERCHANT_OWNER", "GET", "/partner/support/"+ticket.ID, nil), 404)
    expect(call(devseed.OwnerID, "MERCHANT_OWNER", "POST", "/partner/support/"+ticket.ID+"/messages", map[string]string{"body": "Cross-owner reply"}), 404)
    expect(call(devseed.AdminID, "ADMIN", "POST", "/admin/partner-support/"+ticket.ID+"/messages", map[string]string{"body": "We have checked your order."}), 204)
    expect(call(devseed.AdminID, "ADMIN", "POST", "/admin/manage/support/"+ticket.ID+"/operation", map[string]interface{}{"enabled": false, "reason": "Issue answered and resolved"}), 204)
    expect(call(devseed.DeliveryUserID, "DELIVERY_PARTNER", "POST", "/partner/support/"+ticket.ID+"/messages", map[string]string{"body": "A follow-up question."}), 204)
    conversation := call(devseed.DeliveryUserID, "DELIVERY_PARTNER", "GET", "/partner/support/"+ticket.ID, nil)
    expect(conversation, 200)
    if !strings.Contains(conversation.Body.String(), `"status":"OPEN"`) && !strings.Contains(conversation.Body.String(), `"status": "OPEN"`) {
        t.Fatal("follow-up did not reopen case")
    }
    expect(call(devseed.DeliveryUserID, "DELIVERY_PARTNER", "PATCH", "/partner/account", map[string]string{"name": "Updated courier", "phone": "+919000000044"}), 204)
    if err := auth.SaveRefreshSession(ctx, pool, devseed.DeliveryUserID, "local-refresh-marker"); err != nil {
        t.Fatal(err)
    }
    expect(call(devseed.DeliveryUserID, "DELIVERY_PARTNER", "POST", "/partner/password", map[string]string{"current_password": "wrong", "new_password": "ReplacementPassword!2027"}), 400)
    expect(call(devseed.DeliveryUserID, "DELIVERY_PARTNER", "POST", "/partner/password", map[string]string{"current_password": password, "new_password": "ReplacementPassword!2027"}), 204)
    user, err := auth.FindByEmail(ctx, pool, "delivery@yember.test")
    if err != nil || user.VerifyPassword("ReplacementPassword!2027") != nil {
        t.Fatal("password was not updated")
    }
    var activeSessions int
    if err = pool.QueryRow(ctx, `SELECT count(*) FROM auth.refresh_sessions WHERE user_id=$1 AND revoked_at IS NULL`, devseed.DeliveryUserID).Scan(&activeSessions); err != nil || activeSessions != 0 {
        t.Fatal("password change left refresh sessions active")
    }
}

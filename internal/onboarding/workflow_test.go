package onboarding_test

import (
    "bytes"
    "context"
    "encoding/json"
    "mime/multipart"
    "net/http"
    "net/http/httptest"
    "net/url"
    "strings"
    "testing"

    "github.com/imtiyazkumar/yember-core/config"
    "github.com/imtiyazkumar/yember-core/internal/admin"
    "github.com/imtiyazkumar/yember-core/internal/auth"
    "github.com/imtiyazkumar/yember-core/internal/onboarding"
    "github.com/imtiyazkumar/yember-core/internal/testdb"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

func TestPartnerSignupVerificationUploadAndAdminReview(t *testing.T) {
    pool := testdb.New(t)
    testdb.Station(t, pool)
    ctx := context.Background()
    e := echo.New()
    log := zap.NewNop()
    cfg := &config.Config{JWTSecret: "isolated-test-secret-at-least-32-characters"}
    auth.RegisterRoutes(e, cfg, log, pool)
    api := e.Group("/api/v1")
    onboarding.Register(api, pool, cfg, log)
    admin.RegisterRoutes(api.Group("/admin", auth.JWTMiddleware(cfg.JWTSecret, log, pool), auth.RBACMiddleware("ADMIN")), cfg, log, admin.NewService(admin.NewPostgresRepo(pool, log)))
    t.Setenv("DOCUMENT_STORAGE_DIRECTORY", t.TempDir())
    t.Setenv("EMAIL_VERIFICATION_URL", "https://example.invalid/verify")
    request := func(method, path, token string, body interface{}, expected int) map[string]interface{} {
        t.Helper()
        data, _ := json.Marshal(body)
        req := httptest.NewRequest(method, path, bytes.NewReader(data))
        req.Header.Set("Content-Type", "application/json")
        if token != "" {
            req.Header.Set("Authorization", "Bearer "+token)
        }
        rec := httptest.NewRecorder()
        e.ServeHTTP(rec, req)
        if rec.Code != expected {
            t.Fatalf("%s %s returned %d, expected %d: %s", method, path, rec.Code, expected, rec.Body.String())
        }
        result := map[string]interface{}{}
        _ = json.Unmarshal(rec.Body.Bytes(), &result)
        return result
    }
    request("POST", "/api/v1/auth/partner-signup", "", map[string]string{"email": "owner@test.invalid", "password": "test-password-123", "name": "Owner", "kind": "restaurant"}, 201)
    tokens := request("POST", "/api/v1/auth/login", "", map[string]string{"email": "owner@test.invalid", "password": "test-password-123"}, 200)
    access := tokens["access_token"].(string)
    refresh := tokens["refresh_token"].(string)
    request("GET", "/api/v1/admin/users", access, nil, 403)
    request("POST", "/api/v1/auth/request-verification", access, map[string]string{}, 202)
    var emailBody string
    if err := pool.QueryRow(ctx, `SELECT body FROM notifications.email_outbox WHERE subject='Verify your Yember email'`).Scan(&emailBody); err != nil {
        t.Fatal(err)
    }
    link, err := url.Parse(strings.TrimPrefix(emailBody, "Verify your email within one hour: "))
    if err != nil {
        t.Fatal(err)
    }
    verificationToken := link.Query().Get("token")
    request("POST", "/api/v1/auth/verify-email", "", map[string]string{"token": verificationToken}, 200)
    request("POST", "/api/v1/auth/verify-email", "", map[string]string{"token": verificationToken}, 400)
    var upload bytes.Buffer
    writer := multipart.NewWriter(&upload)
    part, _ := writer.CreateFormFile("file", "license.pdf")
    _, _ = part.Write([]byte("%PDF-1.4\nTest license document\n"))
    _ = writer.WriteField("document_type", "business_license")
    _ = writer.Close()
    req := httptest.NewRequest("POST", "/api/v1/onboarding/documents", &upload)
    req.Header.Set("Content-Type", writer.FormDataContentType())
    req.Header.Set("Authorization", "Bearer "+access)
    rec := httptest.NewRecorder()
    e.ServeHTTP(rec, req)
    if rec.Code != 201 {
        t.Fatalf("upload failed: %s", rec.Body.String())
    }
    var document struct {
        ID string `json:"id"`
    }
    _ = json.Unmarshal(rec.Body.Bytes(), &document)
    application := map[string]interface{}{"latitude": 34, "longitude": 74, "name": "Real Test Kitchen", "phone": "1234567890", "address": "Test address", "description": "Test kitchen", "license_number": "TEST-LICENSE", "cuisine": []string{"Test"}, "document_ids": []string{document.ID}}
    submitted := request("POST", "/api/v1/onboarding/application", access, application, 201)
    request("POST", "/api/v1/onboarding/application", access, application, 400)
    request("GET", "/api/v1/onboarding/documents/"+document.ID, access, nil, 200)
    var adminID string
    if err := pool.QueryRow(ctx, `INSERT INTO public.users(email,password_hash,role) VALUES('admin@test.invalid','hash','ADMIN') RETURNING id`).Scan(&adminID); err != nil {
        t.Fatal(err)
    }
    adminToken, _ := auth.GenerateAccessToken(cfg.JWTSecret, adminID, "ADMIN")
    request("PATCH", "/api/v1/admin/merchants/"+submitted["id"].(string)+"/approval", adminToken, map[string]string{"status": "REJECTED"}, 400)
    request("PATCH", "/api/v1/admin/merchants/"+submitted["id"].(string)+"/approval", adminToken, map[string]string{"status": "APPROVED"}, 200)
    rotated := request("POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": refresh}, 200)
    request("POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": refresh}, 401)
    request("POST", "/api/v1/auth/logout", "", map[string]string{"refresh_token": rotated["refresh_token"].(string)}, 204)
    request("POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": rotated["refresh_token"].(string)}, 401)
    if _, err := pool.Exec(ctx, `UPDATE public.users SET disabled_at=now() WHERE id=$1`, adminID); err != nil {
        t.Fatal(err)
    }
    request(http.MethodGet, "/api/v1/admin/users", adminToken, nil, 401)
}

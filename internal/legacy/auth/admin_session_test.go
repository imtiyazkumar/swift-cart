package auth

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/imtiyazkumar/yember-core/internal/legacy/config"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

func TestAdminCookieSessionRotationAndOriginProtection(t *testing.T) {
	pool := testdb.New(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("test-admin-password"), bcrypt.MinCost)
	_, err := pool.Exec(context.Background(), `INSERT INTO public.users(email,password_hash,role) VALUES('admin@test.invalid',$1,'ADMIN')`, string(hash))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{JWTSecret: "isolated-cookie-session-secret-32-characters", CORSAllowedOrigins: "http://localhost:5173", AdminCookieSecure: true}
	e := echo.New()
	RegisterRoutes(e, cfg, zap.NewNop(), pool)
	login := httptest.NewRequest("POST", "/api/v1/auth/admin/login", strings.NewReader(`{"email":"admin@test.invalid","password":"test-admin-password"}`))
	login.Header.Set("Content-Type", "application/json")
	login.Header.Set("X-Yember-Admin", "1")
	login.Header.Set("Origin", "http://localhost:5173")
	record := httptest.NewRecorder()
	e.ServeHTTP(record, login)
	if record.Code != 200 {
		t.Fatalf("login: %d %s", record.Code, record.Body.String())
	}
	cookies := record.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatal("refresh credential is not protected by secure HttpOnly cookie")
	}
	if strings.Contains(record.Body.String(), "refresh_token") || strings.Contains(record.Body.String(), "password_hash") {
		t.Fatal("browser response exposes a credential")
	}
	original := cookies[0]
	session := httptest.NewRequest("POST", "/api/v1/auth/admin/session", nil)
	session.AddCookie(original)
	session.Header.Set("X-Yember-Admin", "1")
	session.Header.Set("Origin", "http://evil.invalid")
	rejected := httptest.NewRecorder()
	e.ServeHTTP(rejected, session)
	if rejected.Code != 403 {
		t.Fatal("cross-origin refresh allowed")
	}
	session.Header.Set("Origin", "http://localhost:5173")
	rotated := httptest.NewRecorder()
	e.ServeHTTP(rotated, session)
	if rotated.Code != 200 {
		t.Fatalf("restore: %s", rotated.Body.String())
	}
	next := rotated.Result().Cookies()[0]
	if next.Value == original.Value {
		t.Fatal("refresh cookie was not rotated")
	}
	replay := httptest.NewRecorder()
	e.ServeHTTP(replay, session)
	if replay.Code != 401 {
		t.Fatal("consumed refresh cookie accepted")
	}
	logout := httptest.NewRequest("POST", "/api/v1/auth/admin/logout", nil)
	logout.Header.Set("X-Yember-Admin", "1")
	logout.AddCookie(next)
	loggedOut := httptest.NewRecorder()
	e.ServeHTTP(loggedOut, logout)
	if loggedOut.Code != 204 {
		t.Fatal("logout failed")
	}
	restore := httptest.NewRequest("POST", "/api/v1/auth/admin/session", nil)
	restore.Header.Set("X-Yember-Admin", "1")
	restore.AddCookie(next)
	expired := httptest.NewRecorder()
	e.ServeHTTP(expired, restore)
	if expired.Code != 401 {
		t.Fatal("logged-out cookie accepted")
	}
}

package platform

import (
	"context"
	"encoding/json"
	"github.com/imtiyazkumar/yember-core/internal/contractschema"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNumericSchemaAuthAndPublicIdentifiers(t *testing.T) {
	p := testdb.Empty(t)
	ctx := context.Background()
	if _, err := p.Exec(ctx, contractschema.SQL); err != nil {
		t.Fatal(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("AdminPassword!2026"), bcrypt.DefaultCost)
	var adminUUID string
	if err := p.QueryRow(ctx, `INSERT INTO users(email,full_name,password_hash,role,status) VALUES('admin@example.test','Admin',$1,'ADMIN','ACTIVE') RETURNING uuid`, string(hash)).Scan(&adminUUID); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	s := &Service{DB: p, JWTSecret: "a-test-secret-with-more-than-thirty-two-characters"}
	s.Register(e)
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		e.ServeHTTP(w, r)
		return w
	}
	login := call("POST", "/auth/login", `{"email":"admin@example.test","password":"AdminPassword!2026"}`, "")
	if login.Code != 200 {
		t.Fatal(login.Code, login.Body)
	}
	var session struct {
		Access  string   `json:"access_token"`
		Refresh string   `json:"refresh_token"`
		User    identity `json:"user"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.User.UUID != adminUUID || session.User.ID != 0 {
		t.Fatal("numeric internal identity leaked")
	}
	create := call("POST", "/admin/manage/menu-categories", `{"name":"Meals","sort_order":1,"is_active":true,"reason":"Create shared categories"}`, session.Access)
	if create.Code != 200 {
		t.Fatal(create.Code, create.Body)
	}
	var category struct {
		ID string `json:"id"`
	}
	json.Unmarshal(create.Body.Bytes(), &category)
	if publicID(category.ID) != nil {
		t.Fatal("category API did not return a UUID")
	}
	var internalID int64
	if err := p.QueryRow(ctx, `SELECT id FROM menu_categories WHERE uuid=$1`, category.ID).Scan(&internalID); err != nil || internalID < 1 {
		t.Fatal("numeric category key missing", err)
	}
	list := call("GET", "/admin/manage/menu-categories", "", session.Access)
	if list.Code != 200 || !strings.Contains(list.Body.String(), category.ID) {
		t.Fatal(list.Code, list.Body)
	}
	refresh := call("POST", "/auth/refresh", `{"refresh_token":"`+session.Refresh+`"}`, "")
	if refresh.Code != 200 {
		t.Fatal(refresh.Code, refresh.Body)
	}
	replay := call("POST", "/auth/refresh", `{"refresh_token":"`+session.Refresh+`"}`, "")
	if replay.Code != 401 {
		t.Fatal("rotated refresh credential accepted", replay.Code)
	}
	var active int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NULL`).Scan(&active); err != nil || active != 0 {
		t.Fatal("refresh replay did not revoke descendants", err)
	}
	var leaks int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM system_activity_logs WHERE after_json::text LIKE '%password_hash%' OR after_json::text LIKE '%token_hash%'`).Scan(&leaks); err != nil || leaks != 0 {
		t.Fatal("audit exposes credentials", err)
	}
}

package db_test

import (
	"context"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"github.com/imtiyazkumar/yember-core/pkg/db"
	"strings"
	"testing"
)

func TestStartupSchemaRejectsMissingRequestProtection(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := db.CheckSchema(ctx, pool); err != nil {
		t.Fatalf("current schema rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `DROP TABLE auth.rate_windows`); err != nil {
		t.Fatal(err)
	}
	err := db.CheckSchema(ctx, pool)
	if err == nil || !strings.Contains(err.Error(), "go run ./cmd/migrate") || !strings.Contains(err.Error(), "auth.rate_windows") {
		t.Fatalf("missing schema must give actionable startup failure: %v", err)
	}
}

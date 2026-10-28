package db_test

import (
	"context"
	"github.com/imtiyazkumar/yember-core/internal/contractschema"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"github.com/imtiyazkumar/yember-core/pkg/db"
	"strings"
	"testing"
)

func TestStartupRejectsExtraFeatureTables(t *testing.T) {
	p := testdb.Empty(t)
	ctx := context.Background()
	if err := contractschema.Install(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckSchema(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "CREATE TABLE public.unapproved_feature(id bigint)"); err != nil {
		t.Fatal(err)
	}
	err := db.CheckSchema(ctx, p)
	if err == nil || !strings.Contains(err.Error(), "unapproved_feature") || !strings.Contains(err.Error(), "go run ./cmd/migrate") {
		t.Fatal("unexpected table accepted", err)
	}
}

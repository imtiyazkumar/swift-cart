package main

import (
	"context"
	"github.com/imtiyazkumar/yember-core/internal/devseed"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"testing"
)

func TestFinanceAccessGrantAuditRollbackAndRevoke(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := devseed.Seed(ctx, pool, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := provision(ctx, pool, "admin@yember.test", "READ", false); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM finance.permissions WHERE user_id=$1 AND permission='READ'`, devseed.AdminID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("grant not idempotent: %d %v", count, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.admin_audit_logs WHERE action='FINANCE_ACCESS_PROVISIONED' AND target_id=$1`, devseed.AdminID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("missing access audit: %d %v", count, err)
	}
	if err := provision(ctx, pool, "admin@yember.test", "CONFIGURE,INVALID", false); err == nil {
		t.Fatal("invalid permission accepted")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM finance.permissions WHERE user_id=$1 AND permission='CONFIGURE'`, devseed.AdminID).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed grant partially committed", err)
	}
	if err := provision(ctx, pool, "customer@yember.test", "READ", false); err == nil {
		t.Fatal("non-admin granted finance access")
	}
	if err := provision(ctx, pool, "admin@yember.test", "READ", true); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM finance.permissions WHERE user_id=$1`, devseed.AdminID).Scan(&count); err != nil || count != 0 {
		t.Fatal("revocation failed", err)
	}
}

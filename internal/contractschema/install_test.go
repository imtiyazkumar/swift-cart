package contractschema

import (
	"context"
	"strings"
	"testing"

	"github.com/imtiyazkumar/yember-core/internal/legacy/devseed"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
)

func TestInstallExactAndIdempotentWithoutTrackingTable(t *testing.T) {
	p := testdb.Empty(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := Install(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.Exec(ctx, `CREATE TABLE public.unexpected_feature(id bigint)`); err != nil {
		t.Fatal(err)
	}
	if err := Validate(ctx, p); err == nil || !strings.Contains(err.Error(), "unexpected public.unexpected_feature") {
		t.Fatal("extra feature table accepted", err)
	}
}

func TestGroupExistingPopulatedTablesPreservesIdentityAndHistory(t *testing.T) {
	p := testdb.Empty(t)
	ctx := context.Background()
	if _, err := p.Exec(ctx, SQL); err != nil {
		t.Fatal(err)
	}
	var before, after uint32
	var id int64
	var uuid string
	if err := p.QueryRow(ctx, `SELECT 'public.users'::regclass::oid`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO users(full_name,role,status) VALUES('Preserved user','CUSTOMER','ACTIVE') RETURNING id,uuid`).Scan(&id, &uuid); err != nil {
		t.Fatal(err)
	}
	if err := Install(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `SELECT 'users.users'::regclass::oid`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("table was rebuilt instead of moved")
	}
	var retained bool
	var count int
	if err := p.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users.users WHERE id=$1 AND uuid=$2 AND full_name='Preserved user')`, id, uuid).Scan(&retained); err != nil || !retained {
		t.Fatal("user changed", err)
	}
	if err := p.QueryRow(ctx, `SELECT count(*) FROM activity.system_activity_logs WHERE entity_uuid=$1`, uuid).Scan(&count); err != nil || count != 1 {
		t.Fatal("history changed", count, err)
	}
	if _, err := p.Exec(ctx, `UPDATE users SET full_name='Updated after move' WHERE uuid=$1`, uuid); err != nil {
		t.Fatal("trigger failed", err)
	}
	if err := Install(ctx, p); err != nil {
		t.Fatal("repeat migration", err)
	}
	if err := p.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("tables remain in public", count, err)
	}
}
func TestEmptyLegacyReplacementRetainsConfigurationAndOnly38Tables(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if err := ReplaceEmptyLegacy(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := Validate(ctx, p); err != nil {
		t.Fatal(err)
	}
	var history int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM system_activity_logs WHERE action='LEGACY_IMPORT' AND entity_type='public.schema_migrations'`).Scan(&history); err != nil || history < 60 {
		t.Fatal("migration history missing", history, err)
	}
	var id int64
	if err := p.QueryRow(ctx, `INSERT INTO users(full_name,phone,role,status) VALUES('After swap','9000000000','CUSTOMER','ACTIVE') RETURNING id`).Scan(&id); err != nil || id < 1 {
		t.Fatal("new identity or trigger depends on removed schema", err)
	}
	var audit int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM system_activity_logs WHERE entity_type='USERS'`).Scan(&audit); err != nil || audit != 1 {
		t.Fatal("new audit trigger failed after namespace swap", audit, err)
	}
}
func TestLegacyReplacementRefusesAnyExistingApplicationRecords(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if err := devseed.Seed(ctx, p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceEmptyLegacy(ctx, p); err == nil || !strings.Contains(err.Error(), "existing records") {
		t.Fatal("nonempty database was reset", err)
	}
	var retained bool
	if err := p.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.users WHERE id=$1) AND to_regclass('finance.entries') IS NOT NULL AND to_regnamespace('yember_install') IS NULL`, devseed.OwnerID).Scan(&retained); err != nil || !retained {
		t.Fatal("failed replacement changed original data", retained, err)
	}
}

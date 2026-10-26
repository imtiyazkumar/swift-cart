package migrations_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"github.com/imtiyazkumar/yember-core/migrations"
)

func TestHistoricalPlan(t *testing.T) {
	plan, err := migrations.Plan()
	if err != nil {
		t.Fatal(err)
	}
	historical, steps, demo := 0, 0, 0
	for _, m := range plan {
		if len(m.Steps) > 0 {
			historical++
		}
		steps += len(m.Steps)
		if m.Demo {
			demo++
			if m.Filename != "010_seed_data.sql" {
				t.Fatal("incorrect demo selection")
			}
		}
	}
	if historical != 54 || steps != 319 || demo != 1 {
		t.Fatalf("lost SQL blocks or demo flag: %d/%d", steps, demo)
	}
}

func TestRejectDamagedBundle(t *testing.T) {
	original := fstest.MapFS{}
	err := fs.WalkDir(os.DirFS("."), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if name == "manifest.json" || strings.HasSuffix(name, ".sql") {
			b, e := os.ReadFile(name)
			if e != nil {
				return e
			}
			original[name] = &fstest.MapFile{Data: b}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"changed", "missing", "duplicate", "unreferenced", "unmarked"} {
		t.Run(kind, func(t *testing.T) {
			copy := fstest.MapFS{}
			for k, v := range original {
				copy[k] = &fstest.MapFile{Data: append([]byte(nil), v.Data...)}
			}
			switch kind {
			case "changed":
				copy["auth.sql"].Data = []byte(strings.Replace(string(copy["auth.sql"].Data), "CREATE EXTENSION", "DROP EXTENSION", 1))
			case "missing":
				delete(copy, "auth.sql")
			case "duplicate":
				copy["auth.sql"].Data = append(copy["auth.sql"].Data, copy["auth.sql"].Data...)
			case "unreferenced":
				copy["auth.sql"].Data = append(copy["auth.sql"].Data, []byte("-- yember-step: unexpected/001\nSELECT 1;\n-- yember-end-step\n")...)
			case "unmarked":
				copy["auth.sql"].Data = append([]byte("SELECT 1;\n"), copy["auth.sql"].Data...)
			}
			if _, err := migrations.Load(copy); err == nil {
				t.Fatal("accepted corrupted migration bundle")
			}
		})
	}
}

func TestUpgradeAndRepeat(t *testing.T) {
	pool := testdb.Empty(t)
	ctx := context.Background()
	plan, err := migrations.Plan()
	if err != nil {
		t.Fatal(err)
	}
	// Upgrade a tracked pre-finance deployment through all remaining migrations.
	if err = migrations.Apply(ctx, pool, plan[:35], migrations.Options{}); err != nil {
		t.Fatal(err)
	}
	var before string
	if err = pool.QueryRow(ctx, `SELECT md5(string_agg(filename||checksum||applied_at::text,',' ORDER BY filename)) FROM schema_migrations`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err = migrations.Apply(ctx, pool, plan, migrations.Options{}); err != nil {
		t.Fatal(err)
	}
	var after string
	if err = pool.QueryRow(ctx, `SELECT md5(string_agg(filename||checksum||applied_at::text,',' ORDER BY filename)) FROM schema_migrations WHERE filename <= '035_order_events.sql'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("existing migration history changed")
	}
	applied := 0
	opts := migrations.Options{Applied: func(string) { applied++ }}
	if err = migrations.Apply(ctx, pool, plan, opts); err != nil {
		t.Fatal(err)
	}
	if applied != 0 {
		t.Fatal("replayed applied migrations")
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 53 {
		t.Fatalf("expected 53 non-demo migrations, got %d", count)
	}
	// Existing LF/CRLF checksums remain accepted without rewriting their records.
	for _, m := range plan {
		if len(m.LegacyChecksums) > 0 {
			if _, err = pool.Exec(ctx, `UPDATE schema_migrations SET checksum=$2 WHERE filename=$1`, m.Filename, m.LegacyChecksums[0]); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if err = migrations.Apply(ctx, pool, plan, opts); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE schema_migrations SET checksum='invalid' WHERE filename='001_init.sql'`); err != nil {
		t.Fatal(err)
	}
	if err = migrations.Apply(ctx, pool, plan, opts); err == nil {
		t.Fatal("accepted mismatched history")
	}
}

func TestUntrackedAndInvalidSelections(t *testing.T) {
	pool := testdb.Empty(t)
	ctx := context.Background()
	plan, err := migrations.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, plan[0].SQL); err != nil {
		t.Fatal(err)
	}
	if err = migrations.Apply(ctx, pool, plan, migrations.Options{}); err == nil {
		t.Fatal("replayed untracked database")
	}
	for _, opts := range []migrations.Options{{File: "payments.sql"}, {BaselineThrough: "missing.sql"}} {
		if err = migrations.Apply(ctx, pool, plan, opts); err == nil {
			t.Fatal("accepted invalid selection")
		}
	}
	if err = migrations.Apply(ctx, pool, plan, migrations.Options{BaselineThrough: "001_init.sql"}); err != nil {
		t.Fatal(err)
	}
}

func TestTransactionRollback(t *testing.T) {
	pool := testdb.Empty(t)
	ctx := context.Background()
	plan := []migrations.Migration{{Filename: "test.sql", Checksum: "test", SQL: "CREATE TABLE rollback_probe(id int); SELECT 1/0;"}}
	if err := migrations.Apply(ctx, pool, plan, migrations.Options{}); err == nil {
		t.Fatal("expected failure")
	}
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.rollback_probe') IS NOT NULL OR EXISTS(SELECT 1 FROM schema_migrations)`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("failed migration left schema or tracking rows")
	}
}

// This comparison is opt-in because the original SQL is deliberately removed
// from the repository. It verifies a saved pre-consolidation bundle verbatim.
func TestOriginalSQLPreserved(t *testing.T) {
	dir := os.Getenv("YEMBER_ORIGINAL_MIGRATIONS")
	if dir == "" {
		t.Skip("set YEMBER_ORIGINAL_MIGRATIONS to a pre-consolidation snapshot")
	}
	plan, err := migrations.Plan()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range plan {
		old, err := os.ReadFile(filepath.Join(dir, m.Filename))
		if err != nil {
			t.Fatal(err)
		}
		if string(old) != m.SQL {
			t.Fatalf("historical SQL changed: %s", m.Filename)
		}
	}
}

package migrations_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"github.com/imtiyazkumar/yember-core/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOriginalDatabaseEquivalent(t *testing.T) {
	dir := os.Getenv("YEMBER_ORIGINAL_MIGRATIONS")
	if dir == "" {
		t.Skip("requires pre-consolidation snapshot")
	}
	dump := os.Getenv("YEMBER_PG_DUMP")
	if dump == "" {
		t.Skip("set YEMBER_PG_DUMP for full schema comparison")
	}
	for _, demo := range []bool{false, true} {
		t.Run(map[bool]string{false: "production", true: "demo"}[demo], func(t *testing.T) {
			old, newDB := testdb.Empty(t), testdb.Empty(t)
			ctx := context.Background()
			plan, err := migrations.Plan()
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range plan {
				if m.Demo && !demo {
					continue
				}
				sql, err := os.ReadFile(filepath.Join(dir, m.Filename))
				if err != nil {
					t.Fatal(err)
				}
				tx, err := old.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = tx.Exec(ctx, string(sql)); err != nil {
					tx.Rollback(ctx)
					t.Fatal(m.Filename, err)
				}
				if err = tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if err = migrations.Apply(ctx, newDB, plan, migrations.Options{IncludeDemo: demo}); err != nil {
				t.Fatal(err)
			}
			snapshot := func(pool *pgxpool.Pool) string {
				cfg := pool.Config().ConnConfig
				cmd := exec.Command(dump, "--schema-only", "--no-owner", "--no-privileges", "--exclude-table=public.schema_migrations")
				cmd.Env = append(os.Environ(), "PGHOST="+cfg.Host, "PGPORT="+fmtPort(cfg.Port), "PGUSER="+cfg.User, "PGPASSWORD="+cfg.Password, "PGDATABASE="+cfg.Database)
				out, err := cmd.Output()
				if err != nil {
					t.Fatalf("pg_dump failed: %v", err)
				}
				text := strings.ReplaceAll(string(out), "\r\n", "\n")
				return regexp.MustCompile(`(?m)^\\(?:un)?restrict .*\n`).ReplaceAllString(text, "")
			}
			a, b := snapshot(old), snapshot(newDB)
			if a != b {
				t.Fatal("original and reorganized PostgreSQL schema dumps differ")
			}
			// Compare every table's row count, including the optional cross-domain seeds.
			rows, err := old.Query(ctx, `SELECT schemaname,tablename FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema') ORDER BY 1,2`)
			if err != nil {
				t.Fatal(err)
			}
			type table struct{ s, n string }
			var tables []table
			for rows.Next() {
				var item table
				if err = rows.Scan(&item.s, &item.n); err != nil {
					t.Fatal(err)
				}
				tables = append(tables, item)
			}
			if err = rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
			for _, item := range tables {
				query := `SELECT count(*) FROM "` + item.s + `"."` + item.n + `"`
				var n1, n2 int
				if err = old.QueryRow(ctx, query).Scan(&n1); err != nil {
					t.Fatal(err)
				}
				if err = newDB.QueryRow(ctx, query).Scan(&n2); err != nil {
					t.Fatal(err)
				}
				if n1 != n2 {
					t.Fatalf("seed row count differs in %s.%s: %d/%d", item.s, item.n, n1, n2)
				}
			}
		})
	}
}

func fmtPort(port uint16) string { return fmt.Sprint(port) }

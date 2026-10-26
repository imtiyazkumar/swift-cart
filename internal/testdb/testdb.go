// Package testdb creates an isolated disposable database for integration tests.
package testdb

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/imtiyazkumar/yember-core/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

// Station explicitly installs coverage for tests that exercise operational flows.
func Station(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO public.stations(id,name,latitude,longitude,radius_m) VALUES('20000000-0000-4000-a000-000000000001','Test station',34,74,5000) ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
}

func PartnerLocation(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO delivery.live_locations(partner_id,latitude,longitude,accuracy_m,observed_at,permission_granted) VALUES($1,34,74,5,now(),true)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE delivery.delivery_partners SET status='ONLINE' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
}

func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := Empty(t)
	plan, err := migrations.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if err = migrations.Apply(context.Background(), pool, plan, migrations.Options{}); err != nil {
		t.Fatal(err)
	}
	return pool
}

// Empty creates an isolated database without applying a schema, for migration tests.
func Empty(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("YEMBER_INTEGRATION") != "1" {
		t.Skip("set YEMBER_INTEGRATION=1 to create an isolated PostgreSQL test database")
	}
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(source), "../..")
	env, err := godotenv.Read(filepath.Join(root, ".env"))
	if err != nil {
		env = map[string]string{}
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = env["POSTGRES_DSN"]
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("test database DSN is invalid")
	}
	cfg.ConnConfig.Database = "postgres"
	admin, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal("could not connect to PostgreSQL for isolated tests")
	}
	name := "yember_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(context.Background(), "CREATE DATABASE "+quoted); err != nil {
		admin.Close()
		t.Fatalf("could not create isolated test database: %v", err)
	}
	testCfg := cfg.Copy()
	testCfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(context.Background(), testCfg)
	if err != nil {
		t.Fatal("could not open isolated test database")
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+quoted+" WITH (FORCE)")
		admin.Close()
	})
	return pool
}

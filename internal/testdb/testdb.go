// Package testdb creates an isolated disposable database for integration tests.
package testdb

import (
    "context"
    "os"
    "path/filepath"
    "sort"
    "strings"
    "testing"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/joho/godotenv"
)

func New(t *testing.T) *pgxpool.Pool {
    t.Helper()
    if os.Getenv("YEMBER_INTEGRATION") != "1" {
        t.Skip("set YEMBER_INTEGRATION=1 to create an isolated PostgreSQL test database")
    }
    root := "../.."
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
    files, err := filepath.Glob(filepath.Join(root, "migrations", "*.sql"))
    if err != nil {
        t.Fatal(err)
    }
    sort.Strings(files)
    for _, file := range files {
        if filepath.Base(file) == "010_seed_data.sql" {
            continue
        }
        data, err := os.ReadFile(file)
        if err != nil {
            t.Fatal(err)
        }
        tx, err := pool.Begin(context.Background())
        if err != nil {
            t.Fatal(err)
        }
        if _, err = tx.Exec(context.Background(), string(data)); err != nil {
            _ = tx.Rollback(context.Background())
            t.Fatalf("migration %s: %v", filepath.Base(file), err)
        }
        if err = tx.Commit(context.Background()); err != nil {
            t.Fatal(err)
        }
    }
    return pool
}

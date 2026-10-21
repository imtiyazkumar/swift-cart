package main

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "flag"
    "fmt"
    "io/fs"
    "log"
    "path/filepath"
    "sort"
    "strings"

    "os"

    "github.com/imtiyazkumar/yember-core/config"
    "github.com/imtiyazkumar/yember-core/pkg/db"
    "github.com/joho/godotenv"

    "go.uber.org/zap"
)

func main() {
    selectedMigration := flag.String("file", "", "apply only this migration filename from migrations/")
    baselineThrough := flag.String("baseline-through", "", "record existing migrations through this filename without executing them; inspect the existing schema first")
    includeDemo := flag.Bool("include-demo-seed", false, "include demo catalog and users; never enable on production")
    flag.Parse()

    // Load .env file (if present)
    if err := godotenv.Load(); err != nil {
        log.Printf("no .env file loaded: %v", err)
    }
    // Load configuration
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("load config: %v", err)
    }
    // Initialize logger
    logg := zap.NewExample()
    defer logg.Sync()

    // Init DB pool
    pool, err := db.NewPool(cfg)
    if err != nil {
        logg.Sugar().Fatalw("db pool", "error", err)
    }
    defer pool.Close()
    ctx := context.Background()
    var existing bool
    if err = pool.QueryRow(ctx, `SELECT to_regclass('public.users') IS NOT NULL`).Scan(&existing); err != nil {
        log.Fatal(err)
    }
    if _, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations(filename TEXT PRIMARY KEY,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
        log.Fatal(err)
    }
    var tracked int
    if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.schema_migrations`).Scan(&tracked); err != nil {
        log.Fatal(err)
    }
    if existing && tracked == 0 && *baselineThrough == "" && *selectedMigration == "" {
        log.Fatal("existing untracked database: inspect schema and use --baseline-through <last-applied-file>; historical migrations will not be replayed")
    }

    // Find migration files sorted
    migDir := "migrations"
    files, err := fs.ReadDir(os.DirFS("."), migDir)
    if err != nil {
        logg.Sugar().Fatalw("read migrations dir", "error", err)
    }
    var migFiles []string
    for _, f := range files {
        if f.IsDir() {
            continue
        }
        name := f.Name()
        if strings.HasSuffix(name, ".sql") {
            migFiles = append(migFiles, filepath.Join(migDir, name))
        }
    }
    sort.Strings(migFiles)
    if *selectedMigration != "" {
        found := false
        for _, mf := range migFiles {
            if filepath.Base(mf) == filepath.Base(*selectedMigration) {
                migFiles = []string{mf}
                found = true
                break
            }
        }
        if !found {
            logg.Sugar().Fatalw("migration file not found", "file", *selectedMigration)
        }
    }

    // Execute each migration in a transaction
    for _, mf := range migFiles {
        if filepath.Base(mf) == "010_seed_data.sql" && !*includeDemo {
            continue
        }
        content, err := os.ReadFile(mf)
        if err != nil {
            logg.Sugar().Fatalw("read migration", "file", mf, "error", err)
        }
        sql := string(content)
        digest := sha256.Sum256(content)
        checksum := hex.EncodeToString(digest[:])
        logg.Sugar().Infow("apply migration", "file", mf)
        ctx := context.Background()
        tx, err := pool.Begin(ctx)
        if err != nil {
            logg.Sugar().Fatalw("begin tx", "error", err)
        }
        if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(894212)`); err != nil {
            log.Fatal(err)
        }
        var previous string
        if err = tx.QueryRow(ctx, `SELECT COALESCE((SELECT checksum FROM public.schema_migrations WHERE filename=$1),'')`, filepath.Base(mf)).Scan(&previous); err != nil {
            log.Fatal(err)
        }
        if previous != "" {
            _ = tx.Rollback(ctx)
            if previous != checksum {
                log.Fatalf("migration checksum mismatch: %s", filepath.Base(mf))
            }
            continue
        }
        if *baselineThrough != "" && filepath.Base(mf) <= *baselineThrough {
            sql = "SELECT 1"
        }
        _, err = tx.Exec(ctx, sql)
        if err != nil {
            tx.Rollback(ctx)
            logg.Sugar().Fatalw("exec migration", "file", mf, "error", err)
        }
        if _, err = tx.Exec(ctx, `INSERT INTO public.schema_migrations(filename,checksum) VALUES($1,$2)`, filepath.Base(mf), checksum); err != nil {
            _ = tx.Rollback(ctx)
            log.Fatal(err)
        }
        if err = tx.Commit(ctx); err != nil {
            logg.Sugar().Fatalw("commit migration", "file", mf, "error", err)
        }
        fmt.Printf("✅ %s applied\n", mf)
    }
    fmt.Println("All migrations applied successfully")
}

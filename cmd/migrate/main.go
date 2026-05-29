package main

import (
    "context"
    "fmt"
    "io/fs"
    "log"
    "path/filepath"
    "sort"
    "strings"

    "github.com/joho/godotenv"
    "github.com/yourorg/swiftkart/config"
    "github.com/yourorg/swiftkart/pkg/db"
    "os"

    "go.uber.org/zap"
)

func main() {
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

    // Execute each migration in a transaction
    for _, mf := range migFiles {
        content, err := os.ReadFile(mf)
        if err != nil {
            logg.Sugar().Fatalw("read migration", "file", mf, "error", err)
        }
        sql := string(content)
        logg.Sugar().Infow("apply migration", "file", mf)
        ctx := context.Background()
        tx, err := pool.Begin(ctx)
        if err != nil {
            logg.Sugar().Fatalw("begin tx", "error", err)
        }
        _, err = tx.Exec(ctx, sql)
        if err != nil {
            tx.Rollback(ctx)
            logg.Sugar().Fatalw("exec migration", "file", mf, "error", err)
        }
        if err = tx.Commit(ctx); err != nil {
            logg.Sugar().Fatalw("commit migration", "file", mf, "error", err)
        }
        fmt.Printf("✅ %s applied\n", mf)
    }
    fmt.Println("All migrations applied successfully")
}

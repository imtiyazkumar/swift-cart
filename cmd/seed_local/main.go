package main

import (
    "context"
    "log"
    "os"

    "github.com/imtiyazkumar/yember-core/internal/devseed"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/joho/godotenv"
)

func main() {
    _ = godotenv.Load()
    dsn := os.Getenv("POSTGRES_DSN")
    if err := devseed.ValidateTarget(os.Getenv("APP_ENV"), dsn); err != nil {
        log.Fatal(err)
    }
    pool, err := pgxpool.New(context.Background(), dsn)
    if err != nil {
        log.Fatal("could not open local database")
    }
    defer pool.Close()
    if err = devseed.Seed(context.Background(), pool, os.Getenv("LOCAL_SEED_PASSWORD")); err != nil {
        log.Fatal(err)
    }
    log.Print("Local fixtures ready: admin@yember.test, customer@yember.test, restaurant@yember.test, delivery@yember.test. Password is LOCAL_SEED_PASSWORD in the selected Core env file. Existing accounts were not reset.")
}

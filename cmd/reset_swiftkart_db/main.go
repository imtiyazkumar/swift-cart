package main

import (
    "context"
    "log"
    "os"

    "github.com/jackc/pgx/v5/pgxpool"
)

func main() {
    dsn := "postgres://postgres:Imtiyaz%40907@localhost:5432/postgres"
    cfg, err := pgxpool.ParseConfig(dsn)
    if err != nil {
        log.Fatalf("parse dsn: %v", err)
    }
    pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
    if err != nil {
        log.Fatalf("connect pool: %v", err)
    }
    defer pool.Close()

    // Drop if exists (ignore error)
    _, _ = pool.Exec(context.Background(), "DROP DATABASE IF EXISTS swiftkart")
    // Create fresh database
    _, err = pool.Exec(context.Background(), "CREATE DATABASE swiftkart")
    if err != nil {
        log.Fatalf("create database failed: %v", err)
    }
    log.Println("swiftkart database reset successfully")
    os.Exit(0)
}

package main

import (
    "context"
    "log"
    "os"

    "github.com/jackc/pgx/v5/pgxpool"
)

func main() {
    // DSN for the default 'postgres' database
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

    // Create the swiftkart database if it does not exist
    _, err = pool.Exec(context.Background(), "CREATE DATABASE swiftkart")
    if err != nil {
        // If the DB already exists, ignore the error
        if pgErr, ok := err.(interface{ Code() string }); ok && pgErr.Code() == "42P04" {
            log.Printf("database already exists, skipping")
        } else {
            log.Fatalf("create database failed: %v", err)
        }
    } else {
        log.Println("database swiftkart created successfully")
    }
    // Exit
    os.Exit(0)
}

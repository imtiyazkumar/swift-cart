package main

import (
	"context"
	"log"
	"net/url"
	"strings"

	"github.com/imtiyazkumar/yember-core/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	appConfig, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	databaseURL, err := url.Parse(appConfig.PostgresDSN)
	if err != nil {
		log.Fatalf("parse database URL: %v", err)
	}
	databaseName := strings.TrimPrefix(databaseURL.Path, "/")
	if databaseName == "" {
		log.Fatal("database name is missing from POSTGRES_DSN")
	}
	databaseURL.Path = "/postgres"

	cfg, err := pgxpool.ParseConfig(databaseURL.String())
	if err != nil {
		log.Fatalf("parse dsn: %v", err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		log.Fatalf("connect pool: %v", err)
	}
	defer pool.Close()

	var exists bool
	err = pool.QueryRow(context.Background(), "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", databaseName).Scan(&exists)
	if err != nil {
		log.Fatalf("check database: %v", err)
	}
	if exists {
		log.Printf("database %s already exists, skipping", databaseName)
		return
	}

	quotedName := `"` + strings.ReplaceAll(databaseName, `"`, `""`) + `"`
	if _, err = pool.Exec(context.Background(), "CREATE DATABASE "+quotedName); err != nil {
		log.Fatalf("create database %s: %v", databaseName, err)
	}
	log.Printf("database %s created successfully", databaseName)
}

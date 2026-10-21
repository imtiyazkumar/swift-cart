package config

import (
    "os"
    "testing"
)

func TestLoadReadsRequiredPostgresDSNAndDefaults(t *testing.T) {
    // Register restoration, then remove PORT so this test exercises the default
    // even when the test runner loads a local API profile with a custom port.
    t.Setenv("PORT", "")
    if err := os.Unsetenv("PORT"); err != nil {
        t.Fatal(err)
    }
    t.Setenv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/yember?sslmode=disable")
    t.Setenv("JWT_SECRET", "test-secret-with-at-least-32-characters")
    t.Setenv("POSTGRESDSN", "")
    t.Setenv("JWTSECRET", "")

    cfg, err := Load()
    if err != nil {
        t.Fatalf("load config failed: %v", err)
    }
    if cfg.PostgresDSN == "" {
        t.Fatal("expected postgres dsn")
    }
    if cfg.JWTSecret != "test-secret-with-at-least-32-characters" {
        t.Fatal("expected configured JWT secret")
    }
    if cfg.Port != "8080" {
        t.Fatalf("expected default port 8080, got %q", cfg.Port)
    }
}

package config

import "testing"

func TestLoadReadsRequiredPostgresDSNAndDefaults(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/swiftkart?sslmode=disable")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("POSTGRESDSN", "")
	t.Setenv("JWTSECRET", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.PostgresDSN == "" {
		t.Fatal("expected postgres dsn")
	}
	if cfg.JWTSecret != "changeme" {
		t.Fatalf("expected default jwt secret, got %q", cfg.JWTSecret)
	}
	if cfg.Port != "8080" {
		t.Fatalf("expected default port 8080, got %q", cfg.Port)
	}
}

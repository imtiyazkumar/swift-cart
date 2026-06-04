package config

import "testing"

func TestLoadReadsEnvironment(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/swiftkart?sslmode=disable")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("JWT_EXPIRE_MINS", "20")
	t.Setenv("REFRESH_EXPIRE_DAYS", "10")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.Port != "9090" || cfg.LogLevel != "debug" {
		t.Fatalf("unexpected basic config: %+v", cfg)
	}
	if cfg.JwtExpireMins != 20 || cfg.RefreshExpire != 10 {
		t.Fatalf("unexpected token config: %+v", cfg)
	}
}

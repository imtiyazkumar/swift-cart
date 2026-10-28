package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	// PostgreSQL DSN
	PostgresDSN string `envconfig:"POSTGRES_DSN"`
	// JWT secret for signing tokens
	JWTSecret string `envconfig:"JWT_SECRET"`
	// Log level (debug, info, warn, error)
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
	// Port for the HTTP server
	Port               string `envconfig:"PORT" default:"8080"`
	CORSAllowedOrigins string `envconfig:"CORS_ALLOWED_ORIGINS" default:"http://localhost:5173,http://127.0.0.1:5173,http://localhost:8081,http://localhost:8082,http://localhost:8083"`
	AdminCookieSecure  bool   `envconfig:"ADMIN_COOKIE_SECURE" default:"true"`
}

// Load reads environment variables into a Config struct.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	if cfg.PostgresDSN == "" {
		cfg.PostgresDSN = os.Getenv("POSTGRESDSN")
	}
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = os.Getenv("JWTSECRET")
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, errors.New("JWT_SECRET must contain at least 32 characters")
	}
	if cfg.PostgresDSN == "" {
		return nil, errors.New("POSTGRES_DSN is required")
	}
	if os.Getenv("ENVIRONMENT") == "production" {
		if !cfg.AdminCookieSecure {
			return nil, errors.New("production requires secure admin cookies")
		}
		if strings.Contains(cfg.CORSAllowedOrigins, "localhost") || strings.Contains(cfg.CORSAllowedOrigins, "*") {
			return nil, errors.New("production CORS must specify deployed origins")
		}
		if !strings.Contains(cfg.PostgresDSN, "sslmode=verify-full") {
			return nil, errors.New("production database requires sslmode=verify-full")
		}
		if os.Getenv("FINANCE_BANK_KEYS") == "" || os.Getenv("FINANCE_BANK_ACTIVE_KEY") == "" {
			return nil, errors.New("production bank encryption keys are required")
		}
		var keys map[string]string
		if json.Unmarshal([]byte(os.Getenv("FINANCE_BANK_KEYS")), &keys) != nil {
			return nil, errors.New("invalid production bank key ring")
		}
		key, keyErr := base64.StdEncoding.DecodeString(keys[os.Getenv("FINANCE_BANK_ACTIVE_KEY")])
		if keyErr != nil || len(key) != 32 {
			return nil, errors.New("active production bank key must be 32 bytes")
		}
		if len(os.Getenv("OTP_HASH_SECRET")) < 32 {
			return nil, errors.New("production OTP_HASH_SECRET must contain at least 32 characters")
		}
		if os.Getenv("TEXTBEE_DELIVERY_ENABLED") != "true" || os.Getenv("TEXTBEE_API_KEY") == "" {
			return nil, errors.New("production TextBee phone verification must be configured")
		}
	}
	return &cfg, nil
}

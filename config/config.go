package config

import (
	"errors"
	"os"

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
	Port string `envconfig:"PORT" default:"8080"`
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
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = "changeme"
	}
	if cfg.PostgresDSN == "" {
		return nil, errors.New("POSTGRES_DSN is required")
	}
	return &cfg, nil
}

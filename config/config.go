package config

import (
    "github.com/kelseyhightower/envconfig"
)

type Config struct {
    // PostgreSQL DSN
    PostgresDSN string `envconfig:"POSTGRESDSN" required:"true"`
    // Redis address (e.g., localhost:6379)
    RedisAddr string `envconfig:"REDIS_ADDR" default:"localhost:6379"`
    // JWT secret for signing tokens
    JWTSecret string `envconfig:"JWTSECRET" default:"changeme"`
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
    return &cfg, nil
}

package config

import (
    "github.com/kelseyhightower/envconfig"
)

type Config struct {
    Port          string `env:"PORT" default:"8080"`
    LogLevel      string `env:"LOG_LEVEL" default:"info"`
    PostgresDSN   string `env:"POSTGRES_DSN" required:"true"`
    RedisAddr     string `env:"REDIS_ADDR" default:"localhost:6379"`
    JwtSecret     string `env:"JWT_SECRET" required:"true"`
    JwtExpireMins int    `env:"JWT_EXPIRE_MINS" default:"15"`
    RefreshExpire int    `env:"REFRESH_EXPIRE_DAYS" default:"30"`
}

func Load() (*Config, error) {
    var cfg Config
    if err := envconfig.Process("", &cfg); err != nil {
        return nil, err
    }
    return &cfg, nil
}

package config

import (
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Port          string `envconfig:"PORT" default:"8080"`
	LogLevel      string `envconfig:"LOG_LEVEL" default:"info"`
	PostgresDSN   string `envconfig:"POSTGRES_DSN" required:"true"`
	JwtSecret     string `envconfig:"JWT_SECRET" required:"true"`
	JwtExpireMins int    `envconfig:"JWT_EXPIRE_MINS" default:"15"`
	RefreshExpire int    `envconfig:"REFRESH_EXPIRE_DAYS" default:"30"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

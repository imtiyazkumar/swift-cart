package db

import (
    "context"
    "fmt"
    "time"
    "github.com/yourorg/swiftkart/config"
    "github.com/jackc/pgx/v5/pgxpool"
    "go.uber.org/zap"
)

type PostgresDB struct {
    Pool *pgxpool.Pool
    log  *zap.Logger
}

// NewPool creates a pgx connection pool using the provided configuration.
func NewPool(cfg *config.Config) (*pgxpool.Pool, error) {
    poolConfig, err := pgxpool.ParseConfig(cfg.PostgresDSN)
    if err != nil {
        return nil, err
    }
    return pgxpool.New(context.Background(), poolConfig.ConnString())
}

func NewPostgres(dsn string) (*PostgresDB, error) {
    cfg, err := pgxpool.ParseConfig(dsn)
    if err != nil {
        return nil, fmt.Errorf("pgx parse dsn: %w", err)
    }
    // Enable automatic health check connections
    cfg.HealthCheckPeriod = time.Second * 10 // enable health checks
    pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
    if err != nil {
        return nil, fmt.Errorf("pgx pool create: %w", err)
    }
    return &PostgresDB{Pool: pool, log: zap.NewNop()}, nil
}

func (db *PostgresDB) SetLogger(l *zap.Logger) {
    db.log = l
}

func (db *PostgresDB) Ping(ctx context.Context) error {
    return db.Pool.Ping(ctx)
}

func (db *PostgresDB) Close() {
    db.Pool.Close()
}

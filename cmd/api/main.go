package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/labstack/echo/v4"
    "github.com/labstack/echo/v4/middleware"

    "github.com/yourorg/swiftkart/config"
    "github.com/yourorg/swiftkart/pkg/logger"
    "github.com/yourorg/swiftkart/pkg/db"
    "github.com/yourorg/swiftkart/pkg/redis"
    "github.com/yourorg/swiftkart/pkg/middleware"
)

func main() {
    // Load configuration
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("failed to load config: %v", err)
    }

    // Initialize logger
    logg := logger.New(cfg.LogLevel)
    defer logg.Sync()

    // Initialize DB
    pg, err := db.NewPostgres(cfg.PostgresDSN)
    if err != nil {
        logg.Sugar().Fatalw("db init", "error", err)
    }
    defer pg.Close()

    // Initialize Redis
    rdb, err := redis.NewClient(cfg.RedisAddr)
    if err != nil {
        logg.Sugar().Fatalw("redis init", "error", err)
    }
    defer rdb.Close()

    // Echo instance
    e := echo.New()
    e.HideBanner = true
    e.Logger = logger.NewEchoLogger(logg)

    // Middleware
    e.Use(middleware.Recover())
    e.Use(middleware.RequestID())
    e.Use(middleware.Logger())
    e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{Timeout: 30 * time.Second}))
    e.Use(custommiddleware.RequestLogger(logg))
    e.Use(custommiddleware.ErrorHandler(logg))

    // Health endpoints
    e.GET("/healthz", healthHandler)
    e.GET("/readyz", readyHandler(pg, rdb))

    // API version group (placeholder)
    api := e.Group("/api/v1")
    _ = api // future routes will be attached here

    // Start server with graceful shutdown
    go func() {
        addr := ":" + cfg.Port
        if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
            logg.Sugar().Fatalw("server start", "error", err)
        }
    }()
    logg.Sugar().Infow("server started", "addr", ":"+cfg.Port)

    // Wait for interrupt signal
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
    <-quit
    logg.Info("shutdown initiated")

    // Context with timeout for shutdown
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := e.Shutdown(ctx); err != nil {
        logg.Sugar().Errorw("shutdown error", "error", err)
    }
    logg.Info("server gracefully stopped")
}

func healthHandler(c echo.Context) error {
    return c.String(http.StatusOK, "OK")
}

func readyHandler(pg *db.PostgresDB, rdb *redis.Client) echo.HandlerFunc {
    return func(c echo.Context) error {
        if err := pg.Ping(c.Request().Context()); err != nil {
            return c.String(http.StatusServiceUnavailable, "DB not ready")
        }
        if err := rdb.Ping(c.Request().Context()).Err(); err != nil {
            return c.String(http.StatusServiceUnavailable, "Redis not ready")
        }
        return c.String(http.StatusOK, "READY")
    }
}

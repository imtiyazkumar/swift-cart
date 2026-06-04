package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/imtiyazkumar/swiftkart/config"
	"github.com/imtiyazkumar/swiftkart/internal/auth"
	"github.com/imtiyazkumar/swiftkart/internal/catalog"
	"github.com/imtiyazkumar/swiftkart/internal/customer"
	"github.com/imtiyazkumar/swiftkart/internal/delivery"
	"github.com/imtiyazkumar/swiftkart/internal/merchant"
	"github.com/imtiyazkumar/swiftkart/internal/notifications"
	"github.com/imtiyazkumar/swiftkart/internal/order"
	"github.com/imtiyazkumar/swiftkart/internal/payments"
	"github.com/imtiyazkumar/swiftkart/pkg/db"
	"github.com/imtiyazkumar/swiftkart/pkg/events"
	"github.com/imtiyazkumar/swiftkart/pkg/logger"
	custommiddleware "github.com/imtiyazkumar/swiftkart/pkg/middleware"
)

func main() {
	_ = godotenv.Load()

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

	// Echo instance
	e := echo.New()
	e.HideBanner = true
	e.Logger = logger.NewEchoLogger(logg)

	// Middleware
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderAuthorization, echo.HeaderContentType, echo.HeaderXRequestID},
	}))
	e.Use(middleware.Secure())
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{Timeout: 30 * time.Second}))
	e.Use(custommiddleware.RequestLogger(logg))
	// Rate limiting (60 requests per minute per IP)
	e.Use(custommiddleware.NewInMemoryRateLimiter(custommiddleware.RateLimiterConfig{Limit: 60, Period: time.Minute}))

	// Health endpoints
	e.GET("/healthz", healthHandler)
	e.GET("/readyz", readyHandler(pg))

	// API version group (placeholder)
	api := e.Group("/api/v1")
	// Register Auth routes
	auth.RegisterRoutes(e, cfg, logg, pg.Pool)

	// Initialise services
	eventBus := events.NewInMemoryBus()
	merchantSvc := merchant.NewService(merchant.NewPostgresRepo(pg.Pool, logg))
	customerSvc := customer.NewService(customer.NewPostgresRepo(pg.Pool, logg))
	orderSvc := order.NewService(order.NewPostgresRepo(pg.Pool, logg))
	catalogSvc := catalog.NewService(catalog.NewPostgresRepo(pg.Pool, logg))
	deliverySvc := delivery.NewService(delivery.NewPostgresRepo(pg.Pool, logg))
	paymentSvc := payments.NewService(payments.NewPostgresRepo(pg.Pool, logg))
	notificationSvc := notifications.NewService(notifications.NewPostgresRepo(pg.Pool, logg), eventBus)
	notificationSvc.SubscribeOrderEvents()

	// Register Merchant routes
	merchantGroup := api.Group("/merchant")
	merchant.RegisterRoutes(merchantGroup, cfg, logg, merchantSvc)

	// Register Customer routes
	customerGroup := api.Group("/customer")
	customer.RegisterRoutes(customerGroup, cfg, logg, customerSvc)

	// Register Order routes
	orderGroup := api.Group("/order")
	order.RegisterRoutes(orderGroup, cfg, logg, orderSvc)

	catalog.RegisterRoutes(api.Group("/catalog"), cfg, logg, catalogSvc)
	delivery.RegisterRoutes(api.Group("/delivery"), deliverySvc)
	payments.RegisterRoutes(api.Group("/payments"), paymentSvc)

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

func readyHandler(pg *db.PostgresDB) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := pg.Ping(c.Request().Context()); err != nil {
			return c.String(http.StatusServiceUnavailable, "DB not ready")
		}
		return c.String(http.StatusOK, "READY")
	}
}

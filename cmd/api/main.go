package main

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/imtiyazkumar/yember-core/config"
	"github.com/imtiyazkumar/yember-core/internal/contractschema"
	"github.com/imtiyazkumar/yember-core/internal/platform"
	"github.com/imtiyazkumar/yember-core/pkg/db"
	"github.com/imtiyazkumar/yember-core/pkg/logger"
	custommiddleware "github.com/imtiyazkumar/yember-core/pkg/middleware"
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
	schemaContext, cancelSchemaCheck := context.WithTimeout(context.Background(), 10*time.Second)
	err = contractschema.Validate(schemaContext, pg.Pool)
	cancelSchemaCheck()
	if err != nil {
		logg.Sugar().Fatalw("database startup check failed", "error", err)
	}

	// Echo instance
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		code := 500
		message := "internal server error"
		if he, ok := err.(*echo.HTTPError); ok {
			code = he.Code
			if code < 500 {
				message = fmt.Sprint(he.Message)
			}
		}
		if strings.Contains(message, "SQLSTATE") {
			message = "request could not be processed"
		}
		if code >= 500 {
			logg.Warn("request failed", zap.Error(err), zap.String("request_id", c.Response().Header().Get(echo.HeaderXRequestID)))
		}
		if !c.Response().Committed {
			_ = c.JSON(code, map[string]string{"message": message, "request_id": c.Response().Header().Get(echo.HeaderXRequestID)})
		}
	}

	e.IPExtractor = echo.ExtractIPDirect()
	if os.Getenv("TRUST_PROXY_CIDRS") != "" {
		options := []echo.TrustOption{echo.TrustLoopback(false), echo.TrustLinkLocal(false), echo.TrustPrivateNet(false)}
		for _, value := range strings.Split(os.Getenv("TRUST_PROXY_CIDRS"), ",") {
			_, network, parseErr := net.ParseCIDR(strings.TrimSpace(value))
			if parseErr != nil {
				log.Fatalf("invalid trusted proxy network")
			}
			options = append(options, echo.TrustIPRange(network))
		}
		e.IPExtractor = echo.ExtractIPFromXFFHeader(options...)
	}
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.IdleTimeout = 60 * time.Second
	e.Server.MaxHeaderBytes = 32 << 10
	e.Logger = logger.NewEchoLogger(logg)

	// Global Middleware
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit("12M"))
	e.Use(custommiddleware.RequestID())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     strings.Split(cfg.CORSAllowedOrigins, ","),
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{echo.HeaderAuthorization, echo.HeaderContentType, echo.HeaderXRequestID, "X-Yember-Admin"},
		AllowCredentials: true,
	}))
	e.Use(middleware.Secure())
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{Timeout: 30 * time.Second}))
	e.Use(custommiddleware.RequestLogger(logg))
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(20)))

	// Health & Status endpoints
	e.GET("/", homeHandler)
	e.GET("/healthz", healthHandler)
	e.GET("/readyz", readyHandler(pg))

	// All public routes use the document schema. No legacy handlers, workers or
	// SQL-backed limiter can recreate or depend on removed tables.
	service := &platform.Service{DB: pg.Pool, JWTSecret: cfg.JWTSecret, CookieSecure: cfg.AdminCookieSecure, AllowedOrigins: strings.Split(cfg.CORSAllowedOrigins, ",")}
	service.Gateway = platform.GatewayFromEnv()
	service.SendOTP = platform.SendOTPFromEnv()
	service.Register(e)

	// Start server with graceful shutdown
	go func() {
		addr := ":" + cfg.Port
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			logg.Sugar().Fatalw("server start", "error", err)
		}
	}()
	logg.Sugar().Infow("Yember Core server started", "addr", ":"+cfg.Port)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	logg.Info("shutdown initiated")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		logg.Sugar().Errorw("shutdown error", "error", err)
	}
	logg.Info("Yember Core server gracefully stopped")
}

func homeHandler(c echo.Context) error {
	return c.HTML(http.StatusOK, homePageHTML)
}

const homePageHTML = `<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <meta name="theme-color" content="#f3f6ef">
    <meta name="description" content="Yember Core API platform service status">
    <title>Yember Core API | Platform Service</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=DM+Mono:wght@400;500&family=Manrope:wght@400;500;600;700;800&display=swap" rel="stylesheet">
    <style>
        :root {
            color-scheme: light;
            --paper: #f3f6ef;
            --ink: #192d27;
            --muted: #68766f;
            --green: #174c3a;
            --green-light: #d8efdf;
            --lime: #d6f36a;
            --coral: #e9785c;
            --line: rgba(25, 45, 39, .15);
            --sans: 'Manrope', 'Avenir Next', sans-serif;
            --mono: 'DM Mono', Consolas, monospace;
        }
        * { box-sizing: border-box; }
        body {
            margin: 0;
            color: var(--ink);
            background: var(--paper);
            background-image: linear-gradient(rgba(25, 45, 39, .035) 1px, transparent 1px), linear-gradient(90deg, rgba(25, 45, 39, .035) 1px, transparent 1px);
            background-size: 36px 36px;
            font-family: var(--sans);
            -webkit-font-smoothing: antialiased;
        }
        a { color: inherit; }
        .frame { width: min(1120px, calc(100% - 64px)); margin: 0 auto; }
        header {
            height: 78px;
            display: flex;
            align-items: center;
            justify-content: space-between;
            border-bottom: 1px solid var(--line);
        }
        .brand { display: flex; align-items: center; gap: 11px; text-decoration: none; font-size: 16px; font-weight: 800; }
        .brand-mark {
            width: 32px;
            height: 32px;
            display: grid;
            place-items: center;
            background: var(--green);
            color: var(--lime);
            font-size: 17px;
            line-height: 1;
        }
        .brand-name span { color: var(--green); }
        .header-meta, .eyebrow, .label, .mono { font-family: var(--mono); text-transform: uppercase; letter-spacing: 0; }
        .header-meta { display: flex; align-items: center; gap: 9px; color: var(--muted); font-size: 11px; }
        .header-dot, .status-dot { width: 8px; height: 8px; border-radius: 50%; background: #398454; box-shadow: 0 0 0 4px rgba(57, 132, 84, .13); }
        main { padding: 86px 0 76px; }
        .hero { display: grid; grid-template-columns: 1.08fr .92fr; gap: 72px; align-items: center; }
        .hero-copy { animation: arrive .65s ease-out both; }
        .eyebrow { display: flex; align-items: center; gap: 10px; color: var(--green); font-size: 11px; }
        .eyebrow::before { width: 24px; height: 2px; background: var(--coral); content: ''; }
        h1 { max-width: 600px; margin: 24px 0 18px; font-size: 62px; line-height: 1.02; font-weight: 700; letter-spacing: 0; }
        h1 span { color: var(--green); }
        .intro { max-width: 450px; margin: 0; color: var(--muted); font-size: 16px; line-height: 1.8; }
        .actions { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 30px; }
        .action {
            min-height: 44px;
            display: inline-flex;
            align-items: center;
            gap: 10px;
            padding: 0 16px;
            border: 1px solid var(--green);
            background: var(--green);
            color: white;
            font-size: 12px;
            font-weight: 700;
            text-decoration: none;
            transition: background .18s ease, color .18s ease, transform .18s ease;
        }
        .action:hover { transform: translateY(-2px); background: var(--lime); color: var(--ink); }
        .action.secondary { background: transparent; color: var(--ink); }
        .action.secondary:hover { background: var(--green-light); }
        .arrow { color: var(--coral); font-size: 16px; }
        .status-panel {
            position: relative;
            min-height: 320px;
            overflow: hidden;
            padding: 28px;
            background: var(--green);
            color: white;
            animation: arrive .65s .12s ease-out both;
        }
        .status-panel::after {
            position: absolute;
            right: -62px;
            bottom: -88px;
            width: 220px;
            height: 220px;
            border: 1px solid rgba(214, 243, 106, .25);
            box-shadow: 0 0 0 24px rgba(214, 243, 106, .035), 0 0 0 48px rgba(214, 243, 106, .025);
            content: '';
            transform: rotate(45deg);
        }
        .panel-top { display: flex; align-items: center; justify-content: space-between; padding-bottom: 23px; border-bottom: 1px solid rgba(255,255,255,.2); }
        .label { color: rgba(255,255,255,.68); font-size: 10px; }
        .panel-code { color: var(--lime); font-family: var(--mono); font-size: 11px; }
        .status-main { display: flex; align-items: center; gap: 12px; margin-top: 30px; }
        .status-main .status-dot { width: 10px; height: 10px; background: var(--lime); box-shadow: 0 0 0 5px rgba(214,243,106,.16); }
        .status-main strong { font-size: 21px; font-weight: 600; }
        .status-caption { margin: 10px 0 27px 27px; color: rgba(255,255,255,.65); font-family: var(--mono); font-size: 11px; }
        .flow { position: relative; z-index: 1; display: grid; grid-template-columns: 1fr 14px 1fr 14px 1fr 14px 1fr; align-items: center; }
        .flow-node { min-height: 58px; display: flex; flex-direction: column; justify-content: center; gap: 5px; padding: 7px; border: 1px solid rgba(255,255,255,.23); background: rgba(255,255,255,.04); }
        .flow-node strong { font-family: var(--mono); font-size: 9px; font-weight: 400; }
        .flow-node span { color: var(--lime); font-family: var(--mono); font-size: 8px; }
        .flow-line { height: 1px; background: rgba(214,243,106,.65); }
        .details { display: grid; grid-template-columns: repeat(4, 1fr); margin-top: 72px; border-top: 1px solid var(--line); border-bottom: 1px solid var(--line); }
        .detail { padding: 21px 24px 22px 0; }
        .detail + .detail { padding-left: 24px; border-left: 1px solid var(--line); }
        .detail .label { color: var(--muted); font-size: 10px; }
        .detail strong { display: block; margin-top: 10px; font-family: var(--mono); font-size: 13px; font-weight: 500; }
        @keyframes arrive { from { opacity: 0; transform: translateY(10px); } to { opacity: 1; transform: translateY(0); } }
    </style>
</head>
<body>
    <div class="frame">
        <header>
            <a class="brand" href="/" aria-label="Yember Core API home">
                <span class="brand-mark" aria-hidden="true">Y</span>
                <span class="brand-name">yember<span>core</span></span>
            </a>
            <div class="header-meta"><span class="header-dot" aria-hidden="true"></span> SHARED BACKEND PLATFORM / ONLINE</div>
        </header>

        <main>
            <section class="hero" aria-labelledby="page-title">
                <div class="hero-copy">
                    <div class="eyebrow">Yember Platform <span>/</span> 01</div>
                    <h1 id="page-title">Yember Core<br><span>is online.</span></h1>
                    <p class="intro">Central backend platform supporting Customer, Rider, Restaurant, and Administration applications.</p>
                    <div class="actions">
                        <a class="action" href="/readyz">Check readiness <span class="arrow" aria-hidden="true">↗</span></a>
                        <a class="action secondary" href="/healthz">Health check <span class="arrow" aria-hidden="true">↗</span></a>
                    </div>
                </div>

                <aside class="status-panel" aria-label="Service status overview">
                    <div class="panel-top"><span class="label">Platform Engine</span><span class="panel-code">STATUS / 200</span></div>
                    <div class="status-main"><span class="status-dot" aria-hidden="true"></span><strong>All Applications Connected</strong></div>
                    <p class="status-caption">Serving multi-app requests</p>
                    <div class="flow" aria-label="Client applications to Yember Core flow">
                        <div class="flow-node"><strong>CUSTOMER</strong><span>READY</span></div>
                        <span class="flow-line" aria-hidden="true"></span>
                        <div class="flow-node"><strong>RIDER</strong><span>READY</span></div>
                        <span class="flow-line" aria-hidden="true"></span>
                        <div class="flow-node"><strong>RESTAURANT</strong><span>READY</span></div>
                        <span class="flow-line" aria-hidden="true"></span>
                        <div class="flow-node"><strong>ADMIN</strong><span>READY</span></div>
                    </div>
                </aside>
            </section>

            <section class="details" aria-label="API Base Paths">
                <div class="detail"><span class="label">Customer API</span><strong>/api/v1/customer</strong></div>
                <div class="detail"><span class="label">Rider API</span><strong>/api/v1/rider</strong></div>
                <div class="detail"><span class="label">Restaurant API</span><strong>/api/v1/restaurant</strong></div>
                <div class="detail"><span class="label">Admin API</span><strong>/api/v1/admin</strong></div>
            </section>
        </main>
    </div>
</body>
</html>`

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

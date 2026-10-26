package fees

import (
    "github.com/imtiyazkumar/yember-core/config"
    "github.com/imtiyazkumar/yember-core/internal/auth"
"github.com/imtiyazkumar/yember-core/internal/finance"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
    "io"
    "net/http"
    "os"
    "time"
)

func Register(api *echo.Group, pool *pgxpool.Pool, cfg *config.Config, log *zap.Logger) {
    service := New(pool)
    admin := api.Group("/admin/settings", auth.JWTMiddleware(cfg.JWTSecret, log, pool), auth.RBACMiddleware("ADMIN"))
    admin.GET("/onboarding-fee", func(c echo.Context) error {
        policy, err := service.Policy(c.Request().Context())
        if err != nil {
            return echo.NewHTTPError(404, err.Error())
        }
        return c.JSON(200, policy)
    })
    admin.PUT("/onboarding-fee", func(c echo.Context) error {
        var policy Policy
        if err := c.Bind(&policy); err != nil {
            return echo.NewHTTPError(400, "invalid policy")
        }
        if err := service.SetPolicy(c.Request().Context(), c.Get("userID").(string), policy); err != nil {
            return echo.NewHTTPError(400, err.Error())
        }
        return c.JSON(200, policy)
    })
    api.POST("/restaurant/onboarding-payment", func(c echo.Context) error {
        if os.Getenv("ONBOARDING_PAYMENT_PROVIDER")=="RAZORPAY"{result,err:=finance.New(pool).OnboardingCheckout(c.Request().Context(),c.Get("userID").(string));if err!=nil{return echo.NewHTTPError(409,err.Error())};return c.JSON(200,result)}
        checkout, err := service.CreateCheckout(c.Request().Context(), c.Get("userID").(string))
        if err != nil {
            return echo.NewHTTPError(503, err.Error())
        }
        return c.JSON(200, checkout)
    }, auth.JWTMiddleware(cfg.JWTSecret, log, pool), auth.RBACMiddleware("MERCHANT_OWNER"))
    api.POST("/webhooks/stripe", func(c echo.Context) error {
        body, err := io.ReadAll(http.MaxBytesReader(c.Response(), c.Request().Body, 1<<20))
        if err != nil {
            return echo.NewHTTPError(400, "invalid webhook body")
        }
        if !VerifySignature(body, c.Request().Header.Get("Stripe-Signature"), os.Getenv("STRIPE_WEBHOOK_SECRET"), time.Now()) {
            return echo.NewHTTPError(400, "invalid webhook signature")
        }
        if err = service.ApplyEvent(c.Request().Context(), body); err != nil {
            log.Warn("payment event rejected", zap.Error(err))
            return echo.NewHTTPError(409, "payment event could not be applied")
        }
        return c.NoContent(200)
    })
}

package admin

import (
    "context"
    "encoding/json"
    "errors"
    "github.com/imtiyazkumar/yember-core/config"
    "os"
    "strconv"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/labstack/echo/v4"
)

type consoleRepository interface {
    ConsoleList(context.Context, string, ListFilter) (*Page, error)
    ConsoleDetail(context.Context, string, string) (map[string]interface{}, error)
    ConsoleEditProfile(context.Context, string, string, string, ProfileEdit) error
    ConsoleDisable(context.Context, string, string, bool, string) error
    ConsoleAvailability(context.Context, string, string, string, bool, string) error
    ConsoleMenu(context.Context, string, string, string, MenuEdit) error
    ConsoleCoupon(context.Context, string, string, CouponInput, string) error
    ConsoleOperation(context.Context, string, string, string, bool, string) error
    ConsoleOverview(context.Context) (json.RawMessage, error)
}

func registerConsole(g *echo.Group, svc *Service, cfg *config.Config) {
    repo, ok := svc.repo.(consoleRepository)
    if !ok {
        return
    }
    g.GET("/console/integrations", func(c echo.Context) error {
        backend := os.Getenv("DOCUMENT_STORAGE_BACKEND")
        if backend == "" {
            backend = "local"
        }
        storageConfigured := backend == "local" && os.Getenv("DOCUMENT_STORAGE_DIRECTORY") != "" || backend == "s3" && os.Getenv("DOCUMENT_S3_BUCKET") != ""
        c.Response().Header().Set("Cache-Control", "no-store")
        return c.JSON(200, map[string]interface{}{"email_configured": cfg.ResendAPIKey != "" && cfg.EmailFrom != "", "email_delivery_enabled": cfg.EmailDeliveryEnabled, "payment_configured": os.Getenv("STRIPE_SECRET_KEY") != "" && os.Getenv("STRIPE_WEBHOOK_SECRET") != "" && os.Getenv("PAYMENT_SUCCESS_URL") != "" && os.Getenv("PAYMENT_CANCEL_URL") != "", "storage_backend": backend, "storage_configured": storageConfigured, "admin_cookie_secure": cfg.AdminCookieSecure, "customer_payment_method": os.Getenv("CUSTOMER_PAYMENT_METHOD")})
    })
    g.GET("/console/overview", func(c echo.Context) error {
        data, err := repo.ConsoleOverview(c.Request().Context())
        if err != nil {
            return echo.NewHTTPError(500, "could not load operations overview")
        }
        return c.JSON(200, data)
    })
    group := g.Group("/manage")
    group.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            if _, ok := consoleViews[c.Param("resource")]; !ok {
                return echo.NewHTTPError(404, "management resource not found")
            }
            if id := c.Param("id"); id != "" {
                if _, err := uuid.Parse(id); err != nil {
                    return echo.NewHTTPError(400, "invalid record identifier")
                }
            }
            return next(c)
        }
    })
    failure := func(err error) error {
        if errors.Is(err, pgx.ErrNoRows) {
            return echo.NewHTTPError(404, "record not found")
        }
        return echo.NewHTTPError(400, err.Error())
    }
    group.GET("/:resource", func(c echo.Context) error {
        page, _ := strconv.Atoi(c.QueryParam("page"))
        limit, _ := strconv.Atoi(c.QueryParam("limit"))
        result, err := repo.ConsoleList(c.Request().Context(), c.Param("resource"), ListFilter{StationID: c.QueryParam("station_id"), Search: c.QueryParam("q"), Status: c.QueryParam("status"), Role: c.QueryParam("role"), Page: page, Limit: limit})
        if err != nil {
            return echo.NewHTTPError(500, "could not load records").SetInternal(err)
        }
        return c.JSON(200, result)
    })
    group.GET("/:resource/:id", func(c echo.Context) error {
        result, err := repo.ConsoleDetail(c.Request().Context(), c.Param("resource"), c.Param("id"))
        if err != nil {
            if errors.Is(err, pgx.ErrNoRows) {
                return echo.NewHTTPError(404, "record not found")
            }
            return echo.NewHTTPError(500, "could not load record").SetInternal(err)
        }
        return c.JSON(200, result)
    })
    group.PATCH("/:resource/:id/profile", func(c echo.Context) error {
        var input ProfileEdit
        if err := c.Bind(&input); err != nil {
            return echo.NewHTTPError(400, "invalid profile")
        }
        if err := repo.ConsoleEditProfile(c.Request().Context(), c.Get("userID").(string), c.Param("resource"), c.Param("id"), input); err != nil {
            return failure(err)
        }
        return c.NoContent(204)
    })
    group.POST("/:resource/:id/status", func(c echo.Context) error {
        if c.Param("resource") != "users" && c.Param("resource") != "customers" {
            return echo.NewHTTPError(400, "account status applies to users and customers")
        }
        var input struct {
            Disabled bool   `json:"disabled"`
            Reason   string `json:"reason"`
        }
        if err := c.Bind(&input); err != nil {
            return echo.NewHTTPError(400, "invalid account status")
        }
        if err := repo.ConsoleDisable(c.Request().Context(), c.Get("userID").(string), c.Param("id"), input.Disabled, input.Reason); err != nil {
            return failure(err)
        }
        return c.NoContent(204)
    })
    group.POST("/:resource/:id/availability", func(c echo.Context) error {
        var input struct {
            Available bool   `json:"available"`
            Reason    string `json:"reason"`
        }
        if err := c.Bind(&input); err != nil {
            return echo.NewHTTPError(400, "invalid availability")
        }
        if err := repo.ConsoleAvailability(c.Request().Context(), c.Get("userID").(string), c.Param("resource"), c.Param("id"), input.Available, input.Reason); err != nil {
            return failure(err)
        }
        return c.NoContent(204)
    })
    group.PUT("/:resource/:id/items/:item", func(c echo.Context) error {
        if c.Param("resource") != "restaurants" {
            return echo.NewHTTPError(400, "restaurant required")
        }
        if _, err := uuid.Parse(c.Param("item")); err != nil {
            return echo.NewHTTPError(400, "invalid item ID")
        }
        var input MenuEdit
        if err := c.Bind(&input); err != nil {
            return echo.NewHTTPError(400, "invalid menu item")
        }
        if err := repo.ConsoleMenu(c.Request().Context(), c.Get("userID").(string), c.Param("id"), c.Param("item"), input); err != nil {
            return failure(err)
        }
        return c.NoContent(204)
    })
    group.POST("/:resource", func(c echo.Context) error {
        if c.Param("resource") != "coupons" {
            return echo.NewHTTPError(405, "create this record through its registration workflow")
        }
        var input struct {
            CouponInput
            Reason string `json:"reason"`
        }
        if err := c.Bind(&input); err != nil {
            return echo.NewHTTPError(400, "invalid coupon")
        }
        if err := repo.ConsoleCoupon(c.Request().Context(), c.Get("userID").(string), "", input.CouponInput, input.Reason); err != nil {
            return failure(err)
        }
        return c.NoContent(201)
    })
    group.PUT("/:resource/:id", func(c echo.Context) error {
        if c.Param("resource") != "coupons" {
            return echo.NewHTTPError(405, "unsupported update")
        }
        var input struct {
            CouponInput
            Reason string `json:"reason"`
        }
        if err := c.Bind(&input); err != nil {
            return echo.NewHTTPError(400, "invalid coupon")
        }
        if err := repo.ConsoleCoupon(c.Request().Context(), c.Get("userID").(string), c.Param("id"), input.CouponInput, input.Reason); err != nil {
            return failure(err)
        }
        return c.NoContent(204)
    })
    group.POST("/:resource/:id/operation", func(c echo.Context) error {
        var input struct {
            Enabled bool   `json:"enabled"`
            Reason  string `json:"reason"`
        }
        if err := c.Bind(&input); err != nil {
            return echo.NewHTTPError(400, "invalid operation")
        }
        if err := repo.ConsoleOperation(c.Request().Context(), c.Get("userID").(string), c.Param("resource"), c.Param("id"), input.Enabled, input.Reason); err != nil {
            return failure(err)
        }
        return c.NoContent(204)
    })
}

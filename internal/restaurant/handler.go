package restaurant

import (
    "context"
    "net/http"

    "github.com/imtiyazkumar/yember-core/config"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

func RegisterRoutes(g *echo.Group, cfg *config.Config, logg *zap.Logger, svc *Service) {
    g.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            profile, err := svc.GetProfileByOwner(c.Request().Context(), c.Get("userID").(string))
            if err != nil {
                return echo.NewHTTPError(404, "restaurant profile not found")
            }
            if c.Request().Method != "GET" && (profile.ApprovalStatus != "APPROVED" || profile.FeePaymentStatus != "PAID") {
                return echo.NewHTTPError(403, "approval and verified onboarding fee payment are required")
            }
            c.Set("merchantID", profile.ID)
            c.SetRequest(c.Request().WithContext(context.WithValue(c.Request().Context(), merchantContextKey{}, profile.ID)))
            return next(c)
        }
    })
    registerSettings(g, svc)
    g.GET("/menu/items", func(c echo.Context) error {
        repo, ok := svc.repo.(interface {
            ListMenuItems(context.Context, string) ([]MenuItemInput, error)
        })
        if !ok {
            return echo.NewHTTPError(503, "menu repository unavailable")
        }
        items, err := repo.ListMenuItems(c.Request().Context(), c.Get("merchantID").(string))
        if err != nil {
            return echo.NewHTTPError(500, "could not load menu items")
        }
        return c.JSON(200, items)
    })
    // Restaurant profile
    g.GET("/profile", func(c echo.Context) error {
        ownerID, _ := c.Get("userID").(string)
        if ownerID == "" {
            ownerID = c.QueryParam("owner_id")
        }
        profile, err := svc.GetProfileByOwner(c.Request().Context(), ownerID)
        if err != nil {
            return echo.NewHTTPError(http.StatusNotFound, "restaurant profile not found")
        }
        return c.JSON(http.StatusOK, profile)
    })

    // Toggle store open/closed
    g.PATCH("/status", func(c echo.Context) error {
        var req struct {
            MerchantID string `json:"merchant_id"`
            IsActive   bool   `json:"is_active"`
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        if err := svc.UpdateStatus(c.Request().Context(), c.Get("merchantID").(string), req.IsActive); err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, map[string]interface{}{"is_active": req.IsActive})
    })

    // Categories
    g.GET("/menu/categories", func(c echo.Context) error {
        merchantID := c.Get("merchantID").(string)
        cats, err := svc.ListCategories(c.Request().Context(), merchantID)
        if err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, cats)
    })

    g.POST("/menu/categories", func(c echo.Context) error {
        var cat MenuCategory
        if err := c.Bind(&cat); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        cat.MerchantID = c.Get("merchantID").(string)
        if err := svc.CreateCategory(c.Request().Context(), &cat); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, err.Error())
        }
        return c.JSON(http.StatusCreated, cat)
    })

    // Items
    g.POST("/menu/items", func(c echo.Context) error {
        var item MenuItemInput
        if err := c.Bind(&item); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        item.MerchantID = c.Get("merchantID").(string)
        id, err := svc.SaveMenuItem(c.Request().Context(), &item)
        if err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, err.Error())
        }
        item.ID = id
        return c.JSON(http.StatusCreated, item)
    })

    g.PATCH("/menu/items/:id/availability", func(c echo.Context) error {
        itemID := c.Param("id")
        var req struct {
            IsAvailable bool `json:"is_available"`
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        if err := svc.ToggleItemAvailability(c.Request().Context(), itemID, req.IsAvailable); err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, map[string]interface{}{"item_id": itemID, "is_available": req.IsAvailable})
    })

    // Store Order Dashboard
    g.GET("/orders", func(c echo.Context) error {
        merchantID := c.Get("merchantID").(string)
        orders, err := svc.GetIncomingOrders(c.Request().Context(), merchantID)
        if err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, orders)
    })

    g.PATCH("/orders/:id/status", func(c echo.Context) error {
        orderID := c.Param("id")
        var req struct {
            MerchantID string `json:"merchant_id"`
            Status     string `json:"status"`
            Reason     string `json:"reason"`
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        ctx := context.WithValue(c.Request().Context(), orderReasonContextKey{}, req.Reason)
        if err := svc.UpdateOrderStatus(ctx, c.Get("merchantID").(string), orderID, req.Status); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, err.Error())
        }
        return c.JSON(http.StatusOK, map[string]string{"order_id": orderID, "status": req.Status})
    })

    // Daily Sales Summary
    g.GET("/analytics/daily", func(c echo.Context) error {
        merchantID := c.Get("merchantID").(string)
        summary, err := svc.GetDailySummary(c.Request().Context(), merchantID)
        if err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, summary)
    })
}

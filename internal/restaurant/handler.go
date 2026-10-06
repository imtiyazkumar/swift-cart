package restaurant

import (
    "net/http"

    "github.com/imtiyazkumar/yember-core/config"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

func RegisterRoutes(g *echo.Group, cfg *config.Config, logg *zap.Logger, svc *Service) {
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
        if err := svc.UpdateStatus(c.Request().Context(), req.MerchantID, req.IsActive); err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, map[string]interface{}{"is_active": req.IsActive})
    })

    // Categories
    g.GET("/menu/categories", func(c echo.Context) error {
        merchantID := c.QueryParam("merchant_id")
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
        merchantID := c.QueryParam("merchant_id")
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
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        if err := svc.UpdateOrderStatus(c.Request().Context(), req.MerchantID, orderID, req.Status); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, err.Error())
        }
        return c.JSON(http.StatusOK, map[string]string{"order_id": orderID, "status": req.Status})
    })

    // Daily Sales Summary
    g.GET("/analytics/daily", func(c echo.Context) error {
        merchantID := c.QueryParam("merchant_id")
        summary, err := svc.GetDailySummary(c.Request().Context(), merchantID)
        if err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, summary)
    })
}

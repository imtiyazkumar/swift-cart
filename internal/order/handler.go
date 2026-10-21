package order

import (
    "net/http"

    "github.com/imtiyazkumar/yember-core/config"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

// RegisterRoutes wires order endpoints.
func RegisterRoutes(g *echo.Group, cfg *config.Config, logg *zap.Logger, svc *Service) {
    g.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            if c.Request().Method != http.MethodGet {
                return echo.NewHTTPError(http.StatusMethodNotAllowed, "use customer checkout and audited admin order operations")
            }
            return next(c)
        }
    })
    // Create order
    g.POST("/create", func(c echo.Context) error {
        if role, _ := c.Get("role").(string); role != "ADMIN" {
            return echo.NewHTTPError(http.StatusForbidden, "use customer checkout to create an order")
        }
        var req Order
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        created, err := svc.CreateOrder(c.Request().Context(), &req)
        if err != nil {
            logg.Error("create order failed", zap.Error(err))
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusCreated, created)
    })
    // Get order by ID
    g.GET("/:id", func(c echo.Context) error {
        id := c.Param("id")
        o, err := svc.GetOrder(c.Request().Context(), id)
        if err != nil {
            logg.Error("get order failed", zap.Error(err))
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        if o == nil {
            return echo.NewHTTPError(http.StatusNotFound, "order not found")
        }
        role, _ := c.Get("role").(string)
        if role == "CUSTOMER" {
            uid, _ := c.Get("userID").(string)
            if o.CustomerID != uid {
                return echo.NewHTTPError(http.StatusNotFound, "order not found")
            }
        } else if role != "ADMIN" {
            return echo.NewHTTPError(http.StatusForbidden, "insufficient role")
        }
        return c.JSON(http.StatusOK, o)
    })
    // List orders
    g.GET("/", func(c echo.Context) error {
        if role, _ := c.Get("role").(string); role != "ADMIN" {
            return echo.NewHTTPError(http.StatusForbidden, "admin role required")
        }
        list, err := svc.ListOrders(c.Request().Context())
        if err != nil {
            logg.Error("list orders failed", zap.Error(err))
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, list)
    })
    // Update order status
    g.PUT("/:id/status", func(c echo.Context) error {
        if role, _ := c.Get("role").(string); role != "ADMIN" {
            return echo.NewHTTPError(http.StatusForbidden, "admin role required")
        }
        id := c.Param("id")
        var payload struct {
            Status string `json:"status"`
        }
        if err := c.Bind(&payload); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        if err := svc.UpdateOrderStatus(c.Request().Context(), id, payload.Status); err != nil {
            logg.Error("update order status failed", zap.Error(err))
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.NoContent(http.StatusNoContent)
    })
    // Delete order
    g.DELETE("/:id", func(c echo.Context) error {
        if role, _ := c.Get("role").(string); role != "ADMIN" {
            return echo.NewHTTPError(http.StatusForbidden, "admin role required")
        }
        id := c.Param("id")
        if err := svc.DeleteOrder(c.Request().Context(), id); err != nil {
            logg.Error("delete order failed", zap.Error(err))
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.NoContent(http.StatusNoContent)
    })
}

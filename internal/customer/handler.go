package customer

import (
    "net/http"
    "github.com/labstack/echo/v4"
    "github.com/yourorg/swiftkart/config"
    "go.uber.org/zap"
)

// RegisterRoutes wires the customer endpoints onto the provided Echo group.
func RegisterRoutes(g *echo.Group, cfg *config.Config, logg *zap.Logger, svc *Service) {
    // Create a new customer
    g.POST("/create", func(c echo.Context) error {
        var req Customer
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        created, err := svc.CreateCustomer(c.Request().Context(), &req)
        if err != nil {
            logg.Error("create customer failed", zap.Error(err))
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusCreated, created)
    })

    // Get customer by ID
    g.GET("/:id", func(c echo.Context) error {
        id := c.Param("id")
        cust, err := svc.GetCustomer(c.Request().Context(), id)
        if err != nil {
            logg.Error("get customer failed", zap.Error(err))
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        if cust == nil {
            return echo.NewHTTPError(http.StatusNotFound, "customer not found")
        }
        return c.JSON(http.StatusOK, cust)
    })

    // List customers
    g.GET("/", func(c echo.Context) error {
        list, err := svc.ListCustomers(c.Request().Context())
        if err != nil {
            logg.Error("list customers failed", zap.Error(err))
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, list)
    })

    // Update customer
    g.PUT("/:id", func(c echo.Context) error {
        id := c.Param("id")
        var req Customer
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        req.ID = id
        if err := svc.UpdateCustomer(c.Request().Context(), &req); err != nil {
            logg.Error("update customer failed", zap.Error(err))
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.NoContent(http.StatusNoContent)
    })

    // Delete customer
    g.DELETE("/:id", func(c echo.Context) error {
        id := c.Param("id")
        if err := svc.DeleteCustomer(c.Request().Context(), id); err != nil {
            logg.Error("delete customer failed", zap.Error(err))
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.NoContent(http.StatusNoContent)
    })
}

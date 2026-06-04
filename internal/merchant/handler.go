package merchant

import (
	"net/http"

	"github.com/imtiyazkumar/swiftkart/config"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// RegisterRoutes wires the merchant endpoints onto the provided Echo group.
func RegisterRoutes(g *echo.Group, cfg *config.Config, logg *zap.Logger, svc *Service) {
	// Create a new merchant
	g.POST("/create", func(c echo.Context) error {
		var req Merchant
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		created, err := svc.CreateMerchant(c.Request().Context(), &req)
		if err != nil {
			logg.Error("create merchant failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusCreated, created)
	})
	// Get merchant by ID
	g.GET("/:id", func(c echo.Context) error {
		id := c.Param("id")
		m, err := svc.GetMerchant(c.Request().Context(), id)
		if err != nil {
			logg.Error("get merchant failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if m == nil {
			return echo.NewHTTPError(http.StatusNotFound, "merchant not found")
		}
		return c.JSON(http.StatusOK, m)
	})
	// List merchants
	g.GET("/", func(c echo.Context) error {
		list, err := svc.ListMerchants(c.Request().Context())
		if err != nil {
			logg.Error("list merchants failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, list)
	})
	// Update merchant
	g.PUT("/:id", func(c echo.Context) error {
		id := c.Param("id")
		var req Merchant
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		req.ID = id
		if err := svc.UpdateMerchant(c.Request().Context(), &req); err != nil {
			logg.Error("update merchant failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.NoContent(http.StatusNoContent)
	})
	// Delete merchant
	g.DELETE("/:id", func(c echo.Context) error {
		id := c.Param("id")
		if err := svc.DeleteMerchant(c.Request().Context(), id); err != nil {
			logg.Error("delete merchant failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.NoContent(http.StatusNoContent)
	})
}

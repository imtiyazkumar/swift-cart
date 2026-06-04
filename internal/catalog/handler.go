package catalog

import (
	"net/http"
	"strconv"

	"github.com/imtiyazkumar/swiftkart/config"
	"github.com/imtiyazkumar/swiftkart/internal/common"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func RegisterRoutes(g *echo.Group, cfg *config.Config, logg *zap.Logger, svc *Service) {
	g.POST("/categories", func(c echo.Context) error {
		var req Category
		if err := c.Bind(&req); err != nil {
			return common.BadRequest("invalid request body")
		}
		category, err := svc.CreateCategory(c.Request().Context(), &req)
		if err != nil {
			return common.BadRequest(err.Error())
		}
		return c.JSON(http.StatusCreated, category)
	})

	g.POST("/items", func(c echo.Context) error {
		var req Item
		if err := c.Bind(&req); err != nil {
			return common.BadRequest("invalid request body")
		}
		item, err := svc.CreateItem(c.Request().Context(), &req)
		if err != nil {
			logg.Error("create item failed", zap.Error(err))
			return common.BadRequest(err.Error())
		}
		return c.JSON(http.StatusCreated, item)
	})

	g.GET("/items/:id", func(c echo.Context) error {
		item, err := svc.GetItem(c.Request().Context(), c.Param("id"))
		if err != nil {
			return common.Internal("could not fetch item")
		}
		if item == nil {
			return common.NotFound("item not found")
		}
		return c.JSON(http.StatusOK, item)
	})

	g.GET("/merchants/:merchant_id/items", func(c echo.Context) error {
		items, err := svc.ListMerchantItems(c.Request().Context(), c.Param("merchant_id"))
		if err != nil {
			return common.BadRequest(err.Error())
		}
		return c.JSON(http.StatusOK, items)
	})

	g.GET("/items", func(c echo.Context) error {
		limit, _ := strconv.Atoi(c.QueryParam("limit"))
		items, err := svc.SearchItems(c.Request().Context(), c.QueryParam("q"), limit)
		if err != nil {
			return common.Internal("could not search items")
		}
		return c.JSON(http.StatusOK, items)
	})

	g.PUT("/items/:id", func(c echo.Context) error {
		var req Item
		if err := c.Bind(&req); err != nil {
			return common.BadRequest("invalid request body")
		}
		req.ID = c.Param("id")
		if err := svc.UpdateItem(c.Request().Context(), &req); err != nil {
			return common.BadRequest(err.Error())
		}
		return c.NoContent(http.StatusNoContent)
	})
}

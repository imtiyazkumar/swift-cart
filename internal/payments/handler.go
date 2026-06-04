package payments

import (
	"net/http"

	"github.com/imtiyazkumar/swiftkart/internal/common"
	"github.com/labstack/echo/v4"
)

func RegisterRoutes(g *echo.Group, svc *Service) {
	g.POST("/intents", func(c echo.Context) error {
		var req Intent
		if err := c.Bind(&req); err != nil {
			return common.BadRequest("invalid request body")
		}
		intent, err := svc.CreateIntent(c.Request().Context(), &req)
		if err != nil {
			return common.BadRequest(err.Error())
		}
		return c.JSON(http.StatusCreated, intent)
	})
}

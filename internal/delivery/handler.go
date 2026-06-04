package delivery

import (
	"net/http"

	"github.com/imtiyazkumar/swiftkart/internal/common"
	"github.com/labstack/echo/v4"
)

func RegisterRoutes(g *echo.Group, svc *Service) {
	g.POST("/partners/:id/online", func(c echo.Context) error {
		if err := svc.GoOnline(c.Request().Context(), c.Param("id")); err != nil {
			return common.BadRequest(err.Error())
		}
		return c.NoContent(http.StatusNoContent)
	})
	g.POST("/partners/:id/offline", func(c echo.Context) error {
		if err := svc.GoOffline(c.Request().Context(), c.Param("id")); err != nil {
			return common.BadRequest(err.Error())
		}
		return c.NoContent(http.StatusNoContent)
	})
	g.POST("/locations", func(c echo.Context) error {
		var req LocationUpdate
		if err := c.Bind(&req); err != nil {
			return common.BadRequest("invalid request body")
		}
		if err := svc.UpdateLocation(c.Request().Context(), req); err != nil {
			return common.BadRequest(err.Error())
		}
		return c.NoContent(http.StatusAccepted)
	})
	g.POST("/assignments", func(c echo.Context) error {
		var req struct {
			OrderID   string `json:"order_id"`
			PartnerID string `json:"partner_id"`
		}
		if err := c.Bind(&req); err != nil {
			return common.BadRequest("invalid request body")
		}
		assignment, err := svc.AssignOrder(c.Request().Context(), req.OrderID, req.PartnerID)
		if err != nil {
			return common.BadRequest(err.Error())
		}
		return c.JSON(http.StatusCreated, assignment)
	})
}

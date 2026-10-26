package checkout

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func RegisterRoutes(g *echo.Group, svc *Service) {
	g.PUT("/cart/delivery-address", func(c echo.Context) error {
		var req struct {
			AddressID string `json:"address_id"`
		}
		if c.Bind(&req) != nil || req.AddressID == "" {
			return echo.NewHTTPError(400, "delivery address required")
		}
		station, cleared, err := svc.SelectAddress(c.Request().Context(), c.Get("userID").(string), req.AddressID)
		if err != nil {
			return echo.NewHTTPError(400, err.Error())
		}
		return c.JSON(200, map[string]any{"station_id": station, "cart_cleared": cleared})
	})
	g.GET("/orders/:id", func(c echo.Context) error {
		detail, err := svc.Detail(c.Request().Context(), c.Get("userID").(string), c.Param("id"))
		if err != nil {
			return echo.NewHTTPError(404, "order not found")
		}
		return c.JSON(200, detail)
	})
	g.GET("/orders", func(c echo.Context) error {
		uid, _ := c.Get("userID").(string)
		orders, err := svc.ListOrders(c.Request().Context(), uid)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "could not load orders")
		}
		return c.JSON(http.StatusOK, orders)
	})
	g.GET("/cart", func(c echo.Context) error {
		uid, _ := c.Get("userID").(string)
		cart, err := svc.Get(c.Request().Context(), uid)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "could not load cart")
		}
		return c.JSON(http.StatusOK, cart)
	})
	g.POST("/cart/items", func(c echo.Context) error {
		var req struct {
			AddressID string `json:"address_id"`
			ItemID    string `json:"item_id"`
			Quantity  int    `json:"quantity"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		uid, _ := c.Get("userID").(string)
		if err := svc.Add(c.Request().Context(), uid, req.ItemID, req.Quantity, req.AddressID); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		cart, err := svc.Get(c.Request().Context(), uid)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "could not load cart")
		}
		return c.JSON(http.StatusOK, cart)
	})
	g.PATCH("/cart/items/:id", func(c echo.Context) error {
		var req struct {
			Quantity int `json:"quantity"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		uid, _ := c.Get("userID").(string)
		if err := svc.SetQuantity(c.Request().Context(), uid, c.Param("id"), req.Quantity); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return c.NoContent(http.StatusNoContent)
	})
	g.DELETE("/cart", func(c echo.Context) error {
		uid, _ := c.Get("userID").(string)
		if err := svc.Clear(c.Request().Context(), uid); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "could not clear cart")
		}
		return c.NoContent(http.StatusNoContent)
	})
	g.POST("/checkout", func(c echo.Context) error {
		var req struct {
			AddressID      string `json:"address_id"`
			IdempotencyKey string `json:"idempotency_key"`
			PaymentMethod  string `json:"payment_method"`
 Coupon string `json:"coupon_code"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		uid, _ := c.Get("userID").(string)
		result, err := svc.Checkout(c.Request().Context(), uid, req.AddressID, req.IdempotencyKey, req.PaymentMethod, req.Coupon)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return c.JSON(http.StatusCreated, result)
	})
}

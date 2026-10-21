package customer

import (
	"net/http"
	"strconv"

	"github.com/imtiyazkumar/yember-core/config"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// RegisterRoutes wires customer endpoints onto the provided Echo group.
func RegisterRoutes(g *echo.Group, cfg *config.Config, logg *zap.Logger, svc *Service) {
	// List restaurants for customer discovery
	g.GET("/restaurants", func(c echo.Context) error {
		city := c.QueryParam("city")
		search := c.QueryParam("search")
		list, err := svc.ListRestaurants(c.Request().Context(), city, search)
		if err != nil {
			logg.Error("list restaurants failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, list)
	})

	// Get specific restaurant with menu
	g.GET("/restaurants/:id", func(c echo.Context) error {
		id := c.Param("id")
		rest, err := svc.GetRestaurant(c.Request().Context(), id)
		if err != nil {
			logg.Error("get restaurant failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusNotFound, "restaurant not found")
		}
		return c.JSON(http.StatusOK, rest)
	})

	// Validate coupon code
	g.POST("/coupons/validate", func(c echo.Context) error {
		var req struct {
			Code   string `json:"code"`
			Amount int64  `json:"amount"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		coupon, discount, err := svc.ValidateCoupon(c.Request().Context(), req.Code, req.Amount)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return c.JSON(http.StatusOK, map[string]interface{}{
			"valid":        true,
			"coupon":       coupon,
			"discount":     discount,
			"final_amount": req.Amount - discount,
		})
	})

	// Favorites management
	g.GET("/favorites", func(c echo.Context) error {
		customerID, _ := c.Get("userID").(string)
		if customerID == "" {
			return c.JSON(http.StatusOK, []string{"wazwan", "dalview"})
		}
		favs, err := svc.GetFavorites(c.Request().Context(), customerID)
		if err != nil {
			return c.JSON(http.StatusOK, []string{})
		}
		return c.JSON(http.StatusOK, favs)
	})

	g.POST("/favorites/toggle", func(c echo.Context) error {
		var req struct {
			CustomerID string `json:"customer_id"`
			MerchantID string `json:"merchant_id"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		req.CustomerID, _ = c.Get("userID").(string)
		isFav, err := svc.ToggleFavorite(c.Request().Context(), req.CustomerID, req.MerchantID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, map[string]interface{}{
			"merchant_id": req.MerchantID,
			"is_favorite": isFav,
		})
	})

	// Track active order
	g.GET("/orders/:id/track", func(c echo.Context) error {
		orderID := c.Param("id")
		customerID, _ := c.Get("userID").(string)
		track, err := svc.GetOrderTracking(c.Request().Context(), customerID, orderID)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "order tracking unavailable")
		}
		return c.JSON(http.StatusOK, track)
	})

	// Submit review & rating
	g.POST("/reviews", func(c echo.Context) error {
		var req ReviewRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		customerID, _ := c.Get("userID").(string)
		if err := svc.SubmitReview(c.Request().Context(), customerID, &req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return c.JSON(http.StatusCreated, map[string]string{"message": "review submitted successfully"})
	})

	// Profile Management
	g.POST("/create", func(c echo.Context) error {
		var req Customer
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		req.ID, _ = c.Get("userID").(string)
		req.ID, _ = c.Get("userID").(string)
		if err := svc.UpdateCustomer(c.Request().Context(), &req); err != nil {
			logg.Error("update customer profile failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return c.JSON(http.StatusOK, req)
	})

	g.GET("/profile/:id", func(c echo.Context) error {
		id := c.Param("id")
		userID, _ := c.Get("userID").(string)
		if id != userID {
			return echo.NewHTTPError(http.StatusForbidden, "cannot access another customer profile")
		}
		cust, err := svc.GetCustomer(c.Request().Context(), id)
		if err != nil || cust == nil {
			return echo.NewHTTPError(http.StatusNotFound, "customer profile not found")
		}
		return c.JSON(http.StatusOK, cust)
	})

	g.PUT("/profile", func(c echo.Context) error {
		var req Customer
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		req.ID, _ = c.Get("userID").(string)
		if req.ID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "customer ID is required")
		}
		if err := svc.UpdateCustomer(c.Request().Context(), &req); err != nil {
			logg.Error("update customer failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "message": "profile updated successfully"})
	})

	g.POST("/addresses", func(c echo.Context) error {
		var req struct {
			CustomerID   string `json:"customer_id"`
			Label        string `json:"label"`
			AddressLine1 string `json:"address_line1"`
			City         string `json:"city"`
			State        string `json:"state"`
			PostalCode   string `json:"postal_code"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		req.CustomerID, _ = c.Get("userID").(string)
		if req.CustomerID == "" || req.AddressLine1 == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "customer_id and address_line1 are required")
		}
		if req.Label == "" {
			req.Label = "Home"
		}
		if req.City == "" || req.State == "" || req.PostalCode == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "city, state, and postal_code are required")
		}
		if err := svc.AddAddress(c.Request().Context(), req.CustomerID, req.Label, req.AddressLine1, req.City, req.State, req.PostalCode); err != nil {
			logg.Error("add address failed", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusCreated, map[string]interface{}{"success": true, "message": "address saved successfully"})
	})

	g.GET("/addresses", func(c echo.Context) error {
		customerID, _ := c.Get("userID").(string)
		list, err := svc.ListAddresses(c.Request().Context(), customerID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "could not load addresses")
		}
		return c.JSON(http.StatusOK, list)
	})

	// Customer notifications log
	g.GET("/notifications", func(c echo.Context) error {
		customerID, _ := c.Get("userID").(string)
		list, err := svc.GetNotifications(c.Request().Context(), customerID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, list)
	})
}

func parseMinAmount(s string) int64 {
	val, _ := strconv.ParseInt(s, 10, 64)
	return val
}

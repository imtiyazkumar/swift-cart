package admin

import (
    "net/http"

    "github.com/imtiyazkumar/yember-core/config"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

func RegisterRoutes(g *echo.Group, cfg *config.Config, logg *zap.Logger, svc *Service) {
    // Platform stats
    g.GET("/dashboard/stats", func(c echo.Context) error {
        stats, err := svc.GetDashboardStats(c.Request().Context())
        if err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, stats)
    })

    // User management
    g.GET("/users", func(c echo.Context) error {
        users, err := svc.ListUsers(c.Request().Context())
        if err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, users)
    })

    g.PATCH("/users/:id/role", func(c echo.Context) error {
        userID := c.Param("id")
        var req struct {
            Role string `json:"role"`
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        if err := svc.UpdateUserRole(c.Request().Context(), userID, req.Role); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, err.Error())
        }
        return c.JSON(http.StatusOK, map[string]string{"user_id": userID, "role": req.Role})
    })

    // Merchant Approvals
    g.GET("/merchants/pending", func(c echo.Context) error {
        list, err := svc.ListPendingMerchants(c.Request().Context())
        if err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, list)
    })

    g.PATCH("/merchants/:id/approval", func(c echo.Context) error {
        merchantID := c.Param("id")
        var req struct {
            Status string `json:"status"` // APPROVED, REJECTED
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        if err := svc.ApproveMerchant(c.Request().Context(), merchantID, req.Status); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, err.Error())
        }
        return c.JSON(http.StatusOK, map[string]string{"merchant_id": merchantID, "approval_status": req.Status})
    })

    // Rider Approvals
    g.GET("/riders/pending", func(c echo.Context) error {
        list, err := svc.ListPendingRiders(c.Request().Context())
        if err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, list)
    })

    g.PATCH("/riders/:id/approval", func(c echo.Context) error {
        partnerID := c.Param("id")
        var req struct {
            Status string `json:"status"` // APPROVED, REJECTED
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        if err := svc.ApproveRider(c.Request().Context(), partnerID, req.Status); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, err.Error())
        }
        return c.JSON(http.StatusOK, map[string]string{"partner_id": partnerID, "approval_status": req.Status})
    })

    // Coupon Management
    g.GET("/coupons", func(c echo.Context) error {
        coupons, err := svc.ListCoupons(c.Request().Context())
        if err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, coupons)
    })

    g.POST("/coupons", func(c echo.Context) error {
        var req CouponInput
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        if err := svc.CreateCoupon(c.Request().Context(), &req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, err.Error())
        }
        return c.JSON(http.StatusCreated, req)
    })

    g.DELETE("/coupons/:id", func(c echo.Context) error {
        id := c.Param("id")
        if err := svc.DeleteCoupon(c.Request().Context(), id); err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.NoContent(http.StatusNoContent)
    })
}

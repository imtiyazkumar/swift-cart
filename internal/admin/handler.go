package admin

import (
    "net/http"

    "github.com/imtiyazkumar/yember-core/config"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

func RegisterRoutes(g *echo.Group, cfg *config.Config, logg *zap.Logger, svc *Service) {
    registerConsole(g, svc, cfg)
    g.GET("/orders/:id/timeline", func(c echo.Context) error {
        repo, err := svc.operations()
        if err != nil {
            return echo.NewHTTPError(503, "operations unavailable")
        }
        data, err := repo.Timeline(c.Request().Context(), c.Param("id"))
        if err != nil {
            return echo.NewHTTPError(500, "could not load timeline")
        }
        return c.JSON(200, data)
    })
    g.POST("/orders/:id/cancel", func(c echo.Context) error {
        var req struct {
            Reason string `json:"reason"`
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(400, "invalid request")
        }
        repo, err := svc.operations()
        if err != nil {
            return echo.NewHTTPError(503, "operations unavailable")
        }
        if err = repo.Intervene(c.Request().Context(), c.Get("userID").(string), c.Param("id"), req.Reason); err != nil {
            return echo.NewHTTPError(409, err.Error())
        }
        return c.NoContent(204)
    })
    for _, kind := range []string{"restaurant", "delivery"} {
        path := "/merchants"
        if kind == "delivery" {
            path = "/riders"
        }
        g.GET(path, func(c echo.Context) error {
            repo, err := svc.operations()
            if err != nil {
                return echo.NewHTTPError(503, "operations unavailable")
            }
            data, err := repo.ListApplications(c.Request().Context(), kind, c.QueryParam("status"))
            if err != nil {
                return echo.NewHTTPError(500, "could not load partners")
            }
            return c.JSON(200, data)
        })
        g.PATCH(path+"/:id/suspension", func(c echo.Context) error {
            var req struct {
                Suspended bool   `json:"suspended"`
                Reason    string `json:"reason"`
            }
            if err := c.Bind(&req); err != nil {
                return echo.NewHTTPError(400, "invalid request")
            }
            repo, err := svc.operations()
            if err != nil {
                return echo.NewHTTPError(503, "operations unavailable")
            }
            if err = repo.Suspend(c.Request().Context(), c.Get("userID").(string), kind, c.Param("id"), req.Suspended, req.Reason); err != nil {
                return echo.NewHTTPError(409, err.Error())
            }
            return c.NoContent(204)
        })
    }
    g.GET("/applications/:id", func(c echo.Context) error {
        repo, err := svc.operations()
        if err != nil {
            return echo.NewHTTPError(503, "operations unavailable")
        }
        data, err := repo.ApplicationDetail(c.Request().Context(), c.Param("id"))
        if err != nil {
            return echo.NewHTTPError(404, "submitted application details not found")
        }
        return c.JSON(200, data)
    })
    g.GET("/audit", func(c echo.Context) error {
        repo, err := svc.operations()
        if err != nil {
            return echo.NewHTTPError(503, "operations unavailable")
        }
        data, err := repo.Audit(c.Request().Context())
        if err != nil {
            return echo.NewHTTPError(500, "could not load audit history")
        }
        return c.JSON(200, data)
    })
    g.GET("/orders", func(c echo.Context) error {
        repo, err := svc.operations()
        if err != nil {
            return echo.NewHTTPError(503, "operations unavailable")
        }
        data, err := repo.Orders(c.Request().Context(), c.QueryParam("status"))
        if err != nil {
            return echo.NewHTTPError(500, "could not load orders")
        }
        return c.JSON(200, data)
    })
    g.GET("/email-failures", func(c echo.Context) error {
        repo, err := svc.operations()
        if err != nil {
            return echo.NewHTTPError(503, "operations unavailable")
        }
        data, err := repo.EmailFailures(c.Request().Context())
        if err != nil {
            return echo.NewHTTPError(500, "could not load email delivery status")
        }
        return c.JSON(200, data)
    })
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
            Role   string `json:"role"`
            Reason string `json:"reason"`
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        repo, err := svc.operations()
        if err != nil {
            return echo.NewHTTPError(503, "operations unavailable")
        }
        if err := repo.ChangeRole(c.Request().Context(), c.Get("userID").(string), userID, req.Role, req.Reason); err != nil {
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
            Reason string `json:"reason"`
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        if err := svc.Decide(c.Request().Context(), c.Get("userID").(string), "restaurant", merchantID, Decision{Status: req.Status, Reason: req.Reason}); err != nil {
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
            Reason string `json:"reason"`
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        if err := svc.Decide(c.Request().Context(), c.Get("userID").(string), "delivery", partnerID, Decision{Status: req.Status, Reason: req.Reason}); err != nil {
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

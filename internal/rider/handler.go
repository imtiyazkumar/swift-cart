package rider

import (
    "context"
    "net/http"

    "github.com/imtiyazkumar/yember-core/config"
    "github.com/imtiyazkumar/yember-core/internal/stations"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

func RegisterRoutes(g *echo.Group, cfg *config.Config, logg *zap.Logger, svc *Service) {
    g.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            profile, err := svc.GetProfile(c.Request().Context(), c.Get("userID").(string))
            if err != nil {
                return echo.NewHTTPError(404, "delivery profile not found")
            }
            if c.Request().Method != "GET" && profile.ApprovalStatus != "APPROVED" {
                return echo.NewHTTPError(403, "approved delivery partner account required")
            }
            c.Set("partnerID", profile.ID)
            c.SetRequest(c.Request().WithContext(context.WithValue(c.Request().Context(), partnerContextKey{}, profile.ID)))
            return next(c)
        }
    })
    g.POST("/deliveries/:id/reject", func(c echo.Context) error {
        repo, ok := svc.repo.(interface {
            RejectJob(context.Context, string, string) error
        })
        if !ok {
            return echo.NewHTTPError(503, "delivery offers unavailable")
        }
        if err := repo.RejectJob(c.Request().Context(), c.Get("partnerID").(string), c.Param("id")); err != nil {
            return echo.NewHTTPError(409, err.Error())
        }
        return c.NoContent(204)
    })
    g.GET("/deliveries/current", func(c echo.Context) error {
        repo, ok := svc.repo.(interface {
            CurrentJobs(context.Context, string) ([]*DeliveryJob, error)
        })
        if !ok {
            return echo.NewHTTPError(503, "delivery assignments unavailable")
        }
        jobs, err := repo.CurrentJobs(c.Request().Context(), c.Get("partnerID").(string))
        if err != nil {
            return echo.NewHTTPError(500, "could not load current delivery")
        }
        return c.JSON(200, jobs)
    })
    // Rider Profile
    g.GET("/profile", func(c echo.Context) error {
        userID, _ := c.Get("userID").(string)
        if userID == "" {
            userID = c.QueryParam("user_id")
        }
        profile, err := svc.GetProfile(c.Request().Context(), userID)
        if err != nil {
            return echo.NewHTTPError(http.StatusNotFound, "rider profile not found")
        }
        return c.JSON(http.StatusOK, profile)
    })

    // Toggle Online/Offline Status
    g.PATCH("/status", func(c echo.Context) error {
        var req struct {
            PartnerID string `json:"partner_id"`
            Status    string `json:"status"`
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        if err := svc.UpdateStatus(c.Request().Context(), c.Get("partnerID").(string), req.Status); err != nil {
            return echo.NewHTTPError(http.StatusConflict, err.Error())
        }
        return c.JSON(http.StatusOK, map[string]string{"status": req.Status})
    })

    // Update GPS Location
    g.POST("/location", func(c echo.Context) error {
        var req stations.Location
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(400, "invalid location")
        }
        repo, ok := svc.repo.(interface {
            RecordVerifiedLocation(context.Context, string, stations.Location) error
        })
        if !ok {
            return echo.NewHTTPError(503, "verified location service unavailable")
        }
        if err := repo.RecordVerifiedLocation(c.Request().Context(), c.Get("partnerID").(string), req); err != nil {
            return echo.NewHTTPError(400, err.Error())
        }
        return c.JSON(http.StatusOK, map[string]string{"message": "location recorded"})
    })

    // List available delivery jobs
    g.GET("/deliveries/available", func(c echo.Context) error {
        jobs, err := svc.ListAvailableJobs(c.Request().Context())
        if err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, jobs)
    })

    // Accept job assignment
    g.POST("/deliveries/:id/accept", func(c echo.Context) error {
        orderID := c.Param("id")
        var req struct {
            PartnerID string `json:"partner_id"`
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        if err := svc.AcceptJob(c.Request().Context(), c.Get("partnerID").(string), orderID); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, err.Error())
        }
        return c.JSON(http.StatusOK, map[string]string{"message": "delivery job accepted"})
    })

    // Update Delivery Status (PICKED_UP, OUT_FOR_DELIVERY, DELIVERED)
    g.PATCH("/deliveries/:id/status", func(c echo.Context) error {
        orderID := c.Param("id")
        var req struct {
            PartnerID string `json:"partner_id"`
            Status    string `json:"status"`
 Code string `json:"delivery_code"`
 CashCollected bool `json:"cash_collected"`
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        if err := svc.UpdateJobStatus(WithCompletion(c.Request().Context(),req.Code,req.CashCollected), c.Get("partnerID").(string), orderID, req.Status); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, err.Error())
        }
        return c.JSON(http.StatusOK, map[string]string{"message": "delivery status updated", "status": req.Status})
    })

    // Earnings summary
    g.GET("/earnings", func(c echo.Context) error {
        partnerID := c.Get("partnerID").(string)
        summary, err := svc.GetEarningsSummary(c.Request().Context(), partnerID)
        if err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, summary)
    })
}

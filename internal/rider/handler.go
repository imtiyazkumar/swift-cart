package rider

import (
    "net/http"

    "github.com/imtiyazkumar/yember-core/config"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

func RegisterRoutes(g *echo.Group, cfg *config.Config, logg *zap.Logger, svc *Service) {
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
        if err := svc.UpdateStatus(c.Request().Context(), req.PartnerID, req.Status); err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, map[string]string{"status": req.Status})
    })

    // Update GPS Location
    g.POST("/location", func(c echo.Context) error {
        var req struct {
            PartnerID string  `json:"partner_id"`
            Latitude  float64 `json:"latitude"`
            Longitude float64 `json:"longitude"`
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        if err := svc.RecordLocation(c.Request().Context(), req.PartnerID, req.Latitude, req.Longitude); err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
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
        if err := svc.AcceptJob(c.Request().Context(), req.PartnerID, orderID); err != nil {
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
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
        }
        if err := svc.UpdateJobStatus(c.Request().Context(), req.PartnerID, orderID, req.Status); err != nil {
            return echo.NewHTTPError(http.StatusBadRequest, err.Error())
        }
        return c.JSON(http.StatusOK, map[string]string{"message": "delivery status updated", "status": req.Status})
    })

    // Earnings summary
    g.GET("/earnings", func(c echo.Context) error {
        partnerID := c.QueryParam("partner_id")
        summary, err := svc.GetEarningsSummary(c.Request().Context(), partnerID)
        if err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        return c.JSON(http.StatusOK, summary)
    })
}

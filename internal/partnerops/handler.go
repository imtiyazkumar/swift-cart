package partnerops

import (
    "context"
    "encoding/json"
    "errors"
    "strconv"
    "strings"

    "github.com/google/uuid"
    "github.com/imtiyazkumar/yember-core/config"
    "github.com/imtiyazkumar/yember-core/internal/auth"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

type Repository struct{ pool *pgxpool.Pool }
type Page struct {
    Items []json.RawMessage `json:"items"`
    Total int               `json:"total"`
    Page  int               `json:"page"`
    Limit int               `json:"limit"`
}

func Register(api *echo.Group, pool *pgxpool.Pool, cfg *config.Config, log *zap.Logger) {
    repo := &Repository{pool: pool}
    group := api.Group("/partner", auth.JWTMiddleware(cfg.JWTSecret, log, pool), auth.RBACMiddleware("MERCHANT_OWNER", "DELIVERY_PARTNER"))
    group.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            if id := c.Param("id"); id != "" {
                if _, err := uuid.Parse(id); err != nil {
                    return echo.NewHTTPError(400, "invalid record identifier")
                }
            }
            c.Response().Header().Set("Cache-Control", "no-store")
            return next(c)
        }
    })
    registerAccount(group, repo)
    registerSupport(group, repo)
    admin := api.Group("/admin/partner-support", auth.JWTMiddleware(cfg.JWTSecret, log, pool), auth.RBACMiddleware("ADMIN"))
    admin.POST("/:id/messages", func(c echo.Context) error {
        if _, err := uuid.Parse(c.Param("id")); err != nil {
            return echo.NewHTTPError(400, "invalid case identifier")
        }
        var input struct {
            Body string `json:"body"`
        }
        if c.Bind(&input) != nil || strings.TrimSpace(input.Body) == "" || len(input.Body) > 4000 {
            return echo.NewHTTPError(400, "a message of 1-4000 characters is required")
        }
        if err := repo.SupportReply(c.Request().Context(), c.Get("userID").(string), c.Param("id"), strings.TrimSpace(input.Body), true); err != nil {
            return err
        }
        return c.NoContent(204)
    })
    group.GET("/overview", func(c echo.Context) error {
        days, _ := strconv.Atoi(c.QueryParam("days"))
        if days != 30 {
            days = 7
        }
        data, err := repo.Overview(c.Request().Context(), c.Get("userID").(string), c.Get("role").(string), days)
        return respond(c, data, err)
    })
    group.GET("/orders", func(c echo.Context) error {
        page, limit := pagination(c)
        data, err := repo.Orders(c.Request().Context(), c.Get("userID").(string), c.Get("role").(string), c.QueryParam("scope"), c.QueryParam("status"), strings.TrimSpace(c.QueryParam("q")), page, limit)
        return respond(c, data, err)
    })
    group.GET("/orders/:id", func(c echo.Context) error {
        data, err := repo.Order(c.Request().Context(), c.Get("userID").(string), c.Get("role").(string), c.Param("id"))
        return respond(c, data, err)
    })
    group.GET("/earnings", func(c echo.Context) error {
        if c.Get("role") != "DELIVERY_PARTNER" {
            return echo.NewHTTPError(403, "delivery partner required")
        }
        page, limit := pagination(c)
        days, _ := strconv.Atoi(c.QueryParam("days"))
        if days != 30 {
            days = 7
        }
        data, err := repo.Earnings(c.Request().Context(), c.Get("userID").(string), page, limit, days)
        return respond(c, data, err)
    })
    group.GET("/reviews", func(c echo.Context) error {
        if c.Get("role") != "MERCHANT_OWNER" {
            return echo.NewHTTPError(403, "restaurant owner required")
        }
        page, limit := pagination(c)
        data, err := repo.Reviews(c.Request().Context(), c.Get("userID").(string), page, limit)
        return respond(c, data, err)
    })
    group.PUT("/reviews/:id/reply", func(c echo.Context) error {
        if c.Get("role") != "MERCHANT_OWNER" {
            return echo.NewHTTPError(403, "restaurant owner required")
        }
        var input struct {
            Reply string `json:"reply"`
        }
        if c.Bind(&input) != nil || len(strings.TrimSpace(input.Reply)) < 1 || len(input.Reply) > 2000 {
            return echo.NewHTTPError(400, "a reply of 1-2000 characters is required")
        }
        tag, err := pool.Exec(c.Request().Context(), `UPDATE merchants.restaurant_reviews r SET reply=$3,replied_at=now() FROM public.merchants m WHERE r.id=$1 AND r.merchant_id=m.id AND m.owner_id=$2 AND m.approval_status='APPROVED' AND m.deleted_at IS NULL AND r.hidden_at IS NULL`, c.Param("id"), c.Get("userID"), strings.TrimSpace(input.Reply))
        if err != nil {
            return echo.NewHTTPError(500, "could not save review reply")
        }
        if tag.RowsAffected() != 1 {
            return echo.NewHTTPError(404, "review not found")
        }
        return c.NoContent(204)
    })
}

func pagination(c echo.Context) (int, int) {
    page, _ := strconv.Atoi(c.QueryParam("page"))
    limit, _ := strconv.Atoi(c.QueryParam("limit"))
    if page < 1 {
        page = 1
    }
    if page > 100000 {
        page = 100000
    }
    if limit < 1 || limit > 50 {
        limit = 20
    }
    return page, limit
}
func respond(c echo.Context, data interface{}, err error) error {
    if errors.Is(err, pgx.ErrNoRows) {
        return echo.NewHTTPError(404, "record not found")
    }
    if err != nil {
        return echo.NewHTTPError(500, "could not load partner workspace")
    }
    return c.JSON(200, data)
}
func (r *Repository) rows(ctx context.Context, query string, args ...interface{}) ([]json.RawMessage, error) {
    rows, err := r.pool.Query(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    result := []json.RawMessage{}
    for rows.Next() {
        var data json.RawMessage
        if err = rows.Scan(&data); err != nil {
            return nil, err
        }
        result = append(result, data)
    }
    return result, rows.Err()
}

package restaurant

import (
    "context"
    "encoding/json"
    "errors"
    "strings"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/labstack/echo/v4"
)

type OperatingHour struct {
    Day    int    `json:"day"`
    Opens  string `json:"opens"`
    Closes string `json:"closes"`
    Closed bool   `json:"closed"`
}
type ProfileInput struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Phone       string          `json:"phone"`
    Address     string          `json:"address"`
    Tags        []string        `json:"tags"`
    Hours       []OperatingHour `json:"hours"`
}

func (r *pgRepo) Settings(ctx context.Context, merchant string) (json.RawMessage, error) {
    var result json.RawMessage
    err := r.db.QueryRow(ctx, `SELECT json_build_object('name',m.name,'description',m.description,'phone',COALESCE(m.phone,''),'tags',m.tags,'address',COALESCE((SELECT address FROM merchants.merchant_branches WHERE merchant_id=m.id ORDER BY created_at LIMIT 1),''),'hours',COALESCE((SELECT json_agg(json_build_object('day',h.day_of_week,'opens',to_char(h.opens_at,'HH24:MI'),'closes',to_char(h.closes_at,'HH24:MI'),'closed',h.is_closed)) FROM merchants.merchant_operating_hours h JOIN merchants.merchant_branches b ON b.id=h.branch_id WHERE b.merchant_id=m.id),'[]'::json)) FROM public.merchants m WHERE id=$1`, merchant).Scan(&result)
    return result, err
}

func (r *pgRepo) SaveSettings(ctx context.Context, merchant string, input ProfileInput) error {
    if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Address) == "" || len(input.Name) > 200 || len(input.Address) > 2000 || len(input.Description) > 5000 || len(input.Phone) > 30 {
        return errors.New("valid restaurant name, address and contact details required")
    }
    days := map[int]bool{}
    for _, h := range input.Hours {
        if h.Day < 0 || h.Day > 6 || days[h.Day] {
            return errors.New("operating days must be unique and between 0 and 6")
        }
        days[h.Day] = true
        if _, err := time.Parse("15:04", h.Opens); err != nil {
            return errors.New("opening time must use HH:MM")
        }
        if _, err := time.Parse("15:04", h.Closes); err != nil {
            return errors.New("closing time must use HH:MM")
        }
    }
    tx, err := r.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    var branch string
    if _, err = tx.Exec(ctx, `UPDATE public.merchants SET name=$2,description=$3,phone=$4,tags=$5,updated_at=now() WHERE id=$1`, merchant, input.Name, input.Description, input.Phone, input.Tags); err != nil {
        return err
    }
    err = tx.QueryRow(ctx, `SELECT id FROM merchants.merchant_branches WHERE merchant_id=$1 ORDER BY created_at LIMIT 1 FOR UPDATE`, merchant).Scan(&branch)
    if err == pgx.ErrNoRows {
        err = tx.QueryRow(ctx, `INSERT INTO merchants.merchant_branches(merchant_id,name,address) VALUES($1,$2,$3) RETURNING id`, merchant, input.Name, input.Address).Scan(&branch)
    } else if err == nil {
        _, err = tx.Exec(ctx, `UPDATE merchants.merchant_branches SET name=$2,address=$3,updated_at=now() WHERE id=$1`, branch, input.Name, input.Address)
    }
    if err != nil {
        return err
    }
    for _, h := range input.Hours {
        if _, err = tx.Exec(ctx, `INSERT INTO merchants.merchant_operating_hours(branch_id,day_of_week,opens_at,closes_at,is_closed) VALUES($1,$2,$3::time,$4::time,$5) ON CONFLICT(branch_id,day_of_week) DO UPDATE SET opens_at=EXCLUDED.opens_at,closes_at=EXCLUDED.closes_at,is_closed=EXCLUDED.is_closed`, branch, h.Day, h.Opens, h.Closes, h.Closed); err != nil {
            return err
        }
    }
    return tx.Commit(ctx)
}

func registerSettings(g *echo.Group, svc *Service) {
    type settingsRepo interface {
        Settings(context.Context, string) (json.RawMessage, error)
        SaveSettings(context.Context, string, ProfileInput) error
    }
    g.GET("/settings", func(c echo.Context) error {
        repo, ok := svc.repo.(settingsRepo)
        if !ok {
            return echo.NewHTTPError(503, "settings unavailable")
        }
        data, err := repo.Settings(c.Request().Context(), c.Get("merchantID").(string))
        if err != nil {
            return echo.NewHTTPError(500, "could not load restaurant settings")
        }
        return c.JSON(200, data)
    })
    g.PUT("/settings", func(c echo.Context) error {
        repo, ok := svc.repo.(settingsRepo)
        if !ok {
            return echo.NewHTTPError(503, "settings unavailable")
        }
        var input ProfileInput
        if err := c.Bind(&input); err != nil {
            return echo.NewHTTPError(400, "invalid settings")
        }
        if err := repo.SaveSettings(c.Request().Context(), c.Get("merchantID").(string), input); err != nil {
            return echo.NewHTTPError(400, err.Error())
        }
        return c.NoContent(204)
    })
}

package partnerops

import (
    "context"
    "encoding/json"
    "strings"

    "github.com/jackc/pgx/v5"
    "github.com/labstack/echo/v4"
)

type TicketInput struct {
    Category   string `json:"category"`
    Subject    string `json:"subject"`
    Message    string `json:"message"`
    OrderID    string `json:"order_id"`
    RequestKey string `json:"request_key"`
}

func registerSupport(g *echo.Group, r *Repository) {
    g.GET("/support", func(c echo.Context) error {
        page, limit := pagination(c)
        user := c.Get("userID").(string)
        result := &Page{Items: []json.RawMessage{}, Page: page, Limit: limit}
        if err := r.pool.QueryRow(c.Request().Context(), `SELECT count(*) FROM public.partner_support_tickets WHERE user_id=$1`, user).Scan(&result.Total); err != nil {
            return echo.NewHTTPError(500, "could not load support cases")
        }
        var err error
        result.Items, err = r.rows(c.Request().Context(), `SELECT jsonb_build_object('id',id,'subject',subject,'category',category,'status',status,'order_id',order_id,'created_at',created_at,'updated_at',updated_at) FROM public.partner_support_tickets WHERE user_id=$1 ORDER BY updated_at DESC,id LIMIT $2 OFFSET $3`, user, limit, (page-1)*limit)
        return respond(c, result, err)
    })
    g.GET("/support/:id", func(c echo.Context) error {
        var ticket json.RawMessage
        err := r.pool.QueryRow(c.Request().Context(), `SELECT jsonb_build_object('id',t.id,'subject',t.subject,'category',t.category,'status',t.status,'order_id',t.order_id,'created_at',t.created_at,'messages',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',m.id,'body',m.body,'author',CASE WHEN m.author_id=t.user_id THEN 'You' ELSE 'Yember support' END,'created_at',m.created_at) ORDER BY m.created_at,m.id) FROM public.partner_support_messages m WHERE m.ticket_id=t.id),'[]'::jsonb)) FROM public.partner_support_tickets t WHERE id=$1 AND user_id=$2`, c.Param("id"), c.Get("userID")).Scan(&ticket)
        return respond(c, ticket, err)
    })
    g.POST("/support", func(c echo.Context) error {
        var input TicketInput
        if c.Bind(&input) != nil {
            return echo.NewHTTPError(400, "invalid support case")
        }
        id, err := r.CreateTicket(c.Request().Context(), c.Get("userID").(string), c.Get("role").(string), input)
        if err != nil {
            return err
        }
        return c.JSON(201, map[string]string{"id": id})
    })
    g.POST("/support/:id/messages", func(c echo.Context) error {
        var input struct {
            Body string `json:"body"`
        }
        if c.Bind(&input) != nil || strings.TrimSpace(input.Body) == "" || len(input.Body) > 4000 {
            return echo.NewHTTPError(400, "a message of 1-4000 characters is required")
        }
        err := r.SupportReply(c.Request().Context(), c.Get("userID").(string), c.Param("id"), strings.TrimSpace(input.Body), false)
        if err != nil {
            return err
        }
        return c.NoContent(204)
    })
}

func (r *Repository) CreateTicket(ctx context.Context, user, role string, input TicketInput) (string, error) {
    input.Subject = strings.TrimSpace(input.Subject)
    input.Message = strings.TrimSpace(input.Message)
    if len(input.Subject) < 3 || len(input.Subject) > 160 || len(input.Message) < 5 || len(input.Message) > 4000 || len(input.RequestKey) < 8 || len(input.RequestKey) > 100 {
        return "", echo.NewHTTPError(400, "provide a subject, message and request identifier")
    }
    if input.Category != "ORDER" && input.Category != "ACCOUNT" && input.Category != "EARNINGS" && input.Category != "MENU" && input.Category != "OTHER" {
        return "", echo.NewHTTPError(400, "invalid support category")
    }
    tx, err := r.pool.Begin(ctx)
    if err != nil {
        return "", echo.NewHTTPError(503, "support service unavailable")
    }
    defer tx.Rollback(ctx)
    if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,894215))`, user); err != nil {
        return "", err
    }
    var id string
    err = tx.QueryRow(ctx, `SELECT id FROM public.partner_support_tickets WHERE user_id=$1 AND request_key=$2`, user, input.RequestKey).Scan(&id)
    if err == nil {
        return id, nil
    }
    if err != pgx.ErrNoRows {
        return "", err
    }
    var open int
    if err = tx.QueryRow(ctx, `SELECT count(*) FROM public.partner_support_tickets WHERE user_id=$1 AND status<>'RESOLVED'`, user).Scan(&open); err != nil {
        return "", err
    }
    if open >= 20 {
        return "", echo.NewHTTPError(409, "reply to an existing case before opening more cases")
    }
    if input.OrderID != "" {
        var allowed bool
        if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.orders o WHERE o.id::text=$2 AND `+orderScope(role)+`)`, user, input.OrderID).Scan(&allowed); err != nil {
            return "", err
        }
        if !allowed {
            return "", echo.NewHTTPError(404, "linked order not found")
        }
    }
    if err = tx.QueryRow(ctx, `INSERT INTO public.partner_support_tickets(user_id,order_id,category,subject,request_key) VALUES($1,NULLIF($2,'')::uuid,$3,$4,$5) RETURNING id`, user, input.OrderID, input.Category, input.Subject, input.RequestKey).Scan(&id); err != nil {
        return "", echo.NewHTTPError(500, "could not create support case")
    }
    if _, err = tx.Exec(ctx, `INSERT INTO public.partner_support_messages(ticket_id,author_id,body) VALUES($1,$2,$3)`, id, user, input.Message); err != nil {
        return "", err
    }
    if err = tx.Commit(ctx); err != nil {
        return "", err
    }
    return id, nil
}

func (r *Repository) SupportReply(ctx context.Context, user, ticket, body string, admin bool) error {
    tx, err := r.pool.Begin(ctx)
    if err != nil {
        return echo.NewHTTPError(503, "support service unavailable")
    }
    defer tx.Rollback(ctx)
    query := `SELECT id FROM public.partner_support_tickets WHERE id=$1`
    if !admin {
        query += ` AND user_id=$2`
    } else {
        query += ` AND EXISTS(SELECT 1 FROM public.users WHERE id=$2 AND role='ADMIN' AND disabled_at IS NULL AND deleted_at IS NULL)`
    }
    query += ` FOR UPDATE`
    var found string
    if err = tx.QueryRow(ctx, query, ticket, user).Scan(&found); err != nil {
        return echo.NewHTTPError(404, "support case not found")
    }
    if _, err = tx.Exec(ctx, `INSERT INTO public.partner_support_messages(ticket_id,author_id,body) VALUES($1,$2,$3)`, ticket, user, body); err != nil {
        return echo.NewHTTPError(500, "could not send message")
    }
    status := "OPEN"
    if admin {
        status = "IN_PROGRESS"
    }
    if _, err = tx.Exec(ctx, `UPDATE public.partner_support_tickets SET status=$2,updated_at=now() WHERE id=$1`, ticket, status); err != nil {
        return echo.NewHTTPError(500, "could not update support case")
    }
    if admin {
        if _, err = tx.Exec(ctx, `INSERT INTO public.admin_audit_logs(admin_id,action,target_type,target_id,details) VALUES($1,'SUPPORT_REPLY_SENT','support',$2,'{"reason":"Support reply sent"}')`, user, ticket); err != nil {
            return echo.NewHTTPError(500, "could not audit reply")
        }
    }
    if err = tx.Commit(ctx); err != nil {
        return echo.NewHTTPError(500, "could not send message")
    }
    return nil
}

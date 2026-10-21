package onboarding

import (
    "io"
    "net/http"
    "os"
    "strings"

    "github.com/google/uuid"
    "github.com/imtiyazkumar/yember-core/config"
    "github.com/imtiyazkumar/yember-core/internal/auth"
    "github.com/imtiyazkumar/yember-core/internal/storage"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

func Register(api *echo.Group, pool *pgxpool.Pool, cfg *config.Config, log *zap.Logger) {
    group := api.Group("/onboarding", auth.JWTMiddleware(cfg.JWTSecret, log, pool))
    service := New(pool)
    group.POST("/application", func(c echo.Context) error {
        role, _ := c.Get("role").(string)
        kind := ""
        if role == "MERCHANT_OWNER" {
            kind = "restaurant"
        } else if role == "DELIVERY_PARTNER" {
            kind = "delivery"
        } else {
            return echo.NewHTTPError(403, "partner role required")
        }
        var a Application
        if err := c.Bind(&a); err != nil {
            return echo.NewHTTPError(400, "invalid application")
        }
        id, err := service.Submit(c.Request().Context(), c.Get("userID").(string), kind, a)
        if err != nil {
            return echo.NewHTTPError(400, err.Error())
        }
        return c.JSON(201, map[string]string{"id": id, "status": "PENDING"})
    })
    group.GET("/application", func(c echo.Context) error {
        var data []byte
        err := pool.QueryRow(c.Request().Context(), `SELECT json_build_object('id',id,'kind',kind,'resource_id',resource_id,'data',data,'document_ids',document_ids,'created_at',created_at) FROM public.partner_applications WHERE user_id=$1`, c.Get("userID")).Scan(&data)
        if err == pgx.ErrNoRows {
            return c.JSON(200, nil)
        }
        if err != nil {
            return echo.NewHTTPError(500, "could not load application")
        }
        return c.Blob(200, "application/json", data)
    })
    group.POST("/documents", func(c echo.Context) error {
        role, _ := c.Get("role").(string)
        if role != "MERCHANT_OWNER" && role != "DELIVERY_PARTNER" {
            return echo.NewHTTPError(403, "partner role required")
        }
        backend := os.Getenv("DOCUMENT_STORAGE_BACKEND")
        if backend == "" {
            backend = "local"
        }
        store, err := storage.Configured(c.Request().Context(), backend)
        if err != nil {
            return echo.NewHTTPError(503, "private document storage is not configured")
        }
        c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, 11<<20)
        header, err := c.FormFile("file")
        if err != nil || header.Size < 1 || header.Size > 10<<20 {
            return echo.NewHTTPError(400, "a file of up to 10 MiB is required")
        }
        documentType := strings.TrimSpace(c.FormValue("document_type"))
        if documentType == "" || len(documentType) > 100 {
            return echo.NewHTTPError(400, "document_type is required")
        }
        input, err := header.Open()
        if err != nil {
            return echo.NewHTTPError(400, "could not read file")
        }
        defer input.Close()
        prefix := make([]byte, 512)
        n, err := io.ReadFull(input, prefix)
        if err != nil && err != io.ErrUnexpectedEOF {
            return echo.NewHTTPError(400, "could not inspect file")
        }
        contentType := http.DetectContentType(prefix[:n])
        if contentType != "application/pdf" && contentType != "image/jpeg" && contentType != "image/png" {
            return echo.NewHTTPError(400, "only PDF, JPEG and PNG documents are supported")
        }
        id := uuid.NewString()
        if _, err = input.Seek(0, io.SeekStart); err != nil {
            return echo.NewHTTPError(400, "could not read uploaded document")
        }
        if err = store.Put(c.Request().Context(), id, contentType, input); err != nil {
            return echo.NewHTTPError(503, "document storage unavailable")
        }
        _, err = pool.Exec(c.Request().Context(), `INSERT INTO public.private_documents(id,user_id,document_type,content_type,byte_size,backend) VALUES($1,$2,$3,$4,$5,$6)`, id, c.Get("userID"), documentType, contentType, header.Size, backend)
        if err != nil {
            _ = store.Delete(c.Request().Context(), id)
            return echo.NewHTTPError(500, "could not save document")
        }
        return c.JSON(201, map[string]string{"id": id, "content_type": contentType, "document_type": documentType})
    })
    group.GET("/documents/:id", func(c echo.Context) error {
        id, err := uuid.Parse(c.Param("id"))
        if err != nil {
            return echo.NewHTTPError(404, "document not found")
        }
        var contentType, backend string
        role, _ := c.Get("role").(string)
        err = pool.QueryRow(c.Request().Context(), `SELECT content_type,backend FROM public.private_documents WHERE id=$1 AND (user_id=$2 OR $3='ADMIN')`, id.String(), c.Get("userID"), role).Scan(&contentType, &backend)
        if err != nil {
            return echo.NewHTTPError(404, "document not found")
        }
        store, err := storage.Configured(c.Request().Context(), backend)
        if err != nil {
            return echo.NewHTTPError(503, "document storage unavailable")
        }
        file, err := store.Open(c.Request().Context(), id.String())
        if err != nil {
            return echo.NewHTTPError(404, "document is unavailable")
        }
        defer file.Close()
        c.Response().Header().Set("Content-Type", contentType)
        c.Response().Header().Set("Content-Disposition", "attachment; filename="+id.String())
        c.Response().Header().Set("Cache-Control", "no-store")
        return c.Stream(200, contentType, file)
    })
}

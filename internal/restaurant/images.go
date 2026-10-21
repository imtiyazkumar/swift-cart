package restaurant

import (
    "context"
    "encoding/json"
    "image"
    _ "image/jpeg"
    _ "image/png"
    "io"
    "net/http"
    "os"
    "strings"

    "github.com/google/uuid"
    "github.com/imtiyazkumar/yember-core/config"
    "github.com/imtiyazkumar/yember-core/internal/auth"
    "github.com/imtiyazkumar/yember-core/internal/storage"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

func RegisterImages(api *echo.Group, pool *pgxpool.Pool, cfg *config.Config, log *zap.Logger) {
    api.POST("/restaurant/menu/items/:id/image", func(c echo.Context) error {
        base := strings.TrimRight(os.Getenv("PUBLIC_API_BASE_URL"), "/")
        if base == "" {
            return echo.NewHTTPError(503, "public API base URL is not configured")
        }
        var owner string
        if err := pool.QueryRow(c.Request().Context(), `SELECT m.owner_id FROM catalog.items i JOIN public.merchants m ON m.id=i.merchant_id WHERE i.id=$1 AND m.approval_status='APPROVED' AND m.fee_payment_status='PAID' AND i.deleted_at IS NULL`, c.Param("id")).Scan(&owner); err != nil || owner != c.Get("userID") {
            return echo.NewHTTPError(404, "menu item not found")
        }
        c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, 6<<20)
        header, err := c.FormFile("file")
        if err != nil || header.Size < 1 || header.Size > 5<<20 {
            return echo.NewHTTPError(400, "a JPEG or PNG image of up to 5 MiB is required")
        }
        file, err := header.Open()
        if err != nil {
            return echo.NewHTTPError(400, "could not read image")
        }
        defer file.Close()
        dimensions, format, err := image.DecodeConfig(file)
        if err != nil || (format != "jpeg" && format != "png") || dimensions.Width > 10000 || dimensions.Height > 10000 {
            return echo.NewHTTPError(400, "invalid or oversized JPEG/PNG image")
        }
        if _, err = file.Seek(0, io.SeekStart); err != nil {
            return echo.NewHTTPError(400, "could not read image")
        }
        backend := os.Getenv("DOCUMENT_STORAGE_BACKEND")
        if backend == "" {
            backend = "local"
        }
        store, err := storage.Configured(c.Request().Context(), backend)
        if err != nil {
            return echo.NewHTTPError(503, "image storage unavailable")
        }
        key := uuid.NewString()
        contentType := "image/" + format
        if err = store.Put(c.Request().Context(), key, contentType, file); err != nil {
            return echo.NewHTTPError(503, "image upload failed")
        }
        imageURL := base + "/menu-images/" + key
        data, _ := json.Marshal(map[string]string{"image_url": imageURL, "image_key": key, "image_backend": backend, "image_content_type": contentType})
        _, err = pool.Exec(c.Request().Context(), `UPDATE catalog.items SET metadata=metadata || $2::jsonb,updated_at=now() WHERE id=$1`, c.Param("id"), data)
        if err != nil {
            _ = store.Delete(context.Background(), key)
            return echo.NewHTTPError(500, "could not attach image")
        }
        return c.JSON(201, map[string]string{"image_url": imageURL})
    }, auth.JWTMiddleware(cfg.JWTSecret, log, pool), auth.RBACMiddleware("MERCHANT_OWNER"))
    api.GET("/menu-images/:key", func(c echo.Context) error {
        key, err := uuid.Parse(c.Param("key"))
        if err != nil {
            return echo.NewHTTPError(404, "image not found")
        }
        var backend, contentType string
        err = pool.QueryRow(c.Request().Context(), `SELECT i.metadata->>'image_backend',i.metadata->>'image_content_type' FROM catalog.items i JOIN public.merchants m ON m.id=i.merchant_id WHERE i.metadata->>'image_key'=$1 AND i.deleted_at IS NULL AND m.is_active AND m.approval_status='APPROVED' AND m.fee_payment_status='PAID'`, key.String()).Scan(&backend, &contentType)
        if err != nil {
            return echo.NewHTTPError(404, "image not found")
        }
        store, err := storage.Configured(c.Request().Context(), backend)
        if err != nil {
            return echo.NewHTTPError(503, "image storage unavailable")
        }
        file, err := store.Open(c.Request().Context(), key.String())
        if err != nil {
            return echo.NewHTTPError(404, "image unavailable")
        }
        defer file.Close()
        c.Response().Header().Set("X-Content-Type-Options", "nosniff")
        c.Response().Header().Set("Cache-Control", "public,max-age=300")
        return c.Stream(200, contentType, file)
    })
}

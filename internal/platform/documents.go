package platform

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func storeUpload(c echo.Context, folder string, imagesOnly bool) (string, error) {
	file, err := c.FormFile("file")
	if err != nil || file.Size <= 0 || file.Size > 10*1024*1024 {
		return "", bad("a PDF, JPEG or PNG file up to 10 MiB is required")
	}
	stream, err := file.Open()
	if err != nil {
		return "", err
	}
	defer stream.Close()
	header := make([]byte, 512)
	n, err := stream.Read(header)
	if err != nil && err != io.EOF {
		return "", err
	}
	header = header[:n]
	mime := http.DetectContentType(header)
	valid := mime == "image/jpeg" || mime == "image/png" || (!imagesOnly && strings.HasPrefix(string(header), "%PDF-"))
	if !valid {
		return "", bad("unsupported file content")
	}
	if err = os.MkdirAll(folder, 0700); err != nil {
		return "", err
	}
	id := uuid.NewString()
	path := filepath.Join(folder, id)
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	successful := false
	defer func() {
		out.Close()
		if !successful {
			_ = os.Remove(path)
		}
	}()
	if _, err = out.Write(header); err != nil {
		return "", err
	}
	written, err := io.Copy(out, io.LimitReader(stream, 10*1024*1024+1))
	if err != nil || written+int64(n) > 10*1024*1024 {
		return "", bad("document is too large")
	}
	if err = out.Close(); err != nil {
		return "", err
	}
	successful = true
	return id, nil
}
func documentRoot() string {
	root := os.Getenv("DOCUMENT_LOCAL_DIR")
	if root == "" {
		root = ".private-documents"
	}
	return root
}
func (s *Service) registerDocuments(api *echo.Group) {
	api.POST("/onboarding/documents", func(c echo.Context) error {
		kind := strings.TrimSpace(c.FormValue("document_type"))
		if !validName(kind, 50) {
			return bad("document type is required")
		}
		var entity int64
		var err error
		table, column := "restaurant_documents", "restaurant_id"
		if person(c).Role == "RESTAURANT_OWNER" {
			entity, err = s.restaurantID(c, false)
		} else {
			table, column = "rider_documents", "rider_id"
			entity, err = s.riderID(c)
		}
		if err != nil {
			return bad("save your application before uploading verification documents")
		}
		id, err := storeUpload(c, documentRoot(), false)
		if err != nil {
			return err
		}
		kept := false
		defer func() {
			if !kept {
				_ = os.Remove(filepath.Join(documentRoot(), id))
			}
		}()
		tx, err := s.begin(c, "CREATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), "INSERT INTO "+table+"(uuid,"+column+",document_type,file_url,verification_status) VALUES($1,$2,$3,$4,'PENDING')", id, entity, kind, "/api/v1/onboarding/documents/"+id); err != nil {
			return err
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return err
		}
		kept = true
		return c.JSON(201, map[string]string{"id": id})
	}, roles("RESTAURANT_OWNER", "DELIVERY_PARTNER"))
	api.GET("/onboarding/documents/:id", func(c echo.Context) error {
		if err := publicID(c.Param("id")); err != nil {
			return err
		}
		var allowed bool
		if err := s.DB.QueryRow(c.Request().Context(), `SELECT EXISTS(SELECT 1 FROM restaurant_documents d JOIN restaurants r ON r.id=d.restaurant_id WHERE d.uuid=$1 AND d.deleted_at IS NULL AND ($2='ADMIN' OR r.owner_user_id=$3) UNION ALL SELECT 1 FROM rider_documents d JOIN riders r ON r.id=d.rider_id WHERE d.uuid=$1 AND d.deleted_at IS NULL AND ($2='ADMIN' OR r.user_id=$3))`, c.Param("id"), person(c).Role, person(c).ID).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return echo.NewHTTPError(404, "document not found")
		}
		c.Response().Header().Set("Cache-Control", "private, no-store")
		c.Response().Header().Set("Content-Disposition", "inline")
		return c.File(filepath.Join(documentRoot(), c.Param("id")))
	})
	api.POST("/restaurant/menu/items/:id/image", func(c echo.Context) error {
		if err := publicID(c.Param("id")); err != nil {
			return err
		}
		restaurant, err := s.restaurantID(c, true)
		if err != nil {
			return err
		}
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		var item int64
		if err = tx.QueryRow(c.Request().Context(), `SELECT id FROM menu_items WHERE uuid=$1 AND restaurant_id=$2 AND deleted_at IS NULL FOR UPDATE`, c.Param("id"), restaurant).Scan(&item); err != nil {
			return unavailable(err)
		}
		id, err := storeUpload(c, filepath.Join(documentRoot(), "menu"), true)
		if err != nil {
			return err
		}
		kept := false
		defer func() {
			if !kept {
				_ = os.Remove(filepath.Join(documentRoot(), "menu", id))
			}
		}()
		if _, err = tx.Exec(c.Request().Context(), `UPDATE menu_items SET image_url=$2 WHERE id=$1`, item, "/api/v1/images/menu/"+id); err != nil {
			return err
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return err
		}
		kept = true
		return c.NoContent(204)
	}, roles("RESTAURANT_OWNER"))
}
func (s *Service) registerImages(api *echo.Group) {
	api.GET("/images/menu/:id", func(c echo.Context) error {
		if err := publicID(c.Param("id")); err != nil {
			return err
		}
		var visible bool
		if err := s.DB.QueryRow(c.Request().Context(), `SELECT EXISTS(SELECT 1 FROM menu_items i JOIN restaurants r ON r.id=i.restaurant_id WHERE i.image_url=$1 AND i.deleted_at IS NULL AND r.deleted_at IS NULL AND r.status='APPROVED')`, "/api/v1/images/menu/"+c.Param("id")).Scan(&visible); err != nil {
			return err
		}
		if !visible {
			return echo.NewHTTPError(404, "image not found")
		}
		return c.File(filepath.Join(documentRoot(), "menu", c.Param("id")))
	})
}

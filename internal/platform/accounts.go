package platform

import (
	"strings"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

func (s *Service) registerAccounts(api *echo.Group) {
	api.POST("/partner/password", func(c echo.Context) error {
		var input struct {
			Current string `json:"current_password"`
			Next    string `json:"new_password"`
		}
		if c.Bind(&input) != nil || len(input.Current) > 72 || len(input.Next) < 12 || len(input.Next) > 72 {
			return bad("current password and a new 12–72 byte password required")
		}
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		var stored string
		if err = tx.QueryRow(c.Request().Context(), `SELECT password_hash FROM users WHERE id=$1 AND status='ACTIVE' AND deleted_at IS NULL FOR UPDATE`, person(c).ID).Scan(&stored); err != nil || bcrypt.CompareHashAndPassword([]byte(stored), []byte(input.Current)) != nil {
			return bad("current password is incorrect")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(input.Next), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(c.Request().Context(), `UPDATE users SET password_hash=$2 WHERE id=$1`, person(c).ID, string(hash)); err != nil {
			return err
		}
		if _, err = tx.Exec(c.Request().Context(), `UPDATE refresh_tokens SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, person(c).ID); err != nil {
			return err
		}
		return commit(c, tx)
	}, roles("RESTAURANT_OWNER", "DELIVERY_PARTNER"))
	api.PUT("/restaurant/profile", func(c echo.Context) error {
		var input struct {
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Phone       string  `json:"phone"`
			Address     string  `json:"address"`
			Latitude    float64 `json:"latitude"`
			Longitude   float64 `json:"longitude"`
			Image       string  `json:"image_url"`
			FSSAI       string  `json:"fssai_number"`
		}
		if c.Bind(&input) != nil || !validName(input.Name, 180) || strings.TrimSpace(input.Address) == "" || len(input.Phone) > 30 || input.Latitude < -90 || input.Latitude > 90 || input.Longitude < -180 || input.Longitude > 180 {
			return bad("valid restaurant name, address and coordinates required")
		}
		restaurant, err := s.restaurantID(c, false)
		if err != nil {
			return err
		}
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		var allowed bool
		if err = tx.QueryRow(c.Request().Context(), `SELECT station_covers(station_id,$2,$3) FROM restaurants WHERE id=$1 FOR UPDATE`, restaurant, input.Latitude, input.Longitude).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return bad("restaurant must remain inside its approved service station")
		}
		if _, err = tx.Exec(c.Request().Context(), `UPDATE restaurants SET name=$2,description=$3,phone=$4,address=$5,latitude=$6,longitude=$7,image_url=NULLIF($8,''),fssai_number=NULLIF($9,'') WHERE id=$1`, restaurant, input.Name, input.Description, input.Phone, input.Address, input.Latitude, input.Longitude, input.Image, input.FSSAI); err != nil {
			return err
		}
		return commit(c, tx)
	}, roles("RESTAURANT_OWNER"))
}

func normalizePhone(value string) string {
	value = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(strings.TrimSpace(value))
	if len(value) == 10 && value[0] >= '6' && value[0] <= '9' {
		return "+91" + value
	}
	if len(value) == 12 && strings.HasPrefix(value, "91") {
		return "+" + value
	}
	return value
}

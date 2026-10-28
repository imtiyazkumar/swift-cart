package platform

import (
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"strconv"
	"strings"
)

func (s *Service) restaurantID(c echo.Context, approved bool) (int64, error) {
	var id int64
	where := ""
	if approved {
		where = " AND status='APPROVED'"
	}
	err := s.DB.QueryRow(c.Request().Context(), `SELECT id FROM restaurants WHERE owner_user_id=$1 AND deleted_at IS NULL`+where+` ORDER BY id LIMIT 1`, person(c).ID).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows && !approved {
			return 0, echo.NewHTTPError(404, "restaurant profile not found")
		}
		return 0, echo.NewHTTPError(403, "an approved restaurant owned by this account is required")
	}
	return id, nil
}
func (s *Service) riderID(c echo.Context) (int64, error) {
	var id int64
	err := s.DB.QueryRow(c.Request().Context(), `SELECT id FROM riders WHERE user_id=$1 AND deleted_at IS NULL`, person(c).ID).Scan(&id)
	if err != nil {
		return 0, echo.NewHTTPError(404, "rider profile not found")
	}
	return id, nil
}
func (s *Service) registerBusiness(api *echo.Group) {
	api.GET("/customer/profile", func(c echo.Context) error {
		return s.one(c, `SELECT `+project("users", "t")+` FROM users t WHERE id=$1`, person(c).ID)
	}, roles("CUSTOMER"))
	api.GET("/stations", func(c echo.Context) error {
		return s.list(c, `SELECT `+project("stations", "t")+` FROM stations t WHERE status='ACTIVE' AND deleted_at IS NULL ORDER BY name`)
	})
	api.GET("/customer/profile/:id", func(c echo.Context) error {
		if c.Param("id") != person(c).UUID {
			return echo.NewHTTPError(404, "profile not found")
		}
		return s.one(c, `SELECT `+project("users", "t")+` FROM users t WHERE id=$1`, person(c).ID)
	})
	api.PUT("/customer/profile", func(c echo.Context) error {
		var input struct {
			Name string `json:"full_name"`
		}
		if c.Bind(&input) != nil || !validName(input.Name, 160) {
			return bad("full name required")
		}
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), `UPDATE users SET full_name=$2 WHERE id=$1`, person(c).ID, input.Name); err != nil {
			return unavailable(err)
		}
		return commit(c, tx)
	}, roles("CUSTOMER"))
	addresses := api.Group("/customer/addresses", roles("CUSTOMER"))
	addresses.GET("", func(c echo.Context) error {
		return s.list(c, `SELECT `+project("user_addresses", "a")+`||jsonb_build_object('name',a.label,'detail',concat_ws(', ',a.address_line1,a.address_line2,a.city)) FROM user_addresses a WHERE a.user_id=$1 AND a.deleted_at IS NULL ORDER BY a.is_default DESC,a.created_at DESC`, person(c).ID)
	})
	saveAddress := func(c echo.Context) error {
		var input struct {
			Label     string  `json:"label"`
			Name      string  `json:"contact_name"`
			Phone     string  `json:"contact_phone"`
			Line1     string  `json:"address_line1"`
			Line2     string  `json:"address_line2"`
			City      string  `json:"city"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Default   bool    `json:"is_default"`
		}
		if c.Bind(&input) != nil || !validName(input.Label, 50) || strings.TrimSpace(input.Line1) == "" || !validName(input.City, 120) || input.Latitude < -90 || input.Latitude > 90 || input.Longitude < -180 || input.Longitude > 180 {
			return bad("valid address, city and coordinates required")
		}
		if input.Name == "" {
			input.Name = person(c).Name
		}
		if input.Phone == "" {
			input.Phone = person(c).Phone
		}
		tx, err := s.begin(c, "CREATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), `SELECT id FROM users WHERE id=$1 FOR UPDATE`, person(c).ID); err != nil {
			return err
		}
		if old := c.Param("id"); old != "" {
			if err = publicID(old); err != nil {
				return err
			}
			tag, e := tx.Exec(c.Request().Context(), `UPDATE user_addresses SET deleted_at=now(),is_default=false WHERE uuid=$1 AND user_id=$2 AND deleted_at IS NULL`, old, person(c).ID)
			if e != nil {
				return unavailable(e)
			}
			if tag.RowsAffected() != 1 {
				return echo.NewHTTPError(404, "address not found")
			}
		}
		if input.Default {
			if _, err = tx.Exec(c.Request().Context(), `UPDATE user_addresses SET is_default=false WHERE user_id=$1 AND deleted_at IS NULL`, person(c).ID); err != nil {
				return err
			}
		}
		var id string
		if err = tx.QueryRow(c.Request().Context(), `INSERT INTO user_addresses(user_id,label,contact_name,contact_phone,address_line1,address_line2,city,latitude,longitude,is_default) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,$10) RETURNING uuid`, person(c).ID, input.Label, input.Name, input.Phone, input.Line1, input.Line2, input.City, input.Latitude, input.Longitude, input.Default).Scan(&id); err != nil {
			return unavailable(err)
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return unavailable(err)
		}
		return c.JSON(200, map[string]string{"id": id})
	}
	addresses.POST("", saveAddress)
	addresses.PUT("/:id", saveAddress)
	addresses.DELETE("/:id", func(c echo.Context) error {
		if err := publicID(c.Param("id")); err != nil {
			return err
		}
		tx, err := s.begin(c, "DELETE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		tag, err := tx.Exec(c.Request().Context(), `UPDATE user_addresses SET deleted_at=now(),is_default=false WHERE uuid=$1 AND user_id=$2 AND deleted_at IS NULL`, c.Param("id"), person(c).ID)
		if err != nil {
			return unavailable(err)
		}
		if tag.RowsAffected() != 1 {
			return echo.NewHTTPError(404, "address not found")
		}
		return commit(c, tx)
	})
	api.POST("/onboarding/application", func(c echo.Context) error {
		var input struct {
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Phone       string  `json:"phone"`
			Address     string  `json:"address"`
			Station     string  `json:"station_id"`
			Latitude    float64 `json:"latitude"`
			Longitude   float64 `json:"longitude"`
			Image       string  `json:"image_url"`
			FSSAI       string  `json:"fssai_number"`
			Vehicle     string  `json:"vehicle_type"`
		}
		if c.Bind(&input) != nil || publicID(input.Station) != nil {
			return bad("active service station required")
		}
		tx, err := s.begin(c, "CREATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		var station int64
		if _, err = tx.Exec(c.Request().Context(), `SELECT id FROM users WHERE id=$1 FOR UPDATE`, person(c).ID); err != nil {
			return err
		}
		var existing string
		existingQuery := `SELECT uuid FROM restaurants WHERE owner_user_id=$1 AND deleted_at IS NULL ORDER BY id LIMIT 1`
		if person(c).Role == "DELIVERY_PARTNER" {
			existingQuery = `SELECT uuid FROM riders WHERE user_id=$1 AND deleted_at IS NULL`
		}
		if err = tx.QueryRow(c.Request().Context(), existingQuery, person(c).ID).Scan(&existing); err == nil {
			return c.JSON(200, map[string]string{"id": existing})
		} else if err != pgx.ErrNoRows {
			return err
		}
		if err = tx.QueryRow(c.Request().Context(), `SELECT id FROM stations WHERE uuid=$1 AND status='ACTIVE' AND deleted_at IS NULL`, input.Station).Scan(&station); err != nil {
			return bad("station is unavailable")
		}
		var id int64
		var uid string
		if person(c).Role == "RESTAURANT_OWNER" {
			if !validName(input.Name, 200) || strings.TrimSpace(input.Address) == "" || input.Latitude < -90 || input.Latitude > 90 || input.Longitude < -180 || input.Longitude > 180 {
				return bad("restaurant name, address and coordinates required")
			}
			var covers bool
			if err = tx.QueryRow(c.Request().Context(), `SELECT station_covers($1,$2,$3)`, station, input.Latitude, input.Longitude).Scan(&covers); err != nil || !covers {
				return bad("restaurant is outside the station service area")
			}
			if err = tx.QueryRow(c.Request().Context(), `INSERT INTO restaurants(owner_user_id,station_id,name,description,phone,address,latitude,longitude,image_url,fssai_number,status,is_open) VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,''),'PENDING',false) RETURNING id,uuid`, person(c).ID, station, input.Name, input.Description, input.Phone, input.Address, input.Latitude, input.Longitude, input.Image, input.FSSAI).Scan(&id, &uid); err != nil {
				return unavailable(err)
			}
			_, err = tx.Exec(c.Request().Context(), `INSERT INTO registration_fees(restaurant_id,amount_paise,currency,status) VALUES($1,20000,'INR','DUE')`, id)
		} else {
			if !validName(input.Vehicle, 50) {
				return bad("vehicle type required")
			}
			if err = tx.QueryRow(c.Request().Context(), `INSERT INTO riders(user_id,station_id,vehicle_type,verification_status,availability_status) VALUES($1,$2,$3,'PENDING','OFFLINE') RETURNING id,uuid`, person(c).ID, station, input.Vehicle).Scan(&id, &uid); err != nil {
				return unavailable(err)
			}
			_, err = tx.Exec(c.Request().Context(), `INSERT INTO registration_fees(delivery_agent_id,amount_paise,currency,status) VALUES($1,15000,'INR','DUE')`, id)
		}
		if err != nil {
			return unavailable(err)
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return unavailable(err)
		}
		return c.JSON(201, map[string]string{"id": uid})
	}, roles("RESTAURANT_OWNER", "DELIVERY_PARTNER"))
	restaurant := api.Group("/restaurant", roles("RESTAURANT_OWNER"))
	restaurant.GET("/profile", func(c echo.Context) error {
		id, err := s.restaurantID(c, false)
		if err != nil {
			return err
		}
		return s.one(c, `SELECT `+project("restaurants", "t")+` FROM restaurants t WHERE id=$1`, id)
	})
	rider := api.Group("/rider", roles("DELIVERY_PARTNER"))
	rider.GET("/profile", func(c echo.Context) error {
		id, err := s.riderID(c)
		if err != nil {
			return err
		}
		return s.one(c, `SELECT `+project("riders", "t")+` FROM riders t WHERE id=$1`, id)
	})
	restaurant.PATCH("/status", func(c echo.Context) error {
		var input struct {
			Open bool `json:"is_active"`
		}
		if c.Bind(&input) != nil {
			return bad("invalid availability")
		}
		id, err := s.restaurantID(c, true)
		if err != nil {
			return err
		}
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		tag, err := tx.Exec(c.Request().Context(), `UPDATE restaurants r SET is_open=$2 WHERE r.id=$1 AND (NOT $2 OR (EXISTS(SELECT 1 FROM registration_fees f WHERE f.restaurant_id=r.id AND f.status='PAID') AND EXISTS(SELECT 1 FROM stations s WHERE s.id=r.station_id AND s.status='ACTIVE' AND s.deleted_at IS NULL)))`, id, input.Open)
		if err != nil || tag.RowsAffected() != 1 {
			return bad("approval, paid registration and an active station are required")
		}
		return commit(c, tx)
	})
	rider.PATCH("/status", func(c echo.Context) error {
		var input struct {
			Status string `json:"status"`
		}
		if c.Bind(&input) != nil || (input.Status != "ONLINE" && input.Status != "OFFLINE") {
			return bad("ONLINE or OFFLINE required")
		}
		id, err := s.riderID(c)
		if err != nil {
			return err
		}
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		tag, err := tx.Exec(c.Request().Context(), `UPDATE riders r SET availability_status=$2 WHERE r.id=$1 AND ($2='OFFLINE' OR (r.verification_status='APPROVED' AND EXISTS(SELECT 1 FROM registration_fees f WHERE f.delivery_agent_id=r.id AND f.status='PAID') AND EXISTS(SELECT 1 FROM stations s WHERE s.id=r.station_id AND s.status='ACTIVE' AND s.deleted_at IS NULL)))`, id, input.Status)
		if err != nil || tag.RowsAffected() != 1 {
			return bad("approval, paid registration and an active station are required")
		}
		return commit(c, tx)
	})
	restaurant.GET("/menu/categories", func(c echo.Context) error {
		return s.list(c, `SELECT `+project("menu_categories", "t")+` FROM menu_categories t WHERE is_active AND deleted_at IS NULL ORDER BY sort_order,name`)
	})
	restaurant.GET("/menu/items", func(c echo.Context) error {
		id, err := s.restaurantID(c, false)
		if err != nil {
			return err
		}
		return s.list(c, `SELECT `+project("menu_items", "t")+`||jsonb_build_object('price',t.base_price) FROM menu_items t WHERE restaurant_id=$1 AND deleted_at IS NULL ORDER BY name`, id)
	})
	saveItem := func(c echo.Context) error {
		id, err := s.restaurantID(c, true)
		if err != nil {
			return err
		}
		var input struct {
			Name        string      `json:"name"`
			Description string      `json:"description"`
			Category    string      `json:"category_id"`
			Price       json.Number `json:"price"`
			BasePrice   json.Number `json:"base_price"`
			Image       string      `json:"image_url"`
			Minutes     *int        `json:"preparation_minutes"`
			Available   bool        `json:"is_available"`
		}
		if c.Bind(&input) != nil || !validName(input.Name, 180) || publicID(input.Category) != nil {
			return bad("item name and global category required")
		}
		price := input.BasePrice
		if price == "" {
			price = input.Price
		}
		p, err := strconv.ParseFloat(string(price), 64)
		if err != nil || p < 0 || p > 9999999999.99 {
			return bad("valid item price required")
		}
		tx, err := s.begin(c, "CREATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		var category int64
		if err = tx.QueryRow(c.Request().Context(), `SELECT id FROM menu_categories WHERE uuid=$1 AND is_active AND deleted_at IS NULL`, input.Category).Scan(&category); err != nil {
			return bad("active global category required")
		}
		uid := c.Param("id")
		if uid == "" {
			err = tx.QueryRow(c.Request().Context(), `INSERT INTO menu_items(restaurant_id,category_id,name,description,base_price,image_url,preparation_minutes,is_available) VALUES($1,$2,$3,$4,$5::text::numeric,NULLIF($6,''),$7,$8) RETURNING uuid`, id, category, input.Name, input.Description, string(price), input.Image, input.Minutes, input.Available).Scan(&uid)
		} else {
			if err = publicID(uid); err != nil {
				return err
			}
			var found string
			err = tx.QueryRow(c.Request().Context(), `UPDATE menu_items SET category_id=$3,name=$4,description=$5,base_price=$6::text::numeric,image_url=NULLIF($7,''),preparation_minutes=$8,is_available=$9 WHERE uuid=$1 AND restaurant_id=$2 AND deleted_at IS NULL RETURNING uuid`, uid, id, category, input.Name, input.Description, string(price), input.Image, input.Minutes, input.Available).Scan(&found)
		}
		if err != nil {
			return unavailable(err)
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return unavailable(err)
		}
		return c.JSON(200, map[string]string{"id": uid})
	}
	restaurant.POST("/menu/items", saveItem)
	restaurant.PUT("/menu/items/:id", saveItem)
	restaurant.PATCH("/menu/items/:id/availability", func(c echo.Context) error {
		id, err := s.restaurantID(c, true)
		if err != nil {
			return err
		}
		if err = publicID(c.Param("id")); err != nil {
			return err
		}
		var input struct {
			Available bool `json:"is_available"`
		}
		if c.Bind(&input) != nil {
			return bad("availability required")
		}
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		tag, err := tx.Exec(c.Request().Context(), `UPDATE menu_items SET is_available=$3 WHERE uuid=$1 AND restaurant_id=$2 AND deleted_at IS NULL`, c.Param("id"), id, input.Available)
		if err != nil {
			return unavailable(err)
		}
		if tag.RowsAffected() != 1 {
			return echo.NewHTTPError(404, "item not found")
		}
		return commit(c, tx)
	})
	restaurant.DELETE("/menu/items/:id", func(c echo.Context) error {
		id, err := s.restaurantID(c, true)
		if err != nil {
			return err
		}
		if err = publicID(c.Param("id")); err != nil {
			return err
		}
		tx, err := s.begin(c, "DELETE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		tag, err := tx.Exec(c.Request().Context(), `UPDATE menu_items SET deleted_at=now(),is_available=false WHERE uuid=$1 AND restaurant_id=$2 AND deleted_at IS NULL`, c.Param("id"), id)
		if err != nil {
			return unavailable(err)
		}
		if tag.RowsAffected() != 1 {
			return echo.NewHTTPError(404, "item not found")
		}
		return commit(c, tx)
	})
	restaurant.GET("/menu/items/:id/variants", func(c echo.Context) error {
		id, err := s.restaurantID(c, false)
		if err != nil {
			return err
		}
		if err = publicID(c.Param("id")); err != nil {
			return err
		}
		return s.list(c, `SELECT `+project("item_variants", "v")+` FROM item_variants v JOIN menu_items i ON i.id=v.menu_item_id WHERE i.uuid=$1 AND i.restaurant_id=$2 AND v.deleted_at IS NULL ORDER BY v.name`, c.Param("id"), id)
	})
	saveVariant := func(c echo.Context) error {
		id, err := s.restaurantID(c, true)
		if err != nil {
			return err
		}
		var input struct {
			Name      string      `json:"name"`
			Delta     json.Number `json:"price_delta"`
			Available bool        `json:"is_available"`
		}
		if c.Bind(&input) != nil || !validName(input.Name, 120) {
			return bad("variant name and price delta required")
		}
		delta, err := strconv.ParseFloat(string(input.Delta), 64)
		if err != nil {
			return bad("valid variant price delta required")
		}
		if err = publicID(c.Param("id")); err != nil {
			return err
		}
		tx, err := s.begin(c, "CREATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		var item int64
		if err = tx.QueryRow(c.Request().Context(), `SELECT id FROM menu_items WHERE uuid=$1 AND restaurant_id=$2 AND deleted_at IS NULL AND base_price+$3>=0 FOR UPDATE`, c.Param("id"), id, delta).Scan(&item); err != nil {
			return bad("owned menu item and nonnegative resulting price required")
		}
		variant := c.Param("variant")
		if variant == "" {
			err = tx.QueryRow(c.Request().Context(), `INSERT INTO item_variants(menu_item_id,name,price_delta,is_available) VALUES($1,$2,$3,$4) RETURNING uuid`, item, input.Name, delta, input.Available).Scan(&variant)
		} else {
			if err = publicID(variant); err != nil {
				return err
			}
			var found string
			err = tx.QueryRow(c.Request().Context(), `UPDATE item_variants SET name=$3,price_delta=$4,is_available=$5 WHERE uuid=$1 AND menu_item_id=$2 AND deleted_at IS NULL RETURNING uuid`, variant, item, input.Name, delta, input.Available).Scan(&found)
		}
		if err != nil {
			return unavailable(err)
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return err
		}
		return c.JSON(200, map[string]string{"id": variant})
	}
	restaurant.POST("/menu/items/:id/variants", saveVariant)
	restaurant.PUT("/menu/items/:id/variants/:variant", saveVariant)
}

package platform

import (
	"encoding/json"
	"fmt"
	"github.com/imtiyazkumar/yember-core/internal/contractschema"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type column struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
	Type       string `json:"sql_type"`
}
type table struct {
	Name    string   `json:"table"`
	Columns []column `json:"columns"`
}

var tables = func() map[string]table {
	var contract struct {
		Tables []table `json:"tables"`
	}
	if err := json.Unmarshal(contractschema.Contract, &contract); err != nil {
		panic(err)
	}
	result := map[string]table{}
	for _, t := range contract.Tables {
		result[t.Name] = t
	}
	return result
}()
var references = map[string]string{"user_id": "users", "owner_user_id": "users", "restaurant_id": "restaurants", "station_id": "stations", "category_id": "menu_categories", "menu_item_id": "menu_items", "variant_id": "item_variants", "cart_id": "carts", "order_id": "orders", "address_id": "user_addresses", "rider_id": "riders", "delivery_agent_id": "riders", "payment_intent_id": "payment_intents", "payment_id": "payments", "restaurant_earning_id": "restaurant_earnings", "agent_earning_id": "delivery_agent_earnings", "delivery_agent_earning_id": "delivery_agent_earnings", "restaurant_payout_id": "restaurant_payouts", "agent_payout_id": "delivery_agent_payouts", "delivery_agent_payout_id": "delivery_agent_payouts", "registration_fee_id": "registration_fees", "bank_details_id": "delivery_agent_bank_details", "created_by": "users", "approved_by": "users", "verified_by": "users", "changed_by": "users", "replaced_by_token_id": "refresh_tokens", "coupon_id": "coupons"}
var resources = map[string]string{"customers": "users", "users": "users", "restaurants": "restaurants", "partners": "riders", "orders": "orders", "payments": "payments", "coupons": "coupons", "audit": "system_activity_logs", "stations": "stations", "menu-categories": "menu_categories", "menu-items": "menu_items", "item-variants": "item_variants", "restaurant-documents": "restaurant_documents", "rider-documents": "rider_documents", "restaurant-banks": "restaurant_bank_accounts", "rider-banks": "delivery_agent_bank_details", "payment-intents": "payment_intents", "registration-fees": "registration_fees", "restaurant-earnings": "restaurant_earnings", "rider-earnings": "delivery_agent_earnings", "restaurant-payouts": "restaurant_payouts", "rider-payouts": "delivery_agent_payouts", "cash-collections": "cash_collections", "financial-transactions": "financial_transactions", "refunds": "refunds", "reconciliation-runs": "reconciliation_runs"}

var editableTables = map[string]bool{"stations": true, "menu_categories": true, "menu_items": true, "item_variants": true, "user_addresses": true, "restaurants": true, "riders": true, "users": true, "coupons": true, "restaurant_documents": true, "rider_documents": true}

func init() {
	for name := range tables {
		resources[strings.ReplaceAll(name, "_", "-")] = name
	}
}

func project(tableName, alias string) string {
	pairs := []string{"'id'," + alias + ".uuid", "'uuid'," + alias + ".uuid"}
	for _, c := range tables[tableName].Columns {
		if c.Name == "id" || c.Name == "uuid" || strings.Contains(c.Name, "hash") || strings.Contains(c.Name, "encrypted") || c.Name == "payload_json" {
			continue
		}
		expr := alias + "." + c.Name
		if target := references[c.Name]; target != "" {
			expr = "(SELECT uuid FROM " + target + " WHERE id=" + expr + ")"
		}
		if c.Name == "actor_id" {
			expr = "(SELECT uuid FROM users WHERE id=" + expr + ")"
		}
		if c.Name == "entity_id" {
			expr = alias + ".entity_uuid"
		}
		pairs = append(pairs, "'"+c.Name+"',"+expr)
	}
	extras := map[string]string{}
	switch tableName {
	case "users":
		extras["name"] = "COALESCE(" + alias + ".full_name,'User')"
	case "riders":
		extras["name"] = "(SELECT full_name FROM users WHERE id=" + alias + ".user_id)"
		extras["phone"] = "(SELECT phone FROM users WHERE id=" + alias + ".user_id)"
		extras["status"] = alias + ".verification_status"
		extras["approval_status"] = alias + ".verification_status"
		extras["availability"] = alias + ".availability_status"
		extras["fee_payment_status"] = "COALESCE((SELECT status FROM registration_fees WHERE delivery_agent_id=" + alias + ".id),'DUE')"
	case "restaurants":
		extras["approval_status"] = alias + ".status"
		extras["is_active"] = alias + ".is_open"
		extras["fee_payment_status"] = "COALESCE((SELECT status FROM registration_fees WHERE restaurant_id=" + alias + ".id),'DUE')"
	case "orders":
		extras["name"] = alias + ".order_number"
		extras["total_amount"] = "round(" + alias + ".total_amount*100)::bigint"
		extras["restaurant"] = "(SELECT name FROM restaurants WHERE id=" + alias + ".restaurant_id)"
		extras["customer"] = "(SELECT full_name FROM users WHERE id=" + alias + ".user_id)"
	case "coupons":
		extras["name"] = alias + ".code"
		extras["status"] = "CASE WHEN " + alias + ".is_active THEN 'ACTIVE' ELSE 'INACTIVE' END"
		extras["used_count"] = "(SELECT count(*) FROM coupon_usages WHERE coupon_id=" + alias + ".id)"
		extras["min_order_amount"] = alias + ".min_order_amount_paise"
		extras["max_discount_amount"] = alias + ".max_discount_amount_paise"
	case "payments":
		extras["name"] = "'Payment #'||left(" + alias + ".uuid::text,8)"
		extras["amount"] = alias + ".amount_paise"
	case "system_activity_logs":
		extras["name"] = alias + ".action"
		extras["actor"] = "(SELECT full_name FROM users WHERE id=" + alias + ".actor_id)"
		extras["target_type"] = alias + ".entity_type"
		extras["target_id"] = alias + ".entity_uuid"
	}
	base := "jsonb_build_object(" + strings.Join(pairs, ",") + ")"
	if len(extras) > 0 {
		extra := []string{}
		for name, expr := range extras {
			extra = append(extra, "'"+name+"',"+expr)
		}
		base += "||jsonb_build_object(" + strings.Join(extra, ",") + ")"
	}
	return base
}
func readBody(c echo.Context) (map[string]any, error) {
	data := map[string]any{}
	decoder := json.NewDecoder(io.LimitReader(c.Request().Body, 1<<20))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		return nil, bad("invalid JSON request")
	}
	return data, nil
}
func numeric(value any) (int64, error) {
	switch v := value.(type) {
	case json.Number:
		return v.Int64()
	case string:
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, fmt.Errorf("integer required")
	}
}
func fieldString(data map[string]any, key string) string {
	value, _ := data[key].(string)
	return strings.TrimSpace(value)
}
func (s *Service) registerAdmin(group *echo.Group) {
	admin := group.Group("/admin", roles("ADMIN"))
	admin.GET("/dashboard/stats", func(c echo.Context) error {
		return s.one(c, `SELECT jsonb_build_object('users',(SELECT count(*) FROM users WHERE deleted_at IS NULL),'restaurants',(SELECT count(*) FROM restaurants WHERE deleted_at IS NULL),'riders',(SELECT count(*) FROM riders WHERE deleted_at IS NULL),'orders',(SELECT count(*) FROM orders WHERE deleted_at IS NULL),'pending_approvals',(SELECT count(*) FROM restaurants WHERE status='PENDING' AND deleted_at IS NULL)+(SELECT count(*) FROM riders WHERE verification_status='PENDING' AND deleted_at IS NULL),'payment_exceptions',(SELECT count(*) FROM reconciliation_runs WHERE status='EXCEPTIONS'))`)
	})
	admin.GET("/contract", func(c echo.Context) error {
		type resource struct {
			Name      string         `json:"resource"`
			Table     string         `json:"table"`
			Columns   []column       `json:"columns"`
			Editable  bool           `json:"editable"`
			Deletable bool           `json:"deletable"`
			Filters   []recordFilter `json:"filters"`
		}
		result := []resource{}
		for _, t := range tables {
			name := strings.ReplaceAll(t.Name, "_", "-")
			result = append(result, resource{Name: name, Table: t.Name, Columns: t.Columns, Editable: editableTables[t.Name], Deletable: editableTables[t.Name] && t.Name != "users", Filters: listFilters(t.Name)})
		}
		sort.Slice(result, func(i, j int) bool { return result[i].Table < result[j].Table })
		return c.JSON(200, result)
	})
	s.registerFilterOptions(admin)
	admin.GET("/manage/:resource", func(c echo.Context) error {
		resource := c.Param("resource")
		name, ok := resources[resource]
		if !ok {
			return echo.NewHTTPError(404, "resource not found")
		}
		limit, _ := strconv.Atoi(c.QueryParam("limit"))
		if limit < 1 || limit > 100 {
			limit = 25
		}
		page, _ := strconv.Atoi(c.QueryParam("page"))
		if page < 1 {
			page = 1
		}
		if page > 100000 {
			return bad("page too large")
		}
		where, args, err := recordWhere(name, c.QueryParams())
		if err != nil {
			return err
		}
		expr := adminProject(name, "t")
		limitParam := fmt.Sprintf("$%d", len(args)+1)
		offsetParam := fmt.Sprintf("$%d", len(args)+2)
		pageParam := fmt.Sprintf("$%d", len(args)+3)
		args = append(args, limit, (page-1)*limit, page)
		return s.one(c, `SELECT jsonb_build_object('items',COALESCE((SELECT jsonb_agg(x.record) FROM (SELECT `+expr+` AS record FROM `+name+` t WHERE `+where+` ORDER BY t.created_at DESC,t.id LIMIT `+limitParam+` OFFSET `+offsetParam+`)x),'[]'::jsonb),'total',(SELECT count(*) FROM `+name+` t WHERE `+where+`),'page',`+pageParam+`::int,'limit',`+limitParam+`::int)`, args...)

	})
	admin.GET("/manage/:resource/:id", func(c echo.Context) error {
		name, ok := resources[c.Param("resource")]
		if !ok {
			return echo.NewHTTPError(404, "resource not found")
		}
		if err := publicID(c.Param("id")); err != nil {
			return err
		}
		return s.one(c, `SELECT jsonb_build_object('record',`+adminProject(name, "t")+`) FROM `+name+` t WHERE t.uuid=$1 AND t.deleted_at IS NULL`, c.Param("id"))
	})
	editable := editableTables
	save := func(c echo.Context) error {
		name, ok := resources[c.Param("resource")]
		if !ok || !editable[name] {
			return echo.NewHTTPError(405, "this entity is managed through its operational workflow")
		}
		data, err := readBody(c)
		if err != nil {
			return err
		}
		reason := fieldString(data, "reason")
		if !validName(reason, 2000) {
			return bad("an audit reason is required")
		}
		delete(data, "reason")
		id := c.Param("id")
		if id != "" {
			if err = publicID(id); err != nil {
				return err
			}
		}
		if c.Param("resource") == "customers" {
			data["role"] = "CUSTOMER"
		}
		if name == "users" {
			if value, ok := data["name"]; ok {
				data["full_name"] = value
				delete(data, "name")
			}
			if id == "" {
				if _, ok := data["role"]; !ok {
					data["role"] = "CUSTOMER"
				}
				if _, ok := data["status"]; !ok {
					data["status"] = "ACTIVE"
				}
			}
		}
		if name == "coupons" {
			for old, next := range map[string]string{"min_order_amount": "min_order_amount_paise", "max_discount_amount": "max_discount_amount_paise"} {
				if value, ok := data[old]; ok {
					if next == "max_discount_amount_paise" && fmt.Sprint(value) == "0" {
						value = nil
					}
					data[next] = value
					delete(data, old)
				}
			}
			if _, present := data["code"]; present {
				code := strings.ToUpper(fieldString(data, "code"))
				if len(code) < 3 || len(code) > 50 {
					return bad("coupon code must contain 3–50 characters")
				}
				data["code"] = code
			} else if id == "" {
				return bad("coupon code required")
			}
		}
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), `SELECT set_config('yember.reason',$1,true)`, reason); err != nil {
			return err
		}
		if _, err = tx.Exec(c.Request().Context(), `SELECT pg_advisory_xact_lock(729401)`); err != nil {
			return err
		}
		if name == "users" && id != "" {
			var currentRole string
			if err = tx.QueryRow(c.Request().Context(), `SELECT role FROM users WHERE uuid=$1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&currentRole); err != nil {
				return unavailable(err)
			}
			roleChange := fieldString(data, "role")
			stateChange := fieldString(data, "status")
			if currentRole == "ADMIN" && ((roleChange != "" && roleChange != "ADMIN") || (stateChange != "" && stateChange != "ACTIVE")) {
				if id == person(c).UUID {
					return bad("you cannot disable or demote your own administrator account")
				}
				var others int
				if err = tx.QueryRow(c.Request().Context(), `SELECT count(*) FROM users WHERE role='ADMIN' AND status='ACTIVE' AND deleted_at IS NULL AND uuid<>$1`, id).Scan(&others); err != nil {
					return err
				}
				if others == 0 {
					return bad("at least one active administrator is required")
				}
			}
		}
		columns := []string{}
		values := []string{}
		args := []any{}
		for _, col := range tables[name].Columns {
			value, present := data[col.Name]
			if !present {
				continue
			}
			if col.Name == "id" || col.Name == "uuid" || col.Name == "created_at" || col.Name == "updated_at" || col.Name == "deleted_at" || strings.Contains(col.Name, "hash") || strings.Contains(col.Name, "encrypted") {
				return bad("field is not directly editable: " + col.Name)
			}
			delete(data, col.Name)
			if target := references[col.Name]; target != "" {
				if value != nil && value != "" {
					text, ok := value.(string)
					if !ok || publicID(text) != nil {
						return bad("a related public UUID is required")
					}
					var related int64
					if err = tx.QueryRow(c.Request().Context(), `SELECT id FROM `+target+` WHERE uuid=$1 AND deleted_at IS NULL`, text).Scan(&related); err != nil {
						return bad("related record not found")
					}
					value = related
				} else {
					value = nil
				}
			}
			if col.Type == "bigint" && references[col.Name] == "" && value != nil {
				parsed, e := numeric(value)
				if e != nil {
					return bad(col.Name + " must be an integer")
				}
				value = parsed
			}
			if col.Type == "int" || col.Type == "integer" {
				if value != nil {
					parsed, e := numeric(value)
					if e != nil {
						return bad(col.Name + " must be an integer")
					}
					value = parsed
				}
			}
			columns = append(columns, col.Name)
			if value != nil && strings.HasPrefix(col.Type, "numeric") {
				text := fmt.Sprint(value)
				number, e := strconv.ParseFloat(text, 64)
				if e != nil || math.IsNaN(number) || math.IsInf(number, 0) {
					return bad(col.Name + " must be numeric")
				}
				value = text
			}
			if value != nil && col.Type == "timestamptz" {
				text, ok := value.(string)
				if !ok {
					return bad(col.Name + " must be an RFC3339 timestamp")
				}
				instant, e := time.Parse(time.RFC3339, text)
				if e != nil {
					return bad(col.Name + " must be an RFC3339 timestamp")
				}
				value = instant
			}
			args = append(args, value)
			expression := fmt.Sprintf("$%d", len(args))
			if strings.HasPrefix(col.Type, "numeric") {
				expression += "::text::numeric"
			}
			values = append(values, expression)
		}
		password := fieldString(data, "password")
		delete(data, "password")
		if password != "" {
			if name != "users" || len(password) < 10 || len(password) > 72 {
				return bad("invalid password")
			}
			hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if e != nil {
				return e
			}
			columns = append(columns, "password_hash")
			args = append(args, string(hash))
			values = append(values, fmt.Sprintf("$%d", len(args)))
		}
		if len(data) > 0 {
			return bad("request includes fields outside the schema")
		}
		if len(columns) == 0 {
			return bad("no changes supplied")
		}
		if id == "" {
			err = tx.QueryRow(c.Request().Context(), `INSERT INTO `+name+` (`+strings.Join(columns, ",")+`) VALUES (`+strings.Join(values, ",")+`) RETURNING uuid`, args...).Scan(&id)
		} else {
			set := []string{}
			for i, col := range columns {
				set = append(set, col+"="+values[i])
			}
			args = append(args, id)
			var found string
			err = tx.QueryRow(c.Request().Context(), `UPDATE `+name+` SET `+strings.Join(set, ",")+` WHERE uuid=$`+strconv.Itoa(len(args))+` AND deleted_at IS NULL RETURNING uuid`, args...).Scan(&found)
		}
		if err != nil {
			return unavailable(err)
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return unavailable(err)
		}
		return c.JSON(200, map[string]string{"id": id})
	}
	admin.POST("/manage/:resource", save)
	admin.PUT("/manage/:resource/:id", save)
	admin.DELETE("/manage/:resource/:id", func(c echo.Context) error {
		name, ok := resources[c.Param("resource")]
		if !ok || !editable[name] || name == "users" {
			return echo.NewHTTPError(405, "deletion is not available for this entity")
		}
		if err := publicID(c.Param("id")); err != nil {
			return err
		}
		data, err := readBody(c)
		if err != nil {
			return err
		}
		if !validName(fieldString(data, "reason"), 2000) {
			return bad("audit reason required")
		}
		tx, err := s.begin(c, "DELETE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), `SELECT set_config('yember.reason',$1,true)`, fieldString(data, "reason")); err != nil {
			return err
		}
		result, err := tx.Exec(c.Request().Context(), `UPDATE `+name+` SET deleted_at=now() WHERE uuid=$1 AND deleted_at IS NULL`, c.Param("id"))
		if err != nil {
			return unavailable(err)
		}
		if result.RowsAffected() != 1 {
			return echo.NewHTTPError(404, "record not found")
		}
		return commit(c, tx)
	})
}

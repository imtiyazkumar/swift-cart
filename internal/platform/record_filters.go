package platform

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// Filter definitions are both the admin UI contract and the SQL allowlist.
// Paths contain only schema-owned identifiers, never request input.
type recordFilter struct {
	Field    string `json:"field"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	Resource string `json:"resource,omitempty"`
	path     []string
	column   string
}

var filterParents = map[string]string{
	"item_variants": "menu_item_id", "cart_items": "cart_id", "order_items": "order_id",
	"order_status_history": "order_id", "restaurant_documents": "restaurant_id",
	"restaurant_bank_accounts": "restaurant_id", "rider_documents": "rider_id",
	"delivery_agent_bank_details": "rider_id", "rider_locations": "rider_id",
	"delivery_assignments": "order_id", "payments": "order_id", "payment_intents": "order_id",
	"registration_fees": "restaurant_id", "restaurant_earnings": "order_id",
	"delivery_agent_earnings": "order_id", "restaurant_payouts": "restaurant_id",
	"delivery_agent_payouts": "delivery_agent_id", "restaurant_payout_allocations": "restaurant_earning_id",
	"delivery_agent_payout_allocations": "delivery_agent_earning_id", "cash_collections": "order_id",
	"refunds": "order_id", "financial_transactions": "order_id", "coupon_usages": "order_id",
	"orders": "restaurant_id", "carts": "restaurant_id", "menu_items": "restaurant_id",
	"restaurants": "station_id", "riders": "station_id",
}

func filterLabel(field string) string {
	labels := map[string]string{"user_id": "Customer / user", "owner_user_id": "Owner", "rider_id": "Delivery agent", "delivery_agent_id": "Delivery agent", "station_id": "Service area", "category_id": "Category", "menu_item_id": "Menu item", "variant_id": "Menu variant", "actor_id": "Actor", "created_from": "Created from", "created_to": "Created through"}
	if label := labels[field]; label != "" {
		return label
	}
	return strings.ToUpper(field[:1]) + strings.ReplaceAll(strings.TrimSuffix(field[1:], "_id"), "_", " ")
}

func filterTarget(field string) string {
	if field == "actor_id" {
		return "users"
	}
	return references[field]
}

func listFilters(name string) []recordFilter {
	result := []recordFilter{}
	seen := map[string]bool{}
	add := func(field, kind, target, column string, path []string) {
		if seen[field] {
			return
		}
		seen[field] = true
		resource := strings.ReplaceAll(target, "_", "-")
		result = append(result, recordFilter{Field: field, Label: filterLabel(field), Kind: kind, Resource: resource, column: column, path: append([]string(nil), path...)})
	}
	for _, col := range tables[name].Columns {
		if target := filterTarget(col.Name); target != "" {
			// Token replacement chains are technical session references, not useful facets.
			if col.Name != "replaced_by_token_id" {
				add(col.Name, "entity", target, col.Name, nil)
			}
		} else if col.Type == "boolean" {
			add(col.Name, "boolean", "", col.Name, nil)
		} else if isChoiceColumn(col.Name) {
			add(col.Name, "choice", "", col.Name, nil)
		}
	}
	// Inherit operational context along the owning parent, e.g. variant -> item
	// -> restaurant -> station, or refund -> order -> restaurant -> station.
	path := []string{}
	parent := name
	for depth := 0; depth < 4; depth++ {
		link := filterParents[parent]
		if link == "" {
			break
		}
		target := filterTarget(link)
		if target == "" {
			break
		}
		path = append(path, link)
		parent = target
		for _, col := range tables[parent].Columns {
			target := filterTarget(col.Name)
			if target == "stations" || target == "restaurants" || target == "orders" || target == "menu_categories" || target == "menu_items" || target == "riders" || (col.Name == "user_id") {
				add(col.Name, "entity", target, col.Name, path)
			}
		}
	}
	// Delivery assignments are the reverse side of an order relationship.
	orderPath := []string(nil)
	hasOrder := name == "orders"
	if !hasOrder {
		for _, col := range tables[name].Columns {
			if col.Name == "order_id" {
				orderPath = []string{"order_id"}
				hasOrder = true
				break
			}
		}
	}
	if hasOrder && !seen["rider_id"] && !seen["delivery_agent_id"] {
		add("delivery_agent_id", "entity", "riders", "rider_id", append(orderPath, "@delivery_assignments:order_id"))
	}
	if name == "orders" {
		add("menu_item_id", "entity", "menu_items", "menu_item_id", []string{"@order_items:order_id"})
		add("variant_id", "entity", "item_variants", "variant_id", []string{"@order_items:order_id"})
		add("category_id", "entity", "menu_categories", "category_id", []string{"@order_items:order_id", "menu_item_id"})
	}
	if name == "menu_categories" {
		add("restaurant_id", "entity", "restaurants", "restaurant_id", []string{"@menu_items:category_id"})
		add("station_id", "entity", "stations", "station_id", []string{"@menu_items:category_id", "restaurant_id"})
	}
	// Line-level records also inherit the selected menu item's restaurant/category.
	for _, col := range tables[name].Columns {
		if col.Name == "menu_item_id" {
			add("category_id", "entity", "menu_categories", "category_id", []string{"menu_item_id"})
			add("restaurant_id", "entity", "restaurants", "restaurant_id", []string{"menu_item_id"})
		}
	}
	add("created_from", "date", "", "created_at", nil)
	add("created_to", "date", "", "created_at", nil)
	return result
}

func isChoiceColumn(name string) bool {
	switch name {
	case "status", "verification_status", "availability_status", "processing_status", "role", "purpose", "method", "stage", "provider", "document_type", "vehicle_type", "transaction_type", "account_code", "direction", "source", "actor_type", "action", "entity_type", "event_type", "discount_type", "from_status", "to_status", "received_by":
		return true
	}
	return false
}

func filterExpression(name string, filter recordFilter, predicate string) string {
	var walk func(string, string, int) string
	walk = func(table, alias string, depth int) string {
		if depth == len(filter.path) {
			if filter.Kind == "entity" {
				return alias + "." + filter.column + "=(SELECT id FROM " + resources[filter.Resource] + " WHERE uuid=" + predicate + "::uuid)"
			}
			return alias + "." + filter.column + predicate
		}
		field := filter.path[depth]
		if strings.HasPrefix(field, "@") {
			parts := strings.SplitN(field[1:], ":", 2)
			next := fmt.Sprintf("filter_parent_%d", depth)
			return "EXISTS(SELECT 1 FROM " + parts[0] + " " + next + " WHERE " + next + "." + parts[1] + "=" + alias + ".id AND " + next + ".deleted_at IS NULL AND " + walk(parts[0], next, depth+1) + ")"
		}
		target := filterTarget(field)
		next := fmt.Sprintf("filter_parent_%d", depth)
		return "EXISTS(SELECT 1 FROM " + target + " " + next + " WHERE " + next + ".id=" + alias + "." + field + " AND " + walk(target, next, depth+1) + ")"
	}
	return walk(name, "t", 0)
}

func recordWhere(name string, params url.Values) (string, []any, error) {
	definitions := map[string]recordFilter{}
	for _, filter := range listFilters(name) {
		definitions[filter.Field] = filter
	}
	where := "t.deleted_at IS NULL"
	args := []any{}
	bind := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	if search := strings.TrimSpace(params.Get("q")); search != "" {
		if len(search) > 200 {
			return "", nil, bad("search must be 200 characters or fewer")
		}
		where += " AND (" + adminProject(name, "t") + ")::text ILIKE '%'||" + bind(search) + "||'%'"
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var from, to time.Time
	for _, key := range keys {
		field := ""
		if strings.HasPrefix(key, "filter[") && strings.HasSuffix(key, "]") {
			field = strings.TrimSuffix(strings.TrimPrefix(key, "filter["), "]")
		} else if key == "status" {
			field = key
		} else {
			continue
		}
		filter, ok := definitions[field]
		if !ok {
			return "", nil, bad("unsupported filter: " + field)
		}
		if len(params[key]) != 1 {
			return "", nil, bad("filter must have one value: " + field)
		}
		value := strings.TrimSpace(params.Get(key))
		if value == "" {
			continue
		}
		if len(value) > 200 {
			return "", nil, bad("filter value too long")
		}
		predicate := ""
		switch filter.Kind {
		case "entity":
			if err := publicID(value); err != nil {
				return "", nil, bad(filter.Label + " must be a public UUID")
			}
			predicate = bind(value)
		case "boolean":
			if value != "true" && value != "false" {
				return "", nil, bad(filter.Label + " must be true or false")
			}
			predicate = "=" + bind(value) + "::boolean"
		case "date":
			day, err := time.Parse("2006-01-02", value)
			if err != nil {
				return "", nil, bad(filter.Label + " must use YYYY-MM-DD")
			}
			// Admin calendar dates follow the operational India timezone. The upper
			// bound is exclusive at the next midnight, so the whole final day matches.
			zone := time.FixedZone("IST", 19800)
			day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, zone)
			if field == "created_from" {
				from = day
				predicate = ">=" + bind(day)
			} else {
				to = day
				predicate = "<" + bind(day.AddDate(0, 0, 1))
			}
		default:
			predicate = "::text=" + bind(value)
		}
		where += " AND " + filterExpression(name, filter, predicate)
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return "", nil, bad("created from must not be after created through")
	}
	return where, args, nil
}

func (s *Service) registerFilterOptions(admin *echo.Group) {
	admin.GET("/manage/:resource/filter-options", func(c echo.Context) error {
		name, ok := resources[c.Param("resource")]
		if !ok {
			return echo.NewHTTPError(404, "resource not found")
		}
		for _, filter := range listFilters(name) {
			if filter.Field != c.QueryParam("field") || filter.Kind != "choice" {
				continue
			}
			// Column identifiers came from the allowlist; search is bound separately.
			return s.one(c, `SELECT jsonb_build_object('items',COALESCE(jsonb_agg(value ORDER BY value),'[]'::jsonb)) FROM (SELECT DISTINCT t.`+filter.column+`::text AS value FROM `+name+` t WHERE t.deleted_at IS NULL AND t.`+filter.column+` IS NOT NULL AND t.`+filter.column+`::text ILIKE '%'||$1||'%' ORDER BY value LIMIT 100) choices`, strings.TrimSpace(c.QueryParam("q")))
		}
		return bad("unsupported choice filter")
	})
}

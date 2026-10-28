package platform

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAdminListFiltersAcrossAllResources(t *testing.T) {
	f := newWorkflow(t)
	ctx := context.Background()
	order := f.checkout(t, "PAY_ON_DELIVERY")
	for _, status := range []string{"ACCEPTED", "PREPARING", "READY_FOR_PICKUP"} {
		f.call(t, "RESTAURANT_OWNER", "PATCH", "/restaurant/orders/"+order+"/status", fmt.Sprintf(`{"status":%q}`, status), 204)
	}
	var station, category, variant string
	for _, row := range []struct {
		sql string
		out *string
	}{
		{`SELECT uuid FROM stations LIMIT 1`, &station},
		{`SELECT uuid FROM menu_categories LIMIT 1`, &category},
		{`INSERT INTO item_variants(menu_item_id,name,price_delta,is_available) SELECT id,'Large',20,true FROM menu_items WHERE uuid=$1 RETURNING uuid`, &variant},
	} {
		var err error
		if row.out == &variant {
			err = f.p.QueryRow(ctx, row.sql, f.item).Scan(row.out)
		} else {
			err = f.p.QueryRow(ctx, row.sql).Scan(row.out)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	check := func(resource string, filters map[string]string, want int) {
		t.Helper()
		params := url.Values{}
		for field, value := range filters {
			params.Set("filter["+field+"]", value)
		}
		page := f.call(t, "ADMIN", "GET", "/admin/manage/"+resource+"?"+params.Encode(), "", 200)
		if int(page["total"].(float64)) != want || len(page["items"].([]any)) != want {
			t.Fatalf("%s %v: %v", resource, filters, page)
		}
	}
	check("menu-items", map[string]string{"station_id": station, "restaurant_id": f.restaurant, "category_id": category, "is_available": "true"}, 1)
	check("item-variants", map[string]string{"station_id": station, "restaurant_id": f.restaurant, "category_id": category, "menu_item_id": f.item}, 1)
	check("order-items", map[string]string{"order_id": order, "restaurant_id": f.restaurant, "category_id": category, "station_id": station, "user_id": f.users["CUSTOMER"].UUID}, 1)
	check("orders", map[string]string{"restaurant_id": f.restaurant, "delivery_agent_id": f.rider}, 1)
	check("orders", map[string]string{"restaurant_id": f.restaurant, "delivery_agent_id": f.users["ADMIN"].UUID}, 0)
	check("orders", map[string]string{"category_id": category, "menu_item_id": f.item}, 1)
	check("menu-categories", map[string]string{"restaurant_id": f.restaurant, "station_id": station}, 1)
	check("menu-items", map[string]string{"is_available": "false"}, 0)
	check("menu-items", map[string]string{"restaurant_id": f.users["ADMIN"].UUID}, 0)
	// Filter predicates must parse for every advertised field, including empty tables.
	for name := range tables {
		filters := listFilters(name)
		if len(filters) < 2 {
			t.Fatalf("%s has no date filters", name)
		}
		seen := map[string]bool{}
		for _, filter := range filters {
			if seen[filter.Field] {
				t.Fatalf("duplicate filter %s/%s", name, filter.Field)
			}
			seen[filter.Field] = true
			value := "test"
			switch filter.Kind {
			case "entity":
				value = f.users["ADMIN"].UUID
			case "date":
				value = "2026-10-28"
			case "boolean":
				value = "false"
			}
			params := url.Values{"filter[" + filter.Field + "]": {value}}
			where, args, err := recordWhere(name, params)
			if err != nil {
				t.Fatal(name, filter.Field, err)
			}
			var count int
			if err = f.p.QueryRow(ctx, "SELECT count(*) FROM "+name+" t WHERE "+where, args...).Scan(&count); err != nil {
				t.Fatal(name, filter.Field, err)
			}
		}
	}
	options := f.call(t, "ADMIN", "GET", "/admin/manage/orders/filter-options?field=status", "", 200)
	if len(options["items"].([]any)) == 0 {
		t.Fatal("choice options empty")
	}
	f.call(t, "CUSTOMER", "GET", "/admin/manage/orders/filter-options?field=status", "", 403)
	for _, query := range []string{"filter[password_hash]=x", "filter[restaurant_id]=invalid", "filter[is_available]=yes", "filter[created_from]=wrong", "filter[created_from]=2026-10-29&filter[created_to]=2026-10-28", "filter[is_available]=true&filter[is_available]=false"} {
		f.call(t, "ADMIN", "GET", "/admin/manage/menu-items?"+query, "", 400)
	}
	f.call(t, "ADMIN", "GET", "/admin/manage/orders/filter-options?field=payload_json", "", 400)
}

func TestRecordDateFiltersUseInclusiveIndiaCalendarDays(t *testing.T) {
	params := url.Values{"filter[created_from]": {"2026-10-28"}, "filter[created_to]": {"2026-10-28"}}
	where, args, err := recordWhere("orders", params)
	if err != nil || len(args) != 2 || !strings.Contains(where, "t.created_at>=") || !strings.Contains(where, "t.created_at<") {
		t.Fatal(where, args, err)
	}
	first := args[0].(time.Time)
	last := args[1].(time.Time)
	if first.UTC().Format(time.RFC3339) != "2026-10-27T18:30:00Z" || last.Sub(first) != 24*time.Hour {
		t.Fatal(fmt.Sprint(args))
	}
}

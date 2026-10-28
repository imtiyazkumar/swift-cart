package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAdminEntityLabelsPreserveIDsAndStayAdminOnly(t *testing.T) {
	f := newWorkflow(t)
	ctx := context.Background()
	var variant string
	if err := f.p.QueryRow(ctx, `INSERT INTO item_variants(menu_item_id,name,price_delta,is_available) SELECT id,'Large',50,true FROM menu_items WHERE uuid=$1 RETURNING uuid`, f.item).Scan(&variant); err != nil {
		t.Fatal(err)
	}
	restaurant := f.call(t, "ADMIN", "GET", "/admin/manage/restaurants/"+f.restaurant, "", 200)["record"].(map[string]any)
	if restaurant["owner_user_id"] != f.users["RESTAURANT_OWNER"].UUID {
		t.Fatal("public relationship identifier changed")
	}
	relation := restaurant["_relations"].(map[string]any)["owner_user_id"].(map[string]any)
	if relation["label"] != "RESTAURANT_OWNER" || relation["id"] != f.users["RESTAURANT_OWNER"].UUID || relation["resource"] != "users" {
		t.Fatal("owner name/route incorrect", relation)
	}
	item := f.call(t, "ADMIN", "GET", "/admin/manage/menu-items/"+f.item, "", 200)["record"].(map[string]any)
	refs := item["_relations"].(map[string]any)
	if refs["restaurant_id"].(map[string]any)["label"] != "Restaurant" || refs["category_id"].(map[string]any)["label"] != "Meals" {
		t.Fatal("menu references are not named", refs)
	}
	record := f.call(t, "ADMIN", "GET", "/admin/manage/item-variants/"+variant, "", 200)["record"].(map[string]any)
	if record["_display_name"] != "Meal / Large" {
		t.Fatal("variant is missing menu context", record["_display_name"])
	}
	if record["menu_item_id"] != f.item {
		t.Fatal("variant payload relationship changed")
	}
	var audit string
	if err := f.p.QueryRow(ctx, `SELECT uuid FROM system_activity_logs WHERE entity_type='MENU_ITEMS' AND entity_uuid=$1 ORDER BY id LIMIT 1`, f.item).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	event := f.call(t, "ADMIN", "GET", "/admin/manage/system-activity-logs/"+audit, "", 200)["record"].(map[string]any)
	target := event["_relations"].(map[string]any)["entity_uuid"].(map[string]any)
	if target["label"] != "Meal" || target["resource"] != "menu-items" {
		t.Fatal("audit entity was not resolved", target)
	}
	// Parse every projection, including table types absent from this small fixture.
	for name := range tables {
		rows, err := f.p.Query(ctx, "SELECT "+adminProject(name, "t")+" FROM "+name+" t LIMIT 1")
		if err != nil {
			t.Fatalf("%s projection: %v", name, err)
		}
		rows.Close()
	}
	var public json.RawMessage
	if err := f.p.QueryRow(ctx, "SELECT "+project("restaurants", "t")+" FROM restaurants t LIMIT 1").Scan(&public); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(public), "_relations") || strings.Contains(string(public), "_display_name") {
		t.Fatal("admin metadata leaked into public projection")
	}
	listing := f.call(t, "ADMIN", "GET", "/admin/manage/item-variants?q=Meal", "", 200)
	if listing["total"].(float64) != 1 {
		t.Fatal("entity names are not searchable", listing)
	}
}

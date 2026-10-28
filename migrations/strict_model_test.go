package migrations_test

import (
	"context"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"testing"
)

func TestRestaurantSchemaAndRemovedFeatures(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	for _, name := range []string{"public.merchants", "catalog.modifiers", "catalog.addons", "auth.admin_mfa_steps", "catalog.categories", "catalog.items", "catalog.item_variants"} {
		var exists bool
		if err := p.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists); err != nil || exists {
			t.Fatalf("out-of-scope relation remains %s: %v", name, err)
		}
	}
	for _, name := range []string{"public.restaurants", "public.menu_categories", "public.menu_items", "public.item_variants"} {
		var exists bool
		if err := p.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists); err != nil || !exists {
			t.Fatalf("required relation missing %s: %v", name, err)
		}
	}
	var columns int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='menu_categories' AND column_name IN ('merchant_id','restaurant_id')`).Scan(&columns); err != nil || columns != 0 {
		t.Fatal("categories still belong to restaurants", err)
	}
	if err := p.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE column_name='merchant_id' AND table_schema NOT IN ('pg_catalog','information_schema')`).Scan(&columns); err != nil || columns != 0 {
		t.Fatal("legacy restaurant reference column remains", err)
	}
}

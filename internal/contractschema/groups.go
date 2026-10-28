package contractschema

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
)

// SearchPath keeps existing table names usable while namespaces follow the documents.
const SearchPath = "public,users,stations,restaurants,orders,delivery,payments,registration,earnings,payouts,cash,accounting,activity,promotions"

var Groups = map[string][]string{
	"users":        {"users", "otp_challenges", "refresh_tokens", "user_addresses"},
	"stations":     {"stations"},
	"restaurants":  {"restaurants", "restaurant_documents", "restaurant_bank_accounts", "menu_categories", "menu_items", "item_variants"},
	"orders":       {"carts", "cart_items", "orders", "order_items", "order_status_history"},
	"delivery":     {"riders", "rider_documents", "delivery_agent_bank_details", "delivery_assignments", "rider_locations"},
	"payments":     {"payment_intents", "payments", "payment_webhook_events", "refunds"},
	"registration": {"registration_fees"},
	"earnings":     {"restaurant_earnings", "delivery_agent_earnings"},
	"payouts":      {"restaurant_payouts", "delivery_agent_payouts", "restaurant_payout_allocations", "delivery_agent_payout_allocations"},
	"cash":         {"cash_collections"},
	"accounting":   {"financial_transactions", "reconciliation_runs"},
	"activity":     {"system_activity_logs"},
	"promotions":   {"coupons", "coupon_usages"},
}

func namespace(table string) string {
	for schema, names := range Groups {
		for _, name := range names {
			if name == table {
				return schema
			}
		}
	}
	panic("table missing document namespace: " + table)
}

// groupTables moves existing relations in place, preserving rows, OIDs, keys,
// indexes, owned identity sequences and dependent triggers atomically.
func groupTables(ctx context.Context, tx pgx.Tx) error {
	for schema, names := range Groups {
		if _, err := tx.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+pgx.Identifier{schema}.Sanitize()); err != nil {
			return err
		}
		for _, name := range names {
			var source string
			if err := tx.QueryRow(ctx, `SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relname=$1 AND c.relkind='r' AND n.nspname IN ('public',$2)`, name, schema).Scan(&source); err != nil {
				return fmt.Errorf("locate %s: %w", name, err)
			}
			if source == "public" {
				if _, err := tx.Exec(ctx, "ALTER TABLE "+pgx.Identifier{source, name}.Sanitize()+" SET SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
					return err
				}
			}
		}
	}
	_, err := tx.Exec(ctx, "SET LOCAL search_path TO "+SearchPath)
	return err
}

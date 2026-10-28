package contractschema

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"regexp"
)

//go:embed import_core.sql
var importCoreSQL string

type ImportReport struct{ Users, Orders, OrderItems, LedgerEntries int64 }

// PrepareLegacy builds and verifies a separate replacement schema. It never
// deletes or renames source tables and is not a production cutover.
func PrepareLegacy(ctx context.Context, pool *pgxpool.Pool, schema string) (ImportReport, error) {
	report := ImportReport{}
	if !regexp.MustCompile(`^yember_next_[a-z0-9_]+$`).MatchString(schema) {
		return report, fmt.Errorf("replacement schema must start with yember_next_")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return report, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(729499)`); err != nil {
		return report, err
	}
	// A consistent source snapshot is essential: rows cannot change between the
	// copy and conservation checks.
	rows, err := tx.Query(ctx, `SELECT n.nspname,c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind='r' AND n.nspname IN ('public','auth','users','restaurants','orders','delivery','finance','payments','notifications') ORDER BY n.nspname,c.relname`)
	if err != nil {
		return report, err
	}
	var sources []pgx.Identifier
	for rows.Next() {
		var namespace, name string
		if err = rows.Scan(&namespace, &name); err != nil {
			rows.Close()
			return report, err
		}
		sources = append(sources, pgx.Identifier{namespace, name})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return report, err
	}
	for _, source := range sources {
		if _, err = tx.Exec(ctx, "LOCK TABLE "+source.Sanitize()+" IN SHARE MODE"); err != nil {
			return report, err
		}
	}
	// Never present a partial financial import as a complete data conversion.
	for _, name := range []string{"finance.payouts", "finance.payout_history", "payments.payments", "payments.refunds", "payments.settlements", "payments.onboarding_payments"} {
		var count int64
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM "+name).Scan(&count); err != nil {
			return report, err
		}
		if count > 0 {
			return report, fmt.Errorf("cannot prepare a lossless replacement yet: %s has %d records whose conversion is not implemented", name, count)
		}
	}
	if _, err = tx.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		return report, err
	}
	if _, err = tx.Exec(ctx, "SET LOCAL search_path TO "+pgx.Identifier{schema}.Sanitize()+",public"); err != nil {
		return report, err
	}
	if _, err = tx.Exec(ctx, SQL); err != nil {
		return report, fmt.Errorf("create replacement schema: %w", err)
	}
	if _, err = tx.Exec(ctx, `ALTER TABLE orders DISABLE TRIGGER order_status_history`); err != nil {
		return report, err
	}
	if _, err = tx.Exec(ctx, importCoreSQL); err != nil {
		return report, fmt.Errorf("copy and verify legacy data: %w", err)
	}
	if _, err = tx.Exec(ctx, importFinanceSQL); err != nil {
		return report, fmt.Errorf("copy and verify financial operations: %w", err)
	}
	if _, err = tx.Exec(ctx, `ALTER TABLE orders ENABLE TRIGGER order_status_history`); err != nil {
		return report, err
	}
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM users),(SELECT count(*) FROM orders),(SELECT count(*) FROM order_items),(SELECT count(*) FROM financial_transactions WHERE event_key LIKE 'legacy:%')`).Scan(&report.Users, &report.Orders, &report.OrderItems, &report.LedgerEntries); err != nil {
		return report, err
	}
	if err = tx.Commit(ctx); err != nil {
		return report, fmt.Errorf("replacement verification failed: %w", err)
	}
	return report, nil
}

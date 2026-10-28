package contractschema

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/imtiyazkumar/yember-core/migrations/document"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
type definition struct {
	Tables []struct {
		Name    string `json:"table"`
		Columns []struct {
			Name string `json:"name"`
			Type string `json:"sql_type"`
		} `json:"columns"`
	} `json:"tables"`
}

// Validate rejects extra application tables, wrong namespaces and schema drift.
// The migration history is recorded in activity logs, not an extra business table.
func Validate(ctx context.Context, pool *pgxpool.Pool) error { return validate(ctx, pool) }
func validate(ctx context.Context, q queryer) error          { return validateNamespace(ctx, q, false) }
func validateNamespace(ctx context.Context, q queryer, ungrouped bool) error {
	var contract definition
	if err := json.Unmarshal(Contract, &contract); err != nil {
		return err
	}
	expected := map[string]bool{}
	for _, t := range contract.Tables {
		schema := namespace(t.Name)
		if ungrouped {
			schema = "public"
		}
		expected[schema+"."+t.Name] = true
	}
	rows, err := q.Query(ctx, `SELECT n.nspname||'.'||c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind IN ('r','p','f') AND n.nspname<>'information_schema' AND n.nspname!~'^pg_' ORDER BY 1`)
	if err != nil {
		return err
	}
	actual := map[string]bool{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		actual[name] = true
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	var differences []string
	for name := range actual {
		if !expected[name] {
			differences = append(differences, "unexpected "+name)
		}
	}
	for name := range expected {
		if !actual[name] {
			differences = append(differences, "missing "+name)
		}
	}
	if len(differences) > 0 {
		sort.Strings(differences)
		return fmt.Errorf("document schema mismatch: %s", strings.Join(differences, ", "))
	}
	for _, t := range contract.Tables {
		rows, err = q.Query(ctx, `SELECT a.attname,format_type(a.atttypid,a.atttypmod),a.attidentity::text FROM pg_attribute a WHERE a.attrelid=$1::regclass AND a.attnum>0 AND NOT a.attisdropped`, func() string {
			if ungrouped {
				return "public." + t.Name
			}
			return namespace(t.Name) + "." + t.Name
		}())
		if err != nil {
			return err
		}
		columns := map[string]string{}
		identity := ""
		for rows.Next() {
			var name, typ, id string
			if err = rows.Scan(&name, &typ, &id); err != nil {
				rows.Close()
				return err
			}
			columns[name] = typ
			if name == "id" {
				identity = id
			}
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		if len(columns) != len(t.Columns) || identity != "a" {
			return fmt.Errorf("%s columns or GENERATED ALWAYS bigint identity differ from the documents", t.Name)
		}
		for _, c := range t.Columns {
			want := c.Type
			switch {
			case strings.HasPrefix(want, "varchar"):
				want = strings.Replace(want, "varchar", "character varying", 1)
			case strings.HasPrefix(want, "char("):
				want = strings.Replace(want, "char(", "character(", 1)
			case want == "int":
				want = "integer"
			case want == "timestamptz":
				want = "timestamp with time zone"
			}
			if columns[c.Name] != want {
				return fmt.Errorf("%s.%s: got %s, require %s", t.Name, c.Name, columns[c.Name], want)
			}
		}
		var unique bool
		if err = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint c JOIN pg_attribute a ON a.attrelid=c.conrelid AND a.attnum=c.conkey[1] WHERE c.conrelid=$1::regclass AND c.contype='u' AND cardinality(c.conkey)=1 AND a.attname='uuid')`, func() string {
			if ungrouped {
				return "public." + t.Name
			}
			return namespace(t.Name) + "." + t.Name
		}()).Scan(&unique); err != nil {
			return err
		}
		if !unique {
			return fmt.Errorf("%s.uuid must be unique", t.Name)
		}
	}
	var invalid int
	if err = q.QueryRow(ctx, `SELECT count(*) FROM pg_constraint c JOIN pg_attribute a ON a.attrelid=c.conrelid AND a.attnum=ANY(c.conkey) JOIN pg_namespace n ON n.oid=c.connamespace WHERE c.contype='f' AND n.nspname<>'information_schema' AND n.nspname!~'^pg_' AND a.atttypid<>'bigint'::regtype`).Scan(&invalid); err != nil {
		return err
	}
	if invalid != 0 {
		return fmt.Errorf("%d foreign keys are not bigint", invalid)
	}
	return nil
}

// Install only bootstraps an empty database or validates an existing exact
// installation. It cannot silently replay the old 81-table migration bundle.
func Install(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(729499)`); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind IN ('r','p','f') AND n.nspname<>'information_schema' AND n.nspname!~'^pg_'`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err = tx.Exec(ctx, SQL); err != nil {
			return err
		}
	}
	if err = validate(ctx, tx); err != nil {
		if oldErr := validateNamespace(ctx, tx, true); oldErr == nil {
			err = groupTables(ctx, tx)
		}
		if err != nil {
			return err
		}
	}
	if err = validate(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "SET LOCAL search_path TO "+SearchPath); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, document.BehaviorsSQL); err != nil {
		return err
	}
	var database string
	if err = tx.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{database}.Sanitize()+" SET search_path TO "+SearchPath); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	pool.Reset()
	return nil
}

// ReplaceEmptyLegacy replaces an inspected, backed-up legacy installation only
// when every operational table is empty. Nonempty installations must use the
// verified lossless conversion; they are never reset by this path.
func ReplaceEmptyLegacy(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(729499)`); err != nil {
		return err
	}
	allowedSchemas := map[string]bool{"public": true, "auth": true, "users": true, "restaurants": true, "merchants": true, "catalog": true, "orders": true, "delivery": true, "finance": true, "payments": true, "notifications": true}
	configuration := map[string]bool{"public.schema_migrations": true, "public.platform_settings": true, "finance.policy": true, "finance.refund_policy": true}
	rows, err := tx.Query(ctx, `SELECT n.nspname,c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind IN ('r','p','f') AND n.nspname<>'information_schema' AND n.nspname!~'^pg_' ORDER BY 1,2`)
	if err != nil {
		return err
	}
	var sources []pgx.Identifier
	for rows.Next() {
		var schema, name string
		if err = rows.Scan(&schema, &name); err != nil {
			rows.Close()
			return err
		}
		if !allowedSchemas[schema] {
			rows.Close()
			return fmt.Errorf("unrecognized source schema %s; refusing replacement", schema)
		}
		sources = append(sources, pgx.Identifier{schema, name})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if len(sources) == 0 {
		return fmt.Errorf("no legacy installation found; use ordinary migration")
	}
	for _, source := range sources {
		if _, err = tx.Exec(ctx, "LOCK TABLE "+source.Sanitize()+" IN ACCESS EXCLUSIVE MODE"); err != nil {
			return err
		}
	}
	for _, source := range sources {
		name := strings.Join(source, ".")
		if configuration[name] {
			continue
		}
		var count int64
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM "+source.Sanitize()).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("%s contains %d existing records; lossless conversion is required, replacement was cancelled", name, count)
		}
	}
	if _, err = tx.Exec(ctx, `CREATE SCHEMA yember_install; SET LOCAL search_path TO yember_install,public`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, SQL); err != nil {
		return fmt.Errorf("install document schema: %w", err)
	}
	for _, source := range sources {
		name := strings.Join(source, ".")
		if !configuration[name] {
			continue
		}
		// These four allowlisted tables contain only migration/operational settings.
		// They remain searchable history in the documented activity table.
		if _, err = tx.Exec(ctx, `INSERT INTO system_activity_logs(actor_type,action,entity_type,after_json,occurred_at) SELECT 'SYSTEM','LEGACY_IMPORT',$1,to_jsonb(t),now() FROM `+source.Sanitize()+" t", name); err != nil {
			return err
		}
	}
	// Keep the entire swap transactional: a failed assertion restores all source
	// namespaces, tables, configuration and records automatically.
	if _, err = tx.Exec(ctx, `ALTER SCHEMA public RENAME TO yember_legacy_public; ALTER SCHEMA yember_install RENAME TO public; SET LOCAL search_path TO public`); err != nil {
		return err
	}
	schemas := []string{"auth", "users", "restaurants", "merchants", "catalog", "orders", "delivery", "finance", "payments", "notifications", "yember_legacy_public"}
	for _, schema := range schemas {
		if _, err = tx.Exec(ctx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `GRANT USAGE ON SCHEMA public TO PUBLIC`); err != nil {
		return err
	}
	if err = validate(ctx, tx); err != nil {
		if oldErr := validateNamespace(ctx, tx, true); oldErr == nil {
			err = groupTables(ctx, tx)
		}
		if err != nil {
			return err
		}
	}
	if err = validate(ctx, tx); err != nil {
		return err
	}
	var database string
	if err = tx.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{database}.Sanitize()+" SET search_path TO "+SearchPath); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	pool.Reset()
	return nil
}

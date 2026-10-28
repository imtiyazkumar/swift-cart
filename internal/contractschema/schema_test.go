package contractschema

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"strings"
	"testing"
)

func TestExactDocumentSchema(t *testing.T) {
	p := testdb.Empty(t)
	ctx := context.Background()
	if _, err := p.Exec(ctx, SQL); err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Tables []struct {
			Table   string `json:"table"`
			Columns []struct {
				Name string `json:"name"`
				Type string `json:"sql_type"`
			} `json:"columns"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(Contract, &contract); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'`).Scan(&count); err != nil || count != len(contract.Tables) {
		t.Fatalf("table contract mismatch got %d want %d: %v", count, len(contract.Tables), err)
	}
	for _, table := range contract.Tables {
		t.Run(table.Table, func(t *testing.T) {
			rows, err := p.Query(ctx, `SELECT a.attname,format_type(a.atttypid,a.atttypmod),a.attidentity::text FROM pg_attribute a WHERE a.attrelid=$1::regclass AND a.attnum>0 AND NOT a.attisdropped`, table.Table)
			if err != nil {
				t.Fatal(err)
			}
			actual := map[string]string{}
			identity := ""
			for rows.Next() {
				var name, typ, id string
				if err = rows.Scan(&name, &typ, &id); err != nil {
					t.Fatal(err)
				}
				actual[name] = typ
				if name == "id" {
					identity = id
				}
			}
			rows.Close()
			if err = rows.Err(); err != nil {
				t.Fatal(err)
			}
			if len(actual) != len(table.Columns) {
				t.Fatalf("column count got %d want %d", len(actual), len(table.Columns))
			}
			if identity != "a" {
				t.Fatal("id is not GENERATED ALWAYS AS IDENTITY")
			}
			for _, col := range table.Columns {
				want := col.Type
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
				if actual[col.Name] != want {
					t.Errorf("%s.%s got %q want %q", table.Table, col.Name, actual[col.Name], want)
				}
			}
			var uuidUnique bool
			if err = p.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint c JOIN pg_attribute a ON a.attrelid=c.conrelid AND a.attnum=c.conkey[1] WHERE c.conrelid=$1::regclass AND c.contype='u' AND cardinality(c.conkey)=1 AND a.attname='uuid')`, table.Table).Scan(&uuidUnique); err != nil || !uuidUnique {
				t.Fatal("public UUID lacks unique constraint", err)
			}
		})
	}
	var nonnumeric int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM pg_constraint c JOIN pg_attribute a ON a.attrelid=c.conrelid AND a.attnum=ANY(c.conkey) WHERE c.contype='f' AND a.atttypid<>'bigint'::regtype`).Scan(&nonnumeric); err != nil || nonnumeric != 0 {
		t.Fatal(fmt.Sprintf("%d foreign keys are not numeric", nonnumeric), err)
	}
}

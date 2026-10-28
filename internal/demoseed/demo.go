package demoseed

import (
	"context"
	"crypto/md5" // Stable fixture identifiers only; not used for authentication.
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/imtiyazkumar/yember-core/internal/contractschema"
	"github.com/imtiyazkumar/yember-core/internal/devseed"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

//go:embed demo.sql demo_images.json
var demoFiles embed.FS

type DemoOptions struct {
	Customers, Restaurants, Riders, Orders int
	Password, DocumentDirectory            string
	DryRun                                 bool
	Progress                               func(string)
}

type DemoCount struct {
	Table  string `json:"table"`
	Before int64  `json:"before"`
	After  int64  `json:"after"`
}
type demoImage struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func ValidateDemoOptions(o DemoOptions) error {
	if o.Customers < 10 || o.Customers > 10000 || o.Restaurants < 5 || o.Restaurants > 1000 || o.Riders < 5 || o.Riders > 2000 || o.Orders < 10 || o.Orders > 100000 {
		return fmt.Errorf("demo sizes must be: customers 10-10000, restaurants 5-1000, riders 5-2000, orders 10-100000")
	}
	if len(o.Password) < 12 || len(o.Password) > 72 {
		return fmt.Errorf("LOCAL_SEED_PASSWORD must contain 12-72 bytes")
	}
	return nil
}

func demoID(kind string, index int) string {
	sum := md5.Sum([]byte("yember-rich-demo-v1:" + kind + ":" + strconv.Itoa(index)))
	return uuid.MustParse(fmt.Sprintf("%x", sum)).String()
}

type countReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func DemoCounts(ctx context.Context, db countReader) ([]DemoCount, error) {
	counts := []DemoCount{}
	for schema, tables := range contractschema.Groups {
		for _, table := range tables {
			var n int64
			if err := db.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{schema, table}.Sanitize()).Scan(&n); err != nil {
				return nil, err
			}
			counts = append(counts, DemoCount{Table: schema + "." + table, Before: n, After: n})
		}
	}
	sort.Slice(counts, func(i, j int) bool { return counts[i].Table < counts[j].Table })
	return counts, nil
}

// SeedDemo adds explicitly synthetic, repeatable fixtures. It never contacts a
// payment/SMS provider, deletes existing records, or resets existing passwords.
func SeedDemo(ctx context.Context, pool *pgxpool.Pool, o DemoOptions) ([]DemoCount, error) {
	if err := ValidateDemoOptions(o); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(o.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET LOCAL search_path TO "+contractschema.SearchPath); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(894214)`); err != nil {
		return nil, err
	}
	before, err := DemoCounts(ctx, tx)
	if err != nil {
		return nil, err
	}
	settings := map[string]string{"customers": strconv.Itoa(o.Customers), "restaurants": strconv.Itoa(o.Restaurants), "riders": strconv.Itoa(o.Riders), "orders": strconv.Itoa(o.Orders), "password_hash": string(hash)}
	for key, value := range settings {
		if _, err = tx.Exec(ctx, `SELECT set_config($1,$2,true)`, "yember.demo_"+key, value); err != nil {
			return nil, err
		}
	}
	var actor int64
	if err = tx.QueryRow(ctx, `SELECT id FROM users WHERE role='ADMIN' AND deleted_at IS NULL ORDER BY (uuid=$1) DESC,id LIMIT 1`, devseed.AdminID).Scan(&actor); err != nil {
		return nil, fmt.Errorf("an existing development administrator is required; run seed_local first")
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('yember.actor_id',$1,true),set_config('yember.action','DEMO_SEED',true),set_config('yember.reason','Synthetic development fixtures; no real financial movement',true),set_config('yember.request_id','demo-seed-v1',true),set_config('yember.ip_address','192.0.2.10',true)`, strconv.FormatInt(actor, 10)); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE demo_images (n int PRIMARY KEY, name text, url text) ON COMMIT DROP`); err != nil {
		return nil, err
	}
	raw, _ := demoFiles.ReadFile("demo_images.json")
	var images []demoImage
	if err = json.Unmarshal(raw, &images); err != nil {
		return nil, err
	}
	for i, image := range images {
		if _, err = tx.Exec(ctx, `INSERT INTO demo_images VALUES($1,$2,$3)`, i+1, image.Name, image.URL); err != nil {
			return nil, err
		}
	}
	// Keep the existing deferred balance trigger enabled. A transaction-scoped
	// expression index makes its per-event lookups practical for bulk fixtures.
	index := "demo_balance_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = tx.Exec(ctx, "CREATE INDEX "+pgx.Identifier{index}.Sanitize()+` ON financial_transactions ((regexp_replace(event_key,':[^:]+$','')))`); err != nil {
		return nil, err
	}
	sql, _ := demoFiles.ReadFile("demo.sql")
	for _, section := range strings.Split(string(sql), "-- STEP ") {
		if strings.TrimSpace(section) == "" {
			continue
		}
		heading, body, _ := strings.Cut(section, "\n")
		if o.Progress != nil {
			o.Progress(strings.TrimSpace(heading))
		}
		if _, err = tx.Exec(ctx, body); err != nil {
			return nil, fmt.Errorf("demo phase %s: %w", strings.TrimSpace(heading), err)
		}
	}
	if o.Progress != nil {
		o.Progress("Validating all deferred constraints and ledger balances")
	}
	if _, err = tx.Exec(ctx, `ANALYZE financial_transactions; SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, "DROP INDEX "+pgx.Identifier{"accounting", index}.Sanitize()); err != nil {
		return nil, err
	}
	after, err := DemoCounts(ctx, tx)
	if err != nil {
		return nil, err
	}
	for i := range after {
		after[i].Before = before[i].Before
		if after[i].After == 0 {
			return nil, fmt.Errorf("%s unexpectedly remains empty", after[i].Table)
		}
	}
	if o.DryRun {
		return after, tx.Rollback(ctx)
	}
	if err = writeDemoDocuments(o); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return after, nil
}

func writeDemoDocuments(o DemoOptions) error {
	root := o.DocumentDirectory
	if root == "" {
		root = ".private-documents"
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	for kind, count := range map[string]int{"restaurant-doc": o.Restaurants * 2, "rider-doc": o.Riders * 2} {
		for i := 1; i <= count; i++ {
			filename := filepath.Join(root, demoID(kind, i))
			f, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if os.IsExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			_, err = f.Write(demoPDF())
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
	return nil
}

func demoPDF() []byte {
	stream := "BT /F1 24 Tf 50 740 Td (YEMBER DEVELOPMENT FIXTURE) Tj 0 -40 Td /F1 14 Tf (DEMO ONLY - NOT A REAL CERTIFICATE) Tj 0 -30 Td (Synthetic onboarding document for local testing.) Tj ET"
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream)}
	var out strings.Builder
	out.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, obj := range objects {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return []byte(out.String())
}

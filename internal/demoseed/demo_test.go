package demoseed

import (
	"context"
	"github.com/imtiyazkumar/yember-core/internal/contractschema"
	"github.com/imtiyazkumar/yember-core/internal/devseed"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"os"
	"path/filepath"
	"testing"
)

func TestDemoValidation(t *testing.T) {
	o := DemoOptions{Customers: 10, Restaurants: 5, Riders: 5, Orders: 100, Password: "DemoTestPassword!2026"}
	if err := ValidateDemoOptions(o); err != nil {
		t.Fatal(err)
	}
	o.Orders = 100001
	if ValidateDemoOptions(o) == nil {
		t.Fatal("unbounded fixture volume accepted")
	}
}
func TestDemoAllTablesBalancesPreservationAndIdempotency(t *testing.T) {
	p := testdb.Empty(t)
	ctx := context.Background()
	if err := contractschema.Install(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO users(uuid,email,password_hash,role,status,full_name) VALUES($1,'admin@yember.test','preserve-this-existing-hash','ADMIN','ACTIVE','Existing administrator')`, devseed.AdminID); err != nil {
		t.Fatal(err)
	}
	o := DemoOptions{Customers: 10, Restaurants: 5, Riders: 5, Orders: 100, Password: "DemoTestPassword!2026", DocumentDirectory: t.TempDir(), Progress: func(s string) { t.Log(s) }}
	before, err := DemoCounts(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	o.DryRun = true
	if _, err = SeedDemo(ctx, p, o); err != nil {
		t.Fatal(err)
	}
	after, err := DemoCounts(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	for i := range before {
		if before[i].After != after[i].After {
			t.Fatal("dry run retained rows", before[i].Table)
		}
	}
	o.DryRun = false
	counts, err := SeedDemo(ctx, p, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 38 {
		t.Fatal("not all 38 tables were reported")
	}
	for _, count := range counts {
		if count.After == 0 {
			t.Fatal("empty table", count.Table)
		}
	}
	verification, err := VerifyDemo(ctx, p, o.DocumentDirectory)
	if err != nil || verification.Tables != 38 || verification.Documents != 20 {
		t.Fatal("committed fixtures failed verification", verification, err)
	}
	again, err := SeedDemo(ctx, p, o)
	if err != nil {
		t.Fatal(err)
	}
	for i := range counts {
		if counts[i].After != again[i].After {
			t.Fatal("rerun added duplicate rows", counts[i].Table, counts[i].After, again[i].After)
		}
	}
	var invalid int
	checks := []string{
		`SELECT count(*) FROM (SELECT regexp_replace(event_key,':[^:]+$','') FROM financial_transactions WHERE event_key LIKE 'demo-v1/%' GROUP BY 1 HAVING sum(CASE direction WHEN 'DEBIT' THEN amount_paise ELSE -amount_paise END)<>0) x`,
		`SELECT count(*) FROM orders o WHERE order_number LIKE 'DEMO-V1-%' AND subtotal<>(SELECT sum(quantity*unit_price) FROM order_items WHERE order_id=o.id)`,
		`SELECT count(*) FROM refresh_tokens WHERE device_label LIKE 'Demo device %' AND (revoked_at IS NULL OR expires_at>now())`,
		`SELECT count(*) FROM otp_challenges WHERE phone LIKE '+910000%' AND expires_at>now()`,
	}
	for _, check := range checks {
		if err = p.QueryRow(ctx, check).Scan(&invalid); err != nil || invalid != 0 {
			t.Fatal("fixture consistency failure", invalid, err)
		}
	}
	var preserved bool
	if err = p.QueryRow(ctx, `SELECT password_hash='preserve-this-existing-hash' AND full_name='Existing administrator' FROM users WHERE uuid=$1`, devseed.AdminID).Scan(&preserved); err != nil || !preserved {
		t.Fatal("existing identity was changed", err)
	}
	data, err := os.ReadFile(filepath.Join(o.DocumentDirectory, demoID("restaurant-doc", 1)))
	if err != nil || len(data) < 100 || string(data[:5]) != "%PDF-" {
		t.Fatal("sample document unavailable", err)
	}
}

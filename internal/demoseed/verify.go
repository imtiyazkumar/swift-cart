package demoseed

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"os"
	"path/filepath"
)

type Verification struct {
	Tables         int   `json:"populated_tables"`
	Rows           int64 `json:"total_rows"`
	ImageFields    int   `json:"image_fields"`
	DistinctImages int   `json:"distinct_images"`
	Documents      int   `json:"documents"`
}

// VerifyDemo checks committed data without contacting external services or
// making financial, authentication, or other application mutations.
func VerifyDemo(ctx context.Context, p *pgxpool.Pool, root string) (Verification, error) {
	result := Verification{}
	counts, err := DemoCounts(ctx, p)
	if err != nil {
		return result, err
	}
	for _, count := range counts {
		if count.After == 0 {
			return result, fmt.Errorf("%s is empty", count.Table)
		}
		result.Tables++
		result.Rows += count.After
	}
	checks := map[string]string{
		"balanced ledger events":        `SELECT count(*) FROM (SELECT regexp_replace(event_key,':[^:]+$','') FROM financial_transactions WHERE event_key LIKE 'demo-v1/%' GROUP BY 1 HAVING sum(CASE direction WHEN 'DEBIT' THEN amount_paise ELSE -amount_paise END)<>0) bad`,
		"order line totals":             `SELECT count(*) FROM orders o WHERE order_number LIKE 'DEMO-V1-%' AND subtotal IS DISTINCT FROM (SELECT sum(quantity*unit_price) FROM order_items WHERE order_id=o.id)`,
		"payment totals":                `SELECT count(*) FROM payments p JOIN orders o ON o.id=p.order_id WHERE p.provider='DEMO' AND p.amount_paise<>round(o.total_amount*100)::bigint`,
		"refund limits":                 `SELECT count(*) FROM payments p WHERE p.provider='DEMO' AND p.amount_paise<(SELECT COALESCE(sum(amount_paise),0) FROM refunds r WHERE r.payment_id=p.id AND r.status<>'FAILED')`,
		"restaurant payout allocations": `SELECT count(*) FROM restaurant_payouts p WHERE provider_transfer_id LIKE 'DEMO_TRANSFER_%' AND amount_paise IS DISTINCT FROM (SELECT sum(amount_paise) FROM restaurant_payout_allocations a WHERE a.restaurant_payout_id=p.id)`,
		"agent payout allocations":      `SELECT count(*) FROM delivery_agent_payouts p WHERE provider_transfer_id LIKE 'DEMO_TRANSFER_%' AND amount_paise IS DISTINCT FROM (SELECT sum(amount_paise) FROM delivery_agent_payout_allocations a WHERE a.delivery_agent_payout_id=p.id)`,
		"expired refresh tokens":        `SELECT count(*) FROM refresh_tokens WHERE device_label LIKE 'Demo device %' AND (revoked_at IS NULL OR expires_at>now())`,
		"expired OTPs":                  `SELECT count(*) FROM otp_challenges WHERE phone LIKE '+910000%' AND (consumed_at IS NULL OR expires_at>now())`,
	}
	for name, query := range checks {
		var invalid int
		if err = p.QueryRow(ctx, query).Scan(&invalid); err != nil {
			return result, err
		}
		if invalid != 0 {
			return result, fmt.Errorf("%s: %d invalid fixtures", name, invalid)
		}
	}
	raw, _ := demoFiles.ReadFile("demo_images.json")
	var photos []demoImage
	if err = json.Unmarshal(raw, &photos); err != nil {
		return result, err
	}
	known := map[string]bool{}
	for _, photo := range photos {
		known[photo.URL] = true
	}
	rows, err := p.Query(ctx, `SELECT image_url FROM restaurants WHERE name LIKE 'Demo %' UNION ALL SELECT image_url FROM menu_categories WHERE name LIKE 'Demo %' UNION ALL SELECT m.image_url FROM menu_items m JOIN restaurants r ON r.id=m.restaurant_id WHERE r.name LIKE 'Demo %'`)
	if err != nil {
		return result, err
	}
	seen := map[string]bool{}
	for rows.Next() {
		var url *string
		if err = rows.Scan(&url); err != nil {
			rows.Close()
			return result, err
		}
		if url == nil || !known[*url] {
			rows.Close()
			return result, fmt.Errorf("an image field does not use a verified manifest URL")
		}
		seen[*url] = true
		result.ImageFields++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	result.DistinctImages = len(seen)
	if root == "" {
		root = ".private-documents"
	}
	rows, err = p.Query(ctx, `SELECT uuid::text FROM restaurant_documents WHERE document_type LIKE 'DEMO_%' UNION ALL SELECT uuid::text FROM rider_documents WHERE document_type LIKE 'DEMO_%'`)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return result, err
		}
		f, e := os.Open(filepath.Join(root, id))
		if e != nil {
			return result, fmt.Errorf("sample document %s unavailable", id)
		}
		header := make([]byte, 5)
		_, e = io.ReadFull(f, header)
		f.Close()
		if e != nil || string(header) != "%PDF-" {
			return result, fmt.Errorf("sample document %s is not a PDF", id)
		}
		result.Documents++
	}
	return result, rows.Err()
}

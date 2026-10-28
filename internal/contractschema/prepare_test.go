package contractschema

import (
	"context"
	"encoding/json"
	"github.com/imtiyazkumar/yember-core/internal/devseed"
	"github.com/imtiyazkumar/yember-core/internal/finance"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"testing"
)

func TestPrepareRetainsSourceAndConservesHistory(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if err := devseed.Seed(ctx, p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE finance.policy SET delivery_charge=0,rider_base=0`); err != nil {
		t.Fatal(err)
	}
	var order string
	if err := p.QueryRow(ctx, `INSERT INTO public.orders(customer_id,restaurant_id,status,total_price,delivery_address) VALUES($1,$2,'DELIVERED',1000,'{"address_line1":"Original delivery destination","city":"Srinagar","latitude":34,"longitude":74}') RETURNING id`, devseed.CustomerID, devseed.RestaurantID).Scan(&order); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO public.order_items(order_id,item_id,name_snapshot,quantity,unit_price,total_price) VALUES($1,'10000000-0000-4000-a000-000000000041','Original menu label',1,100000,100000)`, order); err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = finance.Snapshot(ctx, tx, order); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var attempt string
	if err = p.QueryRow(ctx, `INSERT INTO finance.payment_attempts(order_id,provider,amount,currency,status,provider_id) VALUES($1,'RAZORPAY',100000,'INR','PENDING','plink_preserve') RETURNING id`, order).Scan(&attempt); err != nil {
		t.Fatal(err)
	}
	event := map[string]any{"event": "payment_link.paid", "payload": map[string]any{"payment_link": map[string]any{"entity": map[string]any{"id": "plink_preserve", "reference_id": attempt, "status": "paid", "amount": 100000, "amount_paid": 100000, "currency": "INR", "order_id": "provider_order"}}, "payment": map[string]any{"entity": map[string]any{"id": "pay_preserve", "status": "captured", "amount": 100000, "currency": "INR", "order_id": "provider_order"}}}}
	raw, _ := json.Marshal(event)
	if err = finance.New(p).ApplyRazorpay(ctx, "preserve-event", raw); err != nil {
		t.Fatal(err)
	}
	var refund string
	if err = p.QueryRow(ctx, `INSERT INTO finance.refund_requests(order_id,request_key,amount,reason,requested_by,provider_id,status) VALUES($1,'preserve-refund',5000,'Original refund reason',$2,'rfnd_preserve','PENDING') RETURNING id`, order, devseed.OwnerID).Scan(&refund); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, `INSERT INTO finance.refund_jobs(order_id,status,last_error) VALUES($1,'REVIEW','Provider response requires reconciliation')`, order); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, `INSERT INTO finance.reconciliations(actor,reference,details) VALUES($1,'preservation-reconciliation','{"original_total":100000}')`, devseed.OwnerID); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, `INSERT INTO finance.discrepancies(order_id,event_key,kind,details) VALUES($1,'preservation-discrepancy','REFUND_REQUIRES_ALLOCATION','{"amount":5000}')`, order); err != nil {
		t.Fatal(err)
	}
	var sourceOrders, sourceEntries int64
	p.QueryRow(ctx, `SELECT count(*) FROM public.orders`).Scan(&sourceOrders)
	p.QueryRow(ctx, `SELECT count(*) FROM finance.entries`).Scan(&sourceEntries)
	report, err := PrepareLegacy(ctx, p, "yember_next_preservation_test")
	if err != nil {
		t.Fatal(err)
	}
	if report.Orders != sourceOrders || report.LedgerEntries != sourceEntries || report.Users < 4 {
		t.Fatalf("data conservation report incorrect: %+v", report)
	}
	var sourceStillPresent bool
	if err = p.QueryRow(ctx, `SELECT to_regclass('public.customers') IS NOT NULL AND to_regclass('finance.entries') IS NOT NULL`).Scan(&sourceStillPresent); err != nil || !sourceStillPresent {
		t.Fatal("preparation changed the live schema", err)
	}
	var unit, deliveryAddress string
	var originalLabel string
	if err = p.QueryRow(ctx, `SELECT m.name FROM yember_next_preservation_test.order_items i JOIN yember_next_preservation_test.orders o ON o.id=i.order_id JOIN yember_next_preservation_test.menu_items m ON m.id=i.menu_item_id WHERE o.uuid=$1`, order).Scan(&originalLabel); err != nil || originalLabel != "Original menu label" {
		t.Fatal("normalizing order lines changed the historical receipt label", originalLabel, err)
	}
	var userID int64
	if err = p.QueryRow(ctx, `SELECT i.unit_price::text,a.address_line1,o.user_id FROM yember_next_preservation_test.orders o JOIN yember_next_preservation_test.order_items i ON i.order_id=o.id JOIN yember_next_preservation_test.user_addresses a ON a.id=o.address_id WHERE o.uuid=$1`, order).Scan(&unit, &deliveryAddress, &userID); err != nil || unit != "1000.00" || deliveryAddress != "Original delivery destination" || userID < 1 {
		t.Fatal("historical pricing, address or numeric ownership changed", err)
	}
	var liability int64
	if err = p.QueryRow(ctx, `SELECT sum(CASE t.direction WHEN 'DEBIT' THEN t.amount_paise ELSE -t.amount_paise END)::bigint FROM yember_next_preservation_test.financial_transactions t JOIN yember_next_preservation_test.restaurant_earnings e ON e.id=t.restaurant_earning_id JOIN yember_next_preservation_test.restaurants r ON r.id=e.restaurant_id JOIN yember_next_preservation_test.users u ON u.id=r.owner_user_id WHERE u.uuid=$1 AND t.account_code='RESTAURANT_PAYABLE'`, devseed.OwnerID).Scan(&liability); err != nil || liability != -85000 {
		t.Fatal("restaurant payable changed during conversion", liability, err)
	}
	var retained bool
	if err = p.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM yember_next_preservation_test.refunds WHERE uuid=$1 AND amount_paise=5000 AND provider_refund_id='rfnd_preserve' AND status='PENDING' AND completed_at IS NULL) AND (SELECT count(*) FROM yember_next_preservation_test.refunds)=1 AND (SELECT count(*) FROM yember_next_preservation_test.reconciliation_runs)=2 AND EXISTS(SELECT 1 FROM yember_next_preservation_test.payment_webhook_events WHERE provider_event_id='preserve-event' AND signature_valid IS NULL AND processing_status='IMPORTED' AND payload_json->>'original_payload_available'='false')`, refund).Scan(&retained); err != nil || !retained {
		t.Fatal("refund, reconciliation or original webhook evidence changed", retained, err)
	}
}

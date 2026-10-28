package contractschema

import (
	"context"
	"testing"

	"github.com/imtiyazkumar/yember-core/internal/devseed"
	"github.com/imtiyazkumar/yember-core/internal/finance"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
)

func TestPreparePreservesGrossCashAndSeparateFeeReceivable(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if err := devseed.Seed(ctx, p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE finance.policy SET rider_base=0,delivery_charge=0; UPDATE public.restaurants SET commission_bps=1000`); err != nil {
		t.Fatal(err)
	}
	var order string
	if err := p.QueryRow(ctx, `INSERT INTO public.orders(customer_id,restaurant_id,status,total_price,delivery_address) VALUES($1,$2,'CREATED',1000,'{"latitude":34,"longitude":74}') RETURNING id`, devseed.CustomerID, devseed.RestaurantID).Scan(&order); err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = finance.Snapshot(ctx, tx, order); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO finance.payment_attempts(order_id,provider,amount,currency,status) VALUES($1,'COD',100000,'INR','PENDING')`, order); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO finance.cod_custody(order_id,partner_id,amount) VALUES($1,$2,100000)`, order, devseed.PartnerID); err != nil {
		t.Fatal(err)
	}
	if err = finance.RecordCashCollection(ctx, tx, order); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE public.orders SET status='DELIVERED' WHERE id=$1`, order); err != nil {
		t.Fatal(err)
	}
	if err = finance.Recognize(ctx, tx, order); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	svc := finance.New(p)
	if err = svc.SubmitCashHandover(ctx, devseed.DeliveryUserID, order, 100000); err != nil {
		t.Fatal(err)
	}
	if err = svc.ConfirmCashHandover(ctx, devseed.OwnerID, order, 100000, true); err != nil {
		t.Fatal(err)
	}
	if _, err = PrepareLegacy(ctx, p, "yember_next_cash_test"); err != nil {
		t.Fatal(err)
	}
	var retained bool
	if err = p.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM yember_next_cash_test.cash_collections c
 JOIN yember_next_cash_test.orders o ON o.id=c.order_id
 JOIN yember_next_cash_test.restaurant_payouts p ON p.id=c.restaurant_payout_id
 JOIN yember_next_cash_test.restaurant_payout_allocations a ON a.restaurant_payout_id=p.id
 WHERE o.uuid=$1 AND c.amount_due_paise=100000 AND c.amount_collected_paise=100000 AND c.status='HANDED_OVER'
 AND p.amount_paise=100000 AND p.status='PAID' AND p.paid_at IS NOT NULL AND a.amount_paise=90000
 )`, order).Scan(&retained); err != nil || !retained {
		t.Fatal("gross cash or net allocation changed", retained, err)
	}
	for account, want := range map[string]int64{"CASH_IN_TRANSIT": 0, "RESTAURANT_FEE_RECEIVABLE": 10000, "RESTAURANT_PAYABLE": 0, "YEMBER_BANK": 0} {
		var got int64
		if err = p.QueryRow(ctx, `SELECT COALESCE(sum(CASE direction WHEN 'DEBIT' THEN amount_paise ELSE -amount_paise END),0)::bigint FROM yember_next_cash_test.financial_transactions WHERE account_code=$1`, account).Scan(&got); err != nil || got != want {
			t.Fatalf("%s balance %d want %d: %v", account, got, want, err)
		}
	}
	var linked int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM yember_next_cash_test.financial_transactions WHERE transaction_type='PAYOUT' AND restaurant_payout_id IS NOT NULL`).Scan(&linked); err != nil || linked != 3 {
		t.Fatal("cash postings lost their payout reference", linked, err)
	}
}

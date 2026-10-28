package finance

import (
	"context"
	"github.com/imtiyazkumar/yember-core/internal/devseed"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"testing"
)

func TestGrossCashHandoverLedgerAndReplay(t *testing.T) {
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
	if err = Snapshot(ctx, tx, order); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO finance.payment_attempts(order_id,provider,amount,currency,status) VALUES($1,'COD',100000,'INR','PENDING');`, order); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO finance.cod_custody(order_id,partner_id,amount) VALUES($1,$2,100000)`, order, devseed.PartnerID); err != nil {
		t.Fatal(err)
	}
	if err = RecordCashCollection(ctx, tx, order); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE public.orders SET status='DELIVERED' WHERE id=$1`, order); err != nil {
		t.Fatal(err)
	}
	if err = Recognize(ctx, tx, order); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	s := New(p)
	if err = s.SubmitCashHandover(ctx, devseed.OwnerID, order, 100000); err == nil {
		t.Fatal("restaurant submitted agent handover")
	}
	for i := 0; i < 2; i++ {
		if err = s.SubmitCashHandover(ctx, devseed.DeliveryUserID, order, 100000); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.ConfirmCashHandover(ctx, devseed.DeliveryUserID, order, 100000, true); err == nil {
		t.Fatal("agent acknowledged restaurant cash")
	}
	if err = s.ConfirmCashHandover(ctx, devseed.OwnerID, order, 99999, true); err == nil {
		t.Fatal("mismatched acknowledgement accepted")
	}
	for i := 0; i < 2; i++ {
		if err = s.ConfirmCashHandover(ctx, devseed.OwnerID, order, 100000, true); err != nil {
			t.Fatal(err)
		}
	}
	for account, want := range map[string]int64{"CASH": 0, "CASH_IN_TRANSIT": 0, "RESTAURANT_FEE_RECEIVABLE": 10000, "PARTNER_PAYABLE": 0} {
		var got int64
		if err = p.QueryRow(ctx, `SELECT COALESCE(sum(e.amount),0)::bigint FROM finance.entries e JOIN finance.journals j ON j.id=e.journal_id WHERE j.order_id=$1 AND e.account=$2`, order, account).Scan(&got); err != nil || got != want {
			t.Fatalf("%s got %d want %d: %v", account, got, want, err)
		}
	}
	var count int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM finance.journals WHERE order_id=$1 AND kind='CASH_HANDOVER'`, order).Scan(&count); err != nil || count != 1 {
		t.Fatal("handover duplicated", err)
	}
}

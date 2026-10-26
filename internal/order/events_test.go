package order

import (
	"context"
	"github.com/imtiyazkumar/yember-core/internal/devseed"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"testing"
)

func TestOrderEventsCaptureAllTransitions(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if err := devseed.Seed(ctx, p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := p.QueryRow(ctx, `INSERT INTO public.orders(merchant_id,customer_id,status,total_price,delivery_address) VALUES($1,$2,'CREATED',100,'{"latitude":34,"longitude":74}') RETURNING id`, devseed.RestaurantID, devseed.CustomerID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"ACCEPTED", "PREPARING", "READY_FOR_PICKUP", "PICKED_UP", "OUT_FOR_DELIVERY", "DELIVERED"} {
		tx, err := p.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `UPDATE public.orders SET status=$2::order_status,updated_at=now() WHERE id=$1`, id, status); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO orders.order_status_history(order_id,status,changed_by,notes) VALUES($1,$2::order_status,$3,'Recorded transition')`, id, status, devseed.AdminID); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var count, actors int
	if err := p.QueryRow(ctx, `SELECT count(*),count(changed_by) FROM orders.order_events WHERE order_id=$1`, id).Scan(&count, &actors); err != nil || count != 7 || actors != 6 {
		t.Fatalf("events=%d actors=%d err=%v", count, actors, err)
	}
	if _, err := p.Exec(ctx, `UPDATE public.orders SET status=status WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `SELECT count(*) FROM orders.order_events WHERE order_id=$1`, id).Scan(&count); err != nil || count != 7 {
		t.Fatalf("unchanged update generated event: %d %v", count, err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE public.orders SET status='REFUNDED' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err = p.QueryRow(ctx, `SELECT count(*) FROM orders.order_events WHERE order_id=$1`, id).Scan(&count); err != nil || count != 7 {
		t.Fatal("rolled back transition leaked an event")
	}
}

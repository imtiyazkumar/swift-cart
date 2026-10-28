package devseed

import (
	"context"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
	"testing"
)

func TestStationRelationsAndHistory(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if err := Seed(ctx, p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := SeedStations(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM public.stations`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("stations %d: %v", count, err)
	}
	for _, table := range []string{"delivery.offers", "public.delivery_assignments", "delivery.rider_earnings"} {
		var wrong int
		if err := p.QueryRow(ctx, `SELECT count(*) FROM `+table+` r JOIN public.orders o ON o.id=r.order_id WHERE r.station_id IS DISTINCT FROM o.station_id OR r.station_id IS NULL`).Scan(&wrong); err != nil || wrong != 0 {
			t.Fatalf("%s mismatched: %d %v", table, wrong, err)
		}
	}
	if _, err := p.Exec(ctx, `UPDATE public.orders SET station_id='10000000-0000-4000-a000-000000000081' WHERE id='10000000-0000-4000-a000-000000000100'`); err == nil {
		t.Fatal("historical order station could be reassigned")
	}
	if _, err := p.Exec(ctx, `UPDATE delivery.rider_earnings SET station_id='10000000-0000-4000-a000-000000000081' WHERE order_id='10000000-0000-4000-a000-000000000100'`); err == nil {
		t.Fatal("earnings station could diverge from order")
	}
	if _, err := p.Exec(ctx, `INSERT INTO public.partner_support_tickets(user_id,subject,category,request_key,order_id) VALUES($1,'Station relation test','ORDER','station-test','10000000-0000-4000-a000-000000000100')`, DeliveryUserID); err != nil {
		t.Fatal(err)
	}
	var matches bool
	if err := p.QueryRow(ctx, `SELECT station_id='10000000-0000-4000-a000-000000000080' FROM public.partner_support_tickets LIMIT 1`).Scan(&matches); err != nil || !matches {
		t.Fatalf("support station missing: %v", err)
	}
}

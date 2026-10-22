package dispatch

import (
    "context"
    "github.com/imtiyazkumar/yember-core/internal/testdb"
    "sync"
    "sync/atomic"
    "testing"
)

func TestRoundRobinExpiryAndConcurrentAcceptance(t *testing.T) {
    pool := testdb.New(t)
    testdb.Station(t, pool)
    ctx := context.Background()
    var customer, merchant, order string
    if err := pool.QueryRow(ctx, `INSERT INTO public.users(email,password_hash,role) VALUES('customer@test.invalid','hash','CUSTOMER') RETURNING id`).Scan(&customer); err != nil {
        t.Fatal(err)
    }
    if _, err := pool.Exec(ctx, `INSERT INTO public.customers(id,name,email) VALUES($1,'Customer','customer@test.invalid')`, customer); err != nil {
        t.Fatal(err)
    }
    if err := pool.QueryRow(ctx, `INSERT INTO public.merchants(name,email,is_active,latitude,longitude) VALUES('Kitchen','k@test.invalid',true,34,74) RETURNING id`).Scan(&merchant); err != nil {
        t.Fatal(err)
    }
    if err := pool.QueryRow(ctx, `INSERT INTO public.orders(merchant_id,customer_id,total_price,status,delivery_address) VALUES($1,$2,100,'ACCEPTED','{"latitude":34,"longitude":74}') RETURNING id`, merchant, customer).Scan(&order); err != nil {
        t.Fatal(err)
    }
    partners := []string{}
    for _, email := range []string{"first@test.invalid", "second@test.invalid"} {
        var user, partner string
        if err := pool.QueryRow(ctx, `INSERT INTO public.users(email,password_hash,role) VALUES($1,'hash','DELIVERY_PARTNER') RETURNING id`, email).Scan(&user); err != nil {
            t.Fatal(err)
        }
        if err := pool.QueryRow(ctx, `INSERT INTO delivery.delivery_partners(user_id,name,phone,status,approval_status,station_id) VALUES($1,'Partner','test','OFFLINE','APPROVED','20000000-0000-4000-a000-000000000001') RETURNING id`, user).Scan(&partner); err != nil {
            t.Fatal(err)
        }
        if _, err := pool.Exec(ctx, `INSERT INTO delivery.vehicles(partner_id,vehicle_type,registration_number) VALUES($1,'BIKE','TEST')`, partner); err != nil {
            t.Fatal(err)
        }
        testdb.PartnerLocation(t, pool, partner)
        partners = append(partners, partner)
    }
    if err := Tick(ctx, pool); err != nil {
        t.Fatal(err)
    }
    var first string
    if err := pool.QueryRow(ctx, `SELECT partner_id FROM delivery.offers WHERE order_id=$1 AND status='PENDING'`, order).Scan(&first); err != nil {
        t.Fatal(err)
    }
    if _, err := pool.Exec(ctx, `UPDATE delivery.offers SET expires_at=now()-interval '1 second' WHERE order_id=$1`, order); err != nil {
        t.Fatal(err)
    }
    if err := Accept(ctx, pool, first, order); err == nil {
        t.Fatal("expired offer accepted")
    }
    if _, err := pool.Exec(ctx, `INSERT INTO public.platform_settings(key,value) VALUES('dispatch_policy','{"max_attempts":1}')`); err != nil {
        t.Fatal(err)
    }
    if err := Tick(ctx, pool); err != nil {
        t.Fatal(err)
    }
    var capped int
    if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery.offers WHERE order_id=$1 AND status='PENDING'`, order).Scan(&capped); err != nil || capped != 0 {
        t.Fatal("dispatch retry cap was ignored")
    }
    if _, err := pool.Exec(ctx, `UPDATE public.platform_settings SET value='{"max_attempts":2}' WHERE key='dispatch_policy'`); err != nil {
        t.Fatal(err)
    }
    if err := Tick(ctx, pool); err != nil {
        t.Fatal(err)
    }
    var next string
    if err := pool.QueryRow(ctx, `SELECT partner_id FROM delivery.offers WHERE order_id=$1 AND status='PENDING'`, order).Scan(&next); err != nil {
        t.Fatal(err)
    }
    if next == first {
        t.Fatal("scheduler did not rotate to the next eligible partner")
    }
    var success atomic.Int32
    var wg sync.WaitGroup
    for i := 0; i < 8; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            if Accept(ctx, pool, next, order) == nil {
                success.Add(1)
            }
        }()
    }
    wg.Wait()
    if success.Load() != 1 {
        t.Fatalf("expected one successful acceptance, got %d", success.Load())
    }
    if err := Tick(ctx, pool); err != nil {
        t.Fatal(err)
    }
    var count int
    if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery.delivery_assignments WHERE order_id=$1`, order).Scan(&count); err != nil || count != 1 {
        t.Fatal("duplicate delivery assignment")
    }
    _ = partners
}

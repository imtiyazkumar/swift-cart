package admin

import (
    "context"
    "github.com/imtiyazkumar/yember-core/internal/testdb"
    "go.uber.org/zap"
    "testing"
)

func TestDecisionValidation(t *testing.T) {
    for _, d := range []Decision{{Status: "ACTIVE"}, {Status: "REJECTED"}, {Status: "REJECTED", Reason: "   "}} {
        if d.Validate() == nil {
            t.Fatalf("accepted invalid decision: %+v", d)
        }
    }
    if err := (Decision{Status: "REJECTED", Reason: "Missing license"}).Validate(); err != nil {
        t.Fatal(err)
    }
}

func TestApprovalIsAtomicAndDoesNotActivate(t *testing.T) {
    pool := testdb.New(t)
    testdb.Station(t, pool)
    ctx := context.Background()
    var actor, owner, merchant string
    if err := pool.QueryRow(ctx, `INSERT INTO public.users(email,password_hash,role) VALUES('admin@test.invalid','hash','ADMIN') RETURNING id`).Scan(&actor); err != nil {
        t.Fatal(err)
    }
    if err := pool.QueryRow(ctx, `INSERT INTO public.users(email,password_hash,role) VALUES('owner@test.invalid','hash','MERCHANT_OWNER') RETURNING id`).Scan(&owner); err != nil {
        t.Fatal(err)
    }
    if err := pool.QueryRow(ctx, `INSERT INTO public.merchants(name,email,owner_id,is_active,latitude,longitude) VALUES('Test Kitchen','kitchen@test.invalid',$1,false,34,74) RETURNING id`, owner).Scan(&merchant); err != nil {
        t.Fatal(err)
    }
    svc := NewService(NewPostgresRepo(pool, zap.NewNop()))
    if err := svc.Decide(ctx, owner, "restaurant", merchant, Decision{Status: "APPROVED"}); err == nil {
        t.Fatal("non-admin application decision accepted")
    }
    if err := svc.Decide(ctx, actor, "restaurant", merchant, Decision{Status: "APPROVED"}); err != nil {
        t.Fatal(err)
    }
    var active bool
    if err := pool.QueryRow(ctx, `SELECT is_active FROM public.merchants WHERE id=$1`, merchant).Scan(&active); err != nil || active {
        t.Fatal("approval activated an unpaid restaurant")
    }
    if err := svc.Decide(ctx, actor, "restaurant", merchant, Decision{Status: "REJECTED", Reason: "late decision"}); err == nil {
        t.Fatal("duplicate review accepted")
    }
    var auditCount, emailCount int
    _ = pool.QueryRow(ctx, `SELECT count(*) FROM public.admin_audit_logs WHERE admin_id=$1`, actor).Scan(&auditCount)
    _ = pool.QueryRow(ctx, `SELECT count(*) FROM notifications.email_outbox`).Scan(&emailCount)
    if auditCount != 1 || emailCount < 1 {
        t.Fatal("decision did not record audit and email")
    }
}

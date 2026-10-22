package partnerops

import (
    "context"
    "math"
    "testing"

    "github.com/imtiyazkumar/yember-core/internal/devseed"
    "github.com/imtiyazkumar/yember-core/internal/rider"
    "github.com/imtiyazkumar/yember-core/internal/testdb"
    "go.uber.org/zap"
)

func TestCourierEligibilityOfflineOffersAndCoordinates(t *testing.T) {
    pool := testdb.New(t)
    ctx := context.Background()
    if err := devseed.Seed(ctx, pool, "LocalTestPassword!2026"); err != nil {
        t.Fatal(err)
    }
    repo := rider.NewPostgresRepo(pool, zap.NewNop())
    if _, err := pool.Exec(ctx, `UPDATE delivery.delivery_partners SET eligibility_expires_at=now()-interval '1 hour' WHERE id=$1`, devseed.PartnerID); err != nil {
        t.Fatal(err)
    }
    if err := repo.UpdateStatus(ctx, devseed.PartnerID, "ONLINE"); err == nil {
        t.Fatal("expired eligibility allowed online")
    }
    if _, err := pool.Exec(ctx, `UPDATE delivery.delivery_partners SET eligibility_expires_at=NULL WHERE id=$1`, devseed.PartnerID); err != nil {
        t.Fatal(err)
    }
    if err := repo.UpdateStatus(ctx, devseed.PartnerID, "ONLINE"); err != nil {
        t.Fatal(err)
    }
    if _, err := pool.Exec(ctx, `INSERT INTO delivery.offers(order_id,partner_id,expires_at) VALUES('10000000-0000-4000-a000-000000000107',$1,now()+interval '5 minutes')`, devseed.PartnerID); err != nil {
        t.Fatal(err)
    }
    if err := repo.UpdateStatus(ctx, devseed.PartnerID, "OFFLINE"); err != nil {
        t.Fatal(err)
    }
    var pending, active int
    if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery.offers WHERE partner_id=$1 AND status='PENDING'`, devseed.PartnerID).Scan(&pending); err != nil || pending != 0 {
        t.Fatal("offline left a pending offer")
    }
    if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery.delivery_assignments WHERE partner_id=$1 AND status='ASSIGNED'`, devseed.PartnerID).Scan(&active); err != nil || active != 1 {
        t.Fatal("offline disturbed an accepted delivery")
    }
    if err := repo.RecordLocation(ctx, devseed.PartnerID, 91, 0); err == nil {
        t.Fatal("invalid latitude accepted")
    }
    if err := repo.RecordLocation(ctx, devseed.PartnerID, 0, math.NaN()); err == nil {
        t.Fatal("invalid longitude accepted")
    }
}

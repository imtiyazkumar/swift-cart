package devseed

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

// SeedStations repairs only the documented local fixtures, never arbitrary history.
func SeedStations(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(894214),pg_advisory_xact_lock(894215)`); err != nil {
		return err
	}
	for _, statement := range fixtures {
		if strings.Contains(statement, "INSERT INTO public.stations(") {
			if _, err = tx.Exec(ctx, statement); err != nil {
				return err
			}
		}
	}
	for _, profile := range []struct{ kind, table, id, user string }{{"restaurant", "public.merchants", RestaurantID, OwnerID}, {"delivery", "delivery.delivery_partners", PartnerID, DeliveryUserID}} {
		var missing bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+profile.table+` WHERE id=$1 AND station_id IS NULL)`, profile.id).Scan(&missing); err != nil {
			return err
		}
		if !missing {
			continue
		}
		var request string
		err = tx.QueryRow(ctx, `INSERT INTO public.station_change_requests(kind,resource_id,requested_by,station_id,latitude,longitude,reason,reviewed_by) VALUES($1,$2,$3,'10000000-0000-4000-a000-000000000080',34,74,'Restore documented local development fixture station',$4) RETURNING id`, profile.kind, profile.id, profile.user, AdminID).Scan(&request)
		if err != nil {
			return err
		}
		extra := ""
		if profile.kind == "restaurant" {
			extra = ",latitude=34,longitude=74,is_active=false"
		} else {
			extra = ",status='OFFLINE'"
		}
		if _, err = tx.Exec(ctx, `UPDATE `+profile.table+` SET station_id='10000000-0000-4000-a000-000000000080'`+extra+` WHERE id=$1`, profile.id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE public.station_change_requests SET status='APPROVED',reviewed_at=now(),review_reason='Known local seed fixture repair' WHERE id=$1`, request); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO public.admin_audit_logs(admin_id,action,target_type,target_id,details) VALUES($1,'LOCAL_STATION_REPAIRED',$2,$3,'{"reason":"Restore documented local development fixture"}')`, AdminID, profile.kind, profile.id); err != nil {
			return err
		}
	}
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("10000000-0000-4000-a000-%012d", 100+i)
		if _, err = tx.Exec(ctx, `UPDATE public.orders SET station_id='10000000-0000-4000-a000-000000000080',delivery_address=delivery_address||'{"latitude":34,"longitude":74}'::jsonb WHERE id=$1 AND merchant_id=$2 AND customer_id=$3 AND station_id IS NULL AND delivery_address->>'address_line1'='12 Local Test Street'`, id, RestaurantID, CustomerID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

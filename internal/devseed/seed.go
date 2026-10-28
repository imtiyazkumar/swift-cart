// Package devseed installs repeatable local development fixtures.
package devseed

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const AdminID = "10000000-0000-4000-a000-000000000001"
const CustomerID = "10000000-0000-4000-a000-000000000002"
const OwnerID = "10000000-0000-4000-a000-000000000003"
const DeliveryUserID = "10000000-0000-4000-a000-000000000004"
const RestaurantID = "10000000-0000-4000-a000-000000000010"
const PartnerID = "10000000-0000-4000-a000-000000000020"

func ValidateTarget(environment, dsn string) error {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return errors.New("invalid local database configuration")
	}
	local := strings.EqualFold(cfg.ConnConfig.Host, "localhost")
	if address := net.ParseIP(cfg.ConnConfig.Host); address != nil {
		local = address.IsLoopback()
	}
	if !local || (environment != "development" && environment != "test") ||
		(cfg.ConnConfig.Database != "yember_local" && cfg.ConnConfig.Database != "yember_local_test") {
		return errors.New("local seeding requires APP_ENV=development/test and a loopback yember_local or yember_local_test database")
	}
	return nil
}

func Seed(ctx context.Context, pool *pgxpool.Pool, password string) error {
	if len(password) < 12 || len(password) > 72 {
		return errors.New("LOCAL_SEED_PASSWORD must contain 12-72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(894214)`); err != nil {
		return err
	}
	for _, user := range []struct{ id, email, role, name, phone string }{
		{AdminID, "admin@yember.test", "ADMIN", "Local Administrator", "+919000000001"},
		{CustomerID, "customer@yember.test", "CUSTOMER", "Local Customer", "+919000000002"},
		{OwnerID, "restaurant@yember.test", "RESTAURANT_OWNER", "Local Restaurant Owner", "+919000000003"},
		{DeliveryUserID, "delivery@yember.test", "DELIVERY_PARTNER", "Local Delivery Agent", "+919000000004"},
	} {
		_, err = tx.Exec(ctx, `INSERT INTO public.users(id,email,password_hash,role,full_name,phone,email_verified_at) VALUES($1,$2,$3,$4,$5,$6,now()) ON CONFLICT(id) DO NOTHING`, user.id, user.email, string(hash), user.role, user.name, user.phone)
		if err != nil {
			return fmt.Errorf("could not create %s local identity; existing identities are never overwritten", user.role)
		}
		var email, role string
		if err = tx.QueryRow(ctx, `SELECT email,role FROM public.users WHERE id=$1`, user.id).Scan(&email, &role); err != nil {
			return err
		}
		if email != user.email || role != user.role {
			return errors.New("local seed identity collision; no records changed")
		}
	}
	for i, statement := range fixtures {
		if _, err = tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("local fixture %d: %w", i+1, err)
		}
	}
	var allOrdersExist bool
	if err = tx.QueryRow(ctx, `SELECT count(*)=10 FROM public.orders WHERE id::text IN (SELECT '10000000-0000-4000-a000-'||lpad(n::text,12,'0') FROM generate_series(100,109) n)`).Scan(&allOrdersExist); err != nil {
		return err
	}
	if allOrdersExist {
		return tx.Commit(ctx)
	}
	var originalStatus string
	if err = tx.QueryRow(ctx, `SELECT status FROM public.riders WHERE id=$1`, PartnerID).Scan(&originalStatus); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.rider_locations(partner_id,latitude,longitude,accuracy_m,observed_at,permission_granted) VALUES($1,34,74,5,now(),true)`, PartnerID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE public.riders SET status='ONLINE' WHERE id=$1`, PartnerID); err != nil {
		return err
	}
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("10000000-0000-4000-a000-%012d", 100+i)
		status := "DELIVERED"
		if i == 7 {
			status = "PREPARING"
		}
		if i == 8 {
			status = "READY_FOR_PICKUP"
		}
		if i == 9 {
			status = "CREATED"
		}
		tag, err := tx.Exec(ctx, `INSERT INTO public.orders(id,restaurant_id,customer_id,status,total_price,currency,created_at,delivery_address) VALUES($1,$2,$3,$4,250,'INR',now()-make_interval(days=>$5),'{"address_line1":"12 Local Test Street","city":"Srinagar","state":"Jammu and Kashmir","postal_code":"190001","latitude":34,"longitude":74}') ON CONFLICT(id) DO NOTHING`, id, RestaurantID, CustomerID, status, 6-i%7)
		if err != nil {
			return fmt.Errorf("local order: %w", err)
		}
		if tag.RowsAffected() == 0 {
			continue
		}
		if _, err = tx.Exec(ctx, `INSERT INTO public.order_items(order_id,item_id,name_snapshot,quantity,unit_price,total_price) VALUES($1,'10000000-0000-4000-a000-000000000041','Local vegetable rice bowl',1,25000,25000)`, id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO public.order_status_history(order_id,status,notes) VALUES($1,$2,'Local development fixture')`, id, status); err != nil {
			return err
		}
		if status == "DELIVERED" || status == "READY_FOR_PICKUP" {
			assignmentStatus := "DELIVERED"
			if status == "READY_FOR_PICKUP" {
				assignmentStatus = "ASSIGNED"
			}
			if _, err = tx.Exec(ctx, `INSERT INTO public.delivery_assignments(order_id,partner_id,status) VALUES($1,$2,$3)`, id, PartnerID, assignmentStatus); err != nil {
				return err
			}
		}
		if status == "DELIVERED" {
			if _, err = tx.Exec(ctx, `INSERT INTO delivery.rider_earnings(order_id,partner_id,delivery_fee,tip_amount) VALUES($1,$2,4000,0)`, id, PartnerID); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO restaurants.restaurant_reviews(id,order_id,customer_id,restaurant_id,rating,comment) VALUES('10000000-0000-4000-a000-000000000060','10000000-0000-4000-a000-000000000100',$1,$2,5,'Local demo review for moderation testing') ON CONFLICT(id) DO NOTHING`, CustomerID, RestaurantID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.admin_audit_logs(id,admin_id,action,target_type,details) VALUES('10000000-0000-4000-a000-000000000070',$1,'LOCAL_FIXTURES_CREATED','development','{"reason":"Explicit local development setup"}') ON CONFLICT(id) DO NOTHING`, AdminID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE public.riders SET status=$2 WHERE id=$1`, PartnerID, originalStatus); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

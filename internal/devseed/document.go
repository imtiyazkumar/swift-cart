package devseed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// SeedDocument creates accounts and catalog fixtures using only the approved
// tables. It never fabricates paid fees, completed orders, refunds or payouts.
func SeedDocument(ctx context.Context, pool *pgxpool.Pool, password string) error {
	if len(password) < 12 || len(password) > 72 {
		return fmt.Errorf("LOCAL_SEED_PASSWORD must contain 12–72 bytes")
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
	for _, u := range []struct{ UUID, Email, Role, Name, Phone string }{
		{AdminID, "admin@yember.test", "ADMIN", "Local Administrator", "+919000000001"},
		{CustomerID, "customer@yember.test", "CUSTOMER", "Local Customer", "+919000000002"},
		{OwnerID, "restaurant@yember.test", "RESTAURANT_OWNER", "Local Restaurant Owner", "+919000000003"},
		{DeliveryUserID, "delivery@yember.test", "DELIVERY_PARTNER", "Local Delivery Agent", "+919000000004"},
	} {
		if _, err = tx.Exec(ctx, `INSERT INTO users(uuid,email,phone,full_name,password_hash,role,status) VALUES($1,$2,$3,$4,$5,$6,'ACTIVE') ON CONFLICT(uuid) DO NOTHING`, u.UUID, u.Email, u.Phone, u.Name, string(hash), u.Role); err != nil {
			return err
		}
	}
	if err = seedDocumentStations(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO restaurants(uuid,owner_user_id,station_id,name,address,latitude,longitude,status,is_open) SELECT $1,u.id,s.id,'Local Yember Kitchen','Local test pickup address',34,74,'PENDING',false FROM users u,stations s WHERE u.uuid=$2 AND s.uuid='20000000-0000-4000-a000-000000000001' ON CONFLICT(uuid) DO NOTHING`, RestaurantID, OwnerID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO riders(uuid,user_id,station_id,vehicle_type,verification_status,availability_status) SELECT $1,u.id,s.id,'MOTORCYCLE','PENDING','OFFLINE' FROM users u,stations s WHERE u.uuid=$2 AND s.uuid='20000000-0000-4000-a000-000000000001' ON CONFLICT(uuid) DO NOTHING`, PartnerID, DeliveryUserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO registration_fees(restaurant_id,amount_paise,currency,status) SELECT id,20000,'INR','DUE' FROM restaurants WHERE uuid=$1 ON CONFLICT DO NOTHING`, RestaurantID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO registration_fees(delivery_agent_id,amount_paise,currency,status) SELECT id,15000,'INR','DUE' FROM riders WHERE uuid=$1 ON CONFLICT DO NOTHING`, PartnerID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO menu_categories(uuid,name,sort_order,is_active) VALUES('10000000-0000-4000-a000-000000000030','Meals',1,true) ON CONFLICT(uuid) DO NOTHING`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO menu_items(uuid,restaurant_id,category_id,name,description,base_price,is_available) SELECT '10000000-0000-4000-a000-000000000041',r.id,c.id,'Local meal','Development menu item',350,true FROM restaurants r,menu_categories c WHERE r.uuid=$1 AND c.uuid='10000000-0000-4000-a000-000000000030' ON CONFLICT(uuid) DO NOTHING`, RestaurantID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func SeedDocumentStations(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = seedDocumentStations(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func seedDocumentStations(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `INSERT INTO stations(uuid,name,description,latitude,longitude,radius_km,status) VALUES('20000000-0000-4000-a000-000000000001','Local test station','Development service area',34,74,5,'ACTIVE') ON CONFLICT(uuid) DO NOTHING`)
	return err
}

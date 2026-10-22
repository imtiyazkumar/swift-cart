package stations

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Unserved = "Yember does not serve this area yet. Choose a location within an active station."
const DeliveryUnavailable = "Delivery is unavailable at this location. Choose another delivery address."
const LocationRequired = "A recent, accurate GPS location inside your assigned active station is required. Enable location access and return to your station to go online."

type Queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func Coordinates(lat, lon *float64) error {
	if lat == nil || lon == nil || math.IsNaN(*lat) || math.IsNaN(*lon) || math.IsInf(*lat, 0) || math.IsInf(*lon, 0) || *lat < -90 || *lat > 90 || *lon < -180 || *lon > 180 {
		return errors.New("valid latitude and longitude are required")
	}
	return nil
}

func Resolve(ctx context.Context, q Queryer, lat, lon *float64) (string, error) {
	if err := Coordinates(lat, lon); err != nil {
		return "", err
	}
	var id *string
	if err := q.QueryRow(ctx, `SELECT public.resolve_station($1,$2)::text`, *lat, *lon).Scan(&id); err != nil {
		return "", err
	}
	if id == nil {
		return "", errors.New(Unserved)
	}
	return *id, nil
}

// LockCoverage serializes eligibility checks with station edits until commit.
func LockCoverage(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(894215)`)
	return err
}

func ValidateRestaurant(ctx context.Context, q Queryer, id string) error {
	var valid bool
	if err := q.QueryRow(ctx, `SELECT COALESCE(public.station_covers(station_id,latitude,longitude),false) FROM public.merchants WHERE id=$1`, id).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return errors.New("restaurant location must be covered by its assigned active station; request an approved station change")
	}
	return nil
}

func ValidatePartner(ctx context.Context, q Queryer, id string) error {
	var valid bool
	if err := q.QueryRow(ctx, `SELECT public.partner_in_station($1)`, id).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return errors.New(LocationRequired)
	}
	return nil
}

// ExpireUnavailable never touches accepted assignments or their lifecycle.
func ExpireUnavailable(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `UPDATE delivery.delivery_partners p SET status='OFFLINE',updated_at=now() WHERE status='ONLINE' AND NOT public.partner_in_station(p.id)`); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE delivery.offers f SET status='EXPIRED',updated_at=now() WHERE f.status='PENDING' AND NOT EXISTS(SELECT 1 FROM delivery.delivery_partners p WHERE p.id=f.partner_id AND p.status='ONLINE' AND public.partner_in_station(p.id))`)
	return err
}

type Location struct {
	Latitude   *float64  `json:"latitude"`
	Longitude  *float64  `json:"longitude"`
	Accuracy   *float64  `json:"accuracy_m"`
	ObservedAt time.Time `json:"observed_at"`
	Permission bool      `json:"permission_granted"`
	Mocked     bool      `json:"mocked"`
}

func RecordLocation(ctx context.Context, pool *pgxpool.Pool, partner string, l Location) error {
	err := recordLocation(ctx, pool, partner, l)
	if err != nil {
		tx, e := pool.Begin(ctx)
		if e == nil {
			defer tx.Rollback(ctx)
			if e = LockCoverage(ctx, tx); e != nil {
				return err
			}
			if _, e = tx.Exec(ctx, `UPDATE delivery.delivery_partners SET status='OFFLINE',updated_at=now() WHERE id=$1`, partner); e == nil {
				if _, e = tx.Exec(ctx, `UPDATE delivery.offers SET status='EXPIRED',updated_at=now() WHERE partner_id=$1 AND status='PENDING'`, partner); e == nil {
					_ = tx.Commit(ctx)
				}
			}
		}
	}
	return err
}
func recordLocation(ctx context.Context, pool *pgxpool.Pool, partner string, l Location) error {
	if err := Coordinates(l.Latitude, l.Longitude); err != nil {
		return err
	}
	now := time.Now()
	if !l.Permission || l.Mocked || l.Accuracy == nil || math.IsNaN(*l.Accuracy) || math.IsInf(*l.Accuracy, 0) || *l.Accuracy < 0 || *l.Accuracy > 100 || l.ObservedAt.Before(now.Add(-2*time.Minute)) || l.ObservedAt.After(now.Add(30*time.Second)) {
		return errors.New("enable location permission and send a fresh, non-mocked GPS fix with accuracy within 100 metres")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = LockCoverage(ctx, tx); err != nil {
		return err
	}
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id FROM delivery.delivery_partners WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, partner).Scan(&locked); err != nil {
		return err
	}
	var previousLat, previousLon float64
	var observed time.Time
	err = tx.QueryRow(ctx, `SELECT latitude,longitude,observed_at FROM delivery.live_locations WHERE partner_id=$1 AND observed_at IS NOT NULL ORDER BY recorded_at DESC,id DESC LIMIT 1`, partner).Scan(&previousLat, &previousLon, &observed)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if err == nil {
		seconds := l.ObservedAt.Sub(observed).Seconds()
		if seconds <= 0 {
			return errors.New("GPS fix is older than the latest recorded location")
		}
		var distance float64
		if err = tx.QueryRow(ctx, `SELECT public.station_distance_m($1,$2,$3,$4)`, previousLat, previousLon, *l.Latitude, *l.Longitude).Scan(&distance); err != nil {
			return err
		}
		if distance > seconds*70+200 {
			return errors.New("GPS movement could not be verified; acquire a new accurate location fix")
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO delivery.live_locations(partner_id,latitude,longitude,accuracy_m,observed_at,permission_granted,mocked) VALUES($1,$2,$3,$4,$5,true,false)`, partner, *l.Latitude, *l.Longitude, *l.Accuracy, l.ObservedAt); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE delivery.delivery_partners SET status='OFFLINE',updated_at=now() WHERE id=$1 AND status='ONLINE' AND NOT public.partner_in_station(id)`, partner); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE delivery.offers SET status='EXPIRED',updated_at=now() WHERE partner_id=$1 AND status='PENDING' AND NOT public.partner_in_station($1)`, partner); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type stationContextKey struct{}

func WithStation(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, stationContextKey{}, id)
}
func ContextStation(ctx context.Context) string {
	id, _ := ctx.Value(stationContextKey{}).(string)
	return id
}
func OwnedAddress(ctx context.Context, q Queryer, customer, address string) (string, error) {
	var lat, lon *float64
	if err := q.QueryRow(ctx, `SELECT latitude,longitude FROM users.customer_addresses WHERE id=$1 AND customer_id=$2 AND deleted_at IS NULL FOR SHARE`, address, customer).Scan(&lat, &lon); err != nil {
		return "", errors.New("delivery address not found")
	}
	id, err := Resolve(ctx, q, lat, lon)
	if err != nil {
		return "", errors.New(DeliveryUnavailable)
	}
	return id, nil
}

// UpdateRestaurantLocation preserves the operational assignment; transfers require approval.
func UpdateRestaurantLocation(ctx context.Context, tx pgx.Tx, id, address string, lat, lon *float64) error {
	if err := LockCoverage(ctx, tx); err != nil {
		return err
	}
	var previous string
	if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT address FROM merchants.merchant_branches WHERE merchant_id=m.id AND deleted_at IS NULL ORDER BY created_at,id LIMIT 1),'') FROM public.merchants m WHERE id=$1 FOR UPDATE`, id).Scan(&previous); err != nil {
		return err
	}
	if lat == nil && lon == nil && (address == "" || previous == address) {
		return ValidateRestaurant(ctx, tx, id)
	}
	if err := Coordinates(lat, lon); err != nil {
		return errors.New("valid restaurant coordinates are required when changing its address")
	}
	var valid bool
	if err := tx.QueryRow(ctx, `SELECT public.station_covers(station_id,$2,$3) FROM public.merchants WHERE id=$1`, id, *lat, *lon).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return errors.New("new restaurant location is outside its assigned station; request a station-change approval")
	}
	_, err := tx.Exec(ctx, `UPDATE public.merchants SET latitude=$2,longitude=$3,updated_at=now() WHERE id=$1`, id, *lat, *lon)
	return err
}

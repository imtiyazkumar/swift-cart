// Package devseed owns the current development fixtures and target guard.
package devseed

import (
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"net"
	"strings"
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

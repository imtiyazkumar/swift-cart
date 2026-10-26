// finance_access is an offline operator tool, deliberately not an HTTP privilege-escalation endpoint.
package main

import (
	"context"
	"errors"
	"flag"
	"github.com/imtiyazkumar/yember-core/config"
	"github.com/imtiyazkumar/yember-core/pkg/db"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"log"
	"strings"
)

func main() {
	email := flag.String("email", "", "existing administrator email")
	grant := flag.String("grant", "READ", "comma-separated finance permissions")
	revoke := flag.Bool("revoke", false, "remove selected permissions")
	flag.Parse()
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	pool, err := db.NewPool(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err = provision(context.Background(), pool, *email, *grant, *revoke); err != nil {
		log.Fatal(err)
	}
	log.Print("finance permissions updated")
}
func provision(ctx context.Context, pool *pgxpool.Pool, email, grant string, revoke bool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, `SELECT id FROM public.users WHERE LOWER(email)=LOWER($1) AND role='ADMIN' AND disabled_at IS NULL AND deleted_at IS NULL`, email).Scan(&id); err != nil {
		return errors.New("active administrator required")
	}
	allowed := map[string]bool{"READ": true, "CONFIGURE": true, "PREPARE": true, "APPROVE": true, "PAYOUT": true, "RECONCILE": true, "ADJUST": true}
	for _, permission := range strings.Split(grant, ",") {
		permission = strings.TrimSpace(permission)
		if !allowed[permission] {
			return errors.New("unsupported finance permission")
		}
		query := `INSERT INTO finance.permissions(user_id,permission) VALUES($1,$2) ON CONFLICT DO NOTHING`
		if revoke {
			query = `DELETE FROM finance.permissions WHERE user_id=$1 AND permission=$2`
		}
		if _, err = tx.Exec(ctx, query, id, permission); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.admin_audit_logs(admin_id,action,target_type,target_id,details) VALUES($1,'FINANCE_ACCESS_PROVISIONED','user',$2,jsonb_build_object('permissions',$3::text,'revoked',$4::boolean))`, id, id, grant, revoke); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

package devseed

import (
	"context"
	"testing"

	"github.com/imtiyazkumar/yember-core/internal/auth"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
)

func TestLocalTargetGuard(t *testing.T) {
	for _, input := range []struct {
		environment, dsn string
		allowed          bool
	}{
		{"development", "postgres://localhost/yember_local", true},
		{"test", "postgres://127.0.0.1/yember_local_test", true},
		{"production", "postgres://localhost/yember_local", false},
		{"development", "postgres://remote.example/yember_local", false},
		{"development", "postgres://localhost/yember", false},
	} {
		if allowed := ValidateTarget(input.environment, input.dsn) == nil; allowed != input.allowed {
			t.Fatalf("unexpected target guard result for %s", input.environment)
		}
	}
}

func TestSeedLoginsProfilesAndIdempotency(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	const password = "LocalTestPassword!2026"
	for i := 0; i < 2; i++ {
		if err := Seed(ctx, pool, password); err != nil {
			t.Fatal(err)
		}
	}
	for email, role := range map[string]string{"admin@yember.test": "ADMIN", "customer@yember.test": "CUSTOMER", "restaurant@yember.test": "RESTAURANT_OWNER", "delivery@yember.test": "DELIVERY_PARTNER"} {
		user, err := auth.FindByEmail(ctx, pool, email)
		if err != nil || user.Role != role || user.VerifyPassword(password) != nil {
			t.Fatalf("seed login failed: %s", email)
		}
	}
	for table, count := range map[string]int{"public.users": 4, "public.orders": 10, "public.delivery_assignments": 8, "delivery.rider_earnings": 7, "public.admin_audit_logs": 1, "restaurants.restaurant_operating_hours": 7} {
		var actual int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&actual); err != nil || actual != count {
			t.Fatalf("%s: got %d, want %d: %v", table, actual, count, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE public.users SET full_name='Edited in admin',disabled_at=now() WHERE id=$1`, CustomerID); err != nil {
		t.Fatal(err)
	}
	if err := Seed(ctx, pool, password); err != nil {
		t.Fatal(err)
	}
	var preserved bool
	if err := pool.QueryRow(ctx, `SELECT full_name='Edited in admin' AND disabled_at IS NOT NULL FROM public.users WHERE id=$1`, CustomerID).Scan(&preserved); err != nil || !preserved {
		t.Fatal("rerunning seed overwrote account changes")
	}
}

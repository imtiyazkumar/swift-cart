package devseed

import "testing"

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

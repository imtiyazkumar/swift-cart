package db

import (
	"context"
	"fmt"
	"github.com/imtiyazkumar/yember-core/internal/contractschema"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CheckSchema accepts only the 36 documented tables plus coupons and usages.
func CheckSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if err := contractschema.Validate(ctx, pool); err != nil {
		return fmt.Errorf("database does not match the document schema; run go run ./cmd/migrate after completing a lossless legacy conversion: %w", err)
	}
	return nil
}

package finance

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

func Coupon(ctx context.Context, tx pgx.Tx, code string, gross int64) (string, int64, error) {
	if code == "" {
		return "", 0, nil
	}
	if len(code) > 100 || gross <= 0 || gross > 1000000000 {
		return "", 0, errors.New("invalid promotion basis")
	}
	var id, kind string
	var value, minimum int64
	var maximum *int64
	err := tx.QueryRow(ctx, `SELECT id,discount_type,discount_value,min_order_amount,max_discount_amount FROM orders.coupons WHERE UPPER(code)=UPPER($1) AND is_active AND valid_from<=now() AND (valid_until IS NULL OR valid_until>now()) AND (usage_limit IS NULL OR used_count<usage_limit) FOR UPDATE`, code).Scan(&id, &kind, &value, &minimum, &maximum)
	if err != nil || gross < minimum {
		return "", 0, errors.New("coupon expired, exhausted or not eligible for this cart")
	}
	discount := value
	if kind == "PERCENTAGE" {
		if value > 100 {
			return "", 0, errors.New("invalid promotion rate")
		}
		discount = (gross * value) / 100
	}
	if maximum != nil && discount > *maximum {
		discount = *maximum
	}
	if discount > gross {
		discount = gross
	}
	if discount <= 0 {
		return "", 0, errors.New("invalid promotion discount")
	}
	return id, discount, nil
}

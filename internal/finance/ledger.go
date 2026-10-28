package finance

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

type Line struct {
	Account, User, Component string
	Amount                   int64
}

// Positive is debit; negative is credit. Liabilities/revenue normally have credit balances.
func post(ctx context.Context, tx pgx.Tx, key, kind, order, reference, reason, actor string, lines ...Line) error {
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO finance.journals(event_key,kind,order_id,reference,reason,actor) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,NULLIF($6,'')::uuid) ON CONFLICT(event_key) DO NOTHING RETURNING id`, key, kind, order, reference, reason, actor).Scan(&id)
	if err == pgx.ErrNoRows {
		return checkReplay(ctx, tx, key, kind, order, reference, lines)
	}
	if err != nil {
		return err
	}
	var sum int64
	for _, l := range lines {
		if l.Amount == 0 {
			continue
		}
		sum += l.Amount
		if _, err = tx.Exec(ctx, `INSERT INTO finance.entries(journal_id,account,user_id,component,amount) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5)`, id, l.Account, l.User, l.Component, l.Amount); err != nil {
			return err
		}
	}
	if sum != 0 {
		return errors.New("unbalanced allocation")
	}
	return nil
}

// Snapshot runs inside the order placement transaction. No historical commission is backfilled.
func Snapshot(ctx context.Context, tx pgx.Tx, order string, codes ...string) error {
	var gross int64
	var bps int
	var user string
	var charge, rider int64
	err := tx.QueryRow(ctx, `SELECT ROUND(o.total_price*100)::bigint,m.owner_id,m.commission_bps,p.delivery_charge,p.rider_base FROM public.orders o JOIN public.restaurants m ON m.id=o.restaurant_id CROSS JOIN finance.policy p WHERE o.id=$1`, order).Scan(&gross, &user, &bps, &charge, &rider)
	if err != nil {
		return err
	}
	code := ""
	if len(codes) > 0 {
		code = codes[0]
	}
	couponID, discount, err := Coupon(ctx, tx, code, gross)
	if err != nil {
		return err
	}
	commission, err := Commission(gross, bps)
	if err != nil {
		return err
	}
	if rider > charge+commission {
		return errors.New("rider pricing exceeds configured order margin")
	}
	_, err = tx.Exec(ctx, `INSERT INTO finance.order_financials(order_id,restaurant_user,gross,commission_bps,commission,delivery_charge,rider_base,total,discount) VALUES($1,$2,$3,$4,$5,$6,$7,$3::bigint+$6::bigint-$8::bigint,$8)`, order, user, gross, bps, commission, charge, rider, discount)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE public.orders SET total_price=($2::bigint+$3::bigint-$5::bigint)::numeric/100,delivery_fee=$4 WHERE id=$1`, order, gross, charge, rider, discount)
	if err != nil {
		return err
	}
	if couponID != "" {
		if _, err = tx.Exec(ctx, `INSERT INTO finance.coupon_redemptions(order_id,coupon_id,amount) VALUES($1,$2,$3)`, order, couponID, discount); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE orders.coupons SET used_count=used_count+1 WHERE id=$1`, couponID)
	}
	return err
}

// Recognize releases unearned receipts only after delivery AND centrally confirmed collection.
// It is safe to call from both delivery and payment confirmation transactions.
func Recognize(ctx context.Context, tx pgx.Tx, order string) error {
	var status, restaurant string
	var total, gross, discount, commission, charge, rider, tax, platform int64
	err := tx.QueryRow(ctx, `SELECT o.status,f.restaurant_user,f.total,f.gross,f.discount,f.commission,f.delivery_charge,f.rider_base,f.tax,f.platform_charge FROM public.orders o JOIN finance.order_financials f ON f.order_id=o.id WHERE o.id=$1 FOR UPDATE OF o`, order).Scan(&status, &restaurant, &total, &gross, &discount, &commission, &charge, &rider, &tax, &platform)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if status != "DELIVERED" {
		return nil
	}
	var paid bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM finance.payment_attempts WHERE order_id=$1 AND status='PAID') AND NOT EXISTS(SELECT 1 FROM finance.journals WHERE order_id=$1 AND kind='REFUND')`, order).Scan(&paid); err != nil {
		return err
	}
	if !paid {
		return nil
	}
	agent := ""
	if rider > 0 {
		if err = tx.QueryRow(ctx, `SELECT p.user_id FROM public.delivery_assignments a JOIN public.riders p ON p.id=a.partner_id WHERE a.order_id=$1 AND a.status='DELIVERED'`, order).Scan(&agent); err != nil {
			return errors.New("delivery payee is unavailable")
		}
	}
	return post(ctx, tx, "earn/"+order, "EARN", order, order, "Paid and delivered order", "",
		Line{"UNEARNED", "", "release", total}, Line{"PARTNER_PAYABLE", restaurant, "restaurant_gross", -gross},
		Line{"PARTNER_PAYABLE", restaurant, "commission", commission}, Line{"COMMISSION", "", "commission", -commission}, Line{"PROMOTION_EXPENSE", "", "platform_promotion", discount},
		Line{"PARTNER_PAYABLE", agent, "delivery", -rider}, Line{"DELIVERY_REVENUE", "", "delivery_margin", -(charge + platform - rider)}, Line{"TAX_PAYABLE", "", "tax", -tax})
}

func capture(ctx context.Context, tx pgx.Tx, order, attempt, reference string, amount int64) error {
	var expected int64
	if err := tx.QueryRow(ctx, `SELECT total FROM finance.order_financials WHERE order_id=$1`, order).Scan(&expected); err != nil {
		return err
	}
	if amount != expected {
		return errors.New("payment amount mismatch")
	}
	tag, err := tx.Exec(ctx, `UPDATE finance.payment_attempts SET status='PAID',payment_reference=$2 WHERE id=$1 AND order_id=$3 AND status IN ('CREATING','PENDING','PAID','EXPIRED') AND (payment_reference IS NULL OR payment_reference=$2)`, attempt, reference, order)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("payment attempt cannot be captured")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO finance.refund_jobs(order_id) SELECT id FROM public.orders WHERE id=$1 AND status IN ('CANCELLED','REJECTED') ON CONFLICT DO NOTHING`, order); err != nil {
		return err
	}
	if err := post(ctx, tx, "capture/"+order, "CAPTURE", order, reference, "Provider-confirmed collection", "", Line{"CASH", "", "collection", amount}, Line{"UNEARNED", "", "order_receipt", -amount}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE public.orders SET status='CONFIRMED',updated_at=now() WHERE id=$1 AND status='PAYMENT_PENDING'`, order); err != nil {
		return err
	}
	return Recognize(ctx, tx, order)
}

func checkReplay(ctx context.Context, tx pgx.Tx, key, kind, order, reference string, lines []Line) error {
	var id, priorKind, priorOrder, priorReference string
	if err := tx.QueryRow(ctx, `SELECT id,kind,COALESCE(order_id::text,''),reference FROM finance.journals WHERE event_key=$1`, key).Scan(&id, &priorKind, &priorOrder, &priorReference); err != nil {
		return err
	}
	if priorKind != kind || priorOrder != order || priorReference != reference {
		return errors.New("idempotency key conflicts with an existing financial operation")
	}
	expected := map[string]int64{}
	for _, l := range lines {
		if l.Amount != 0 {
			expected[l.Account+"/"+l.User+"/"+l.Component] += l.Amount
		}
	}
	rows, err := tx.Query(ctx, `SELECT account,COALESCE(user_id::text,''),component,sum(amount)::bigint FROM finance.entries WHERE journal_id=$1 GROUP BY account,user_id,component`, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var account, user, component string
		var amount int64
		if err = rows.Scan(&account, &user, &component, &amount); err != nil {
			return err
		}
		k := account + "/" + user + "/" + component
		if expected[k] != amount {
			return errors.New("idempotency key financial terms changed")
		}
		delete(expected, k)
	}
	if rows.Err() != nil {
		return rows.Err()
	}
	if len(expected) != 0 {
		return errors.New("idempotency key allocation changed")
	}
	return nil
}

package platform

import (
	"fmt"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

type posting struct {
	Account string
	Amount  int64
}
type links struct{ Order, Payment, RestaurantEarning, AgentEarning, RestaurantPayout, AgentPayout, RegistrationFee *int64 }

func ptr(v int64) *int64 { return &v }
func post(c echo.Context, tx pgx.Tx, key, kind string, ref links, lines ...posting) error {
	var total int64
	for _, line := range lines {
		total += line.Amount
	}
	if total != 0 {
		return fmt.Errorf("unbalanced %s posting", kind)
	}
	var prior bool
	if err := tx.QueryRow(c.Request().Context(), `SELECT EXISTS(SELECT 1 FROM financial_transactions WHERE event_key=$1||':0')`, key).Scan(&prior); err != nil {
		return err
	}
	if prior {
		return nil
	}
	index := 0
	for _, line := range lines {
		if line.Amount == 0 {
			continue
		}
		direction := "DEBIT"
		amount := line.Amount
		if amount < 0 {
			amount = -amount
			direction = "CREDIT"
		}
		if _, err := tx.Exec(c.Request().Context(), `INSERT INTO financial_transactions(order_id,payment_id,restaurant_earning_id,agent_earning_id,restaurant_payout_id,agent_payout_id,registration_fee_id,transaction_type,account_code,direction,amount_paise,currency,event_key,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'INR',$12,now())`, ref.Order, ref.Payment, ref.RestaurantEarning, ref.AgentEarning, ref.RestaurantPayout, ref.AgentPayout, ref.RegistrationFee, kind, line.Account, direction, amount, fmt.Sprintf("%s:%d", key, index)); err != nil {
			return err
		}
		index++
	}
	return nil
}
func commissionBPS() (int64, error) {
	value := os.Getenv("RESTAURANT_COMMISSION_BPS")
	if value == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 || n > 10000 {
		return 0, fmt.Errorf("invalid commission configuration")
	}
	return n, nil
}
func (s *Service) recognize(c echo.Context, tx pgx.Tx, order int64) error {
	var restaurant, total, gross, discount, delivery, tax int64
	var status string
	if err := tx.QueryRow(c.Request().Context(), `SELECT restaurant_id,round(total_amount*100)::bigint,round(subtotal*100)::bigint,round(discount_amount*100)::bigint,round(delivery_fee*100)::bigint,round(tax_amount*100)::bigint,status FROM orders WHERE id=$1 FOR UPDATE`, order).Scan(&restaurant, &total, &gross, &discount, &delivery, &tax, &status); err != nil {
		return err
	}
	if status != "DELIVERED" {
		return nil
	}
	var payment int64
	if err := tx.QueryRow(c.Request().Context(), `SELECT id FROM payments WHERE order_id=$1 AND status='CAPTURED' AND amount_paise=$2`, order, total).Scan(&payment); err == pgx.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	var existing bool
	if err := tx.QueryRow(c.Request().Context(), `SELECT EXISTS(SELECT 1 FROM restaurant_earnings WHERE order_id=$1)`, order).Scan(&existing); err != nil {
		return err
	}
	if existing {
		return nil
	}
	bps, err := commissionBPS()
	if err != nil {
		return err
	}
	fee := (gross*bps + 5000) / 10000
	var earning int64
	if err = tx.QueryRow(c.Request().Context(), `INSERT INTO restaurant_earnings(restaurant_id,order_id,gross_amount_paise,fee_amount_paise,adjustment_amount_paise,net_amount_paise,status,eligible_at) VALUES($1,$2,$3,$4,0,$3::bigint-$4::bigint,'AVAILABLE',now()) RETURNING id`, restaurant, order, gross, fee).Scan(&earning); err != nil {
		return err
	}
	var rider int64
	var agent *int64
	if err = tx.QueryRow(c.Request().Context(), `SELECT rider_id FROM delivery_assignments WHERE order_id=$1 AND status='DELIVERED' AND deleted_at IS NULL`, order).Scan(&rider); err == nil {
		var id int64
		if err = tx.QueryRow(c.Request().Context(), `INSERT INTO delivery_agent_earnings(delivery_agent_id,order_id,base_amount_paise,incentive_amount_paise,adjustment_amount_paise,net_amount_paise,status,eligible_at) VALUES($1,$2,$3,0,0,$3,'AVAILABLE',now()) RETURNING id`, rider, order, delivery).Scan(&id); err != nil {
			return err
		}
		agent = &id
	} else if err != pgx.ErrNoRows {
		return err
	}
	lines := []posting{{"ORDER_RECEIPTS", total}, {"PROMOTION_EXPENSE", discount}, {"RESTAURANT_PAYABLE", -(gross - fee)}, {"PLATFORM_FEE_REVENUE", -fee}, {"TAX_PAYABLE", -tax}}
	if agent != nil {
		lines = append(lines, posting{"AGENT_PAYABLE", -delivery})
	} else {
		lines = append(lines, posting{"DELIVERY_REVENUE", -delivery})
	}
	return post(c, tx, fmt.Sprintf("earning/%d", order), "EARNING", links{Order: ptr(order), Payment: ptr(payment), RestaurantEarning: ptr(earning), AgentEarning: agent}, lines...)
}
func (s *Service) collectCash(c echo.Context, tx pgx.Tx, order, rider, received int64) error {
	var expected int64
	if err := tx.QueryRow(c.Request().Context(), `SELECT round(total_amount*100)::bigint FROM orders WHERE id=$1 FOR UPDATE`, order).Scan(&expected); err != nil {
		return err
	}
	if received < 0 || received > 100000000000 {
		return bad("valid nonnegative cash receipt amount required")
	}
	var collected bool
	var online bool
	if err := tx.QueryRow(c.Request().Context(), `SELECT EXISTS(SELECT 1 FROM payments WHERE order_id=$1 AND status='CAPTURED'),EXISTS(SELECT 1 FROM payment_intents WHERE order_id=$1 AND method='RAZORPAY' AND status IN ('CREATING','PENDING','UNKNOWN'))`, order).Scan(&collected, &online); err != nil {
		return err
	}
	if collected {
		return nil
	}
	if online {
		return bad("online payment must be reconciled before taking cash")
	}
	var intent, payment int64
	if err := tx.QueryRow(c.Request().Context(), `SELECT id FROM payment_intents WHERE order_id=$1 AND method='PAY_ON_DELIVERY' ORDER BY id DESC LIMIT 1 FOR UPDATE`, order).Scan(&intent); err != nil {
		return bad("pay on delivery intent required")
	}
	if received > 0 {
		if err := tx.QueryRow(c.Request().Context(), `INSERT INTO payments(payment_intent_id,order_id,provider,method,amount_paise,currency,status,received_by,captured_at) VALUES($1,$2,'CASH','CASH',$3,'INR','CAPTURED','DELIVERY_AGENT',now()) RETURNING id`, intent, order, received).Scan(&payment); err != nil {
			return err
		}
	}
	status := "COLLECTED"
	intentStatus := "SUCCEEDED"
	if received != expected {
		status = "PENDING_REVIEW"
		intentStatus = "PENDING_REVIEW"
	}
	if _, err := tx.Exec(c.Request().Context(), `UPDATE payment_intents SET status=$2 WHERE id=$1`, intent, intentStatus); err != nil {
		return err
	}
	if _, err := tx.Exec(c.Request().Context(), `INSERT INTO cash_collections(order_id,delivery_agent_id,restaurant_id,amount_due_paise,amount_collected_paise,collected_at,status) SELECT id,$2,restaurant_id,$3,$4,now(),$5 FROM orders WHERE id=$1`, order, rider, expected, received, status); err != nil {
		return err
	}
	if received != expected {
		if _, err := tx.Exec(c.Request().Context(), `INSERT INTO reconciliation_runs(source,status,difference_paise,period_start,period_end,summary_json) VALUES('CASH','EXCEPTIONS',$1,now(),now(),jsonb_build_object('order_uuid',(SELECT uuid FROM orders WHERE id=$2),'amount_due_paise',$3::bigint,'amount_collected_paise',$4::bigint,'reason','Cash receipt differs from order amount'))`, received-expected, order, expected, received); err != nil {
			return err
		}
	}
	if received == 0 {
		return nil
	}
	return post(c, tx, fmt.Sprintf("cash/%d", order), "COLLECTION", links{Order: ptr(order), Payment: ptr(payment)}, posting{"CASH_IN_TRANSIT", received}, posting{"ORDER_RECEIPTS", -received})
}

func (s *Service) registerCash(api *echo.Group) {
	partner := api.Group("/partner/finance", roles("RESTAURANT_OWNER", "DELIVERY_PARTNER"))
	partner.GET("/cash-handovers", func(c echo.Context) error {
		return s.list(c, `SELECT jsonb_build_object('id',cash.uuid,'order_id',o.uuid,'amount_paise',cash.amount_collected_paise,'status',COALESCE(p.status,cash.status)) FROM cash_collections cash JOIN orders o ON o.id=cash.order_id JOIN restaurants restaurant ON restaurant.id=cash.restaurant_id JOIN riders rider ON rider.id=cash.delivery_agent_id LEFT JOIN restaurant_payouts p ON p.id=cash.restaurant_payout_id WHERE ($1='RESTAURANT_OWNER' AND restaurant.owner_user_id=$2) OR ($1='DELIVERY_PARTNER' AND rider.user_id=$2) ORDER BY cash.created_at DESC LIMIT 100`, person(c).Role, person(c).ID)
	})
	partner.POST("/cash-handovers/:id", func(c echo.Context) error {
		if publicID(c.Param("id")) != nil {
			return bad("order UUID required")
		}
		var input struct {
			Amount int64 `json:"amount_paise"`
		}
		if c.Bind(&input) != nil || input.Amount <= 0 {
			return bad("positive gross cash amount required")
		}
		tx, err := s.begin(c, "PAYOUT")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		var collection, order, restaurant, rider int64
		var payout *int64
		var expected, due int64
		if err = tx.QueryRow(c.Request().Context(), `SELECT cash.id,cash.order_id,cash.restaurant_id,cash.delivery_agent_id,cash.restaurant_payout_id,cash.amount_collected_paise,cash.amount_due_paise FROM cash_collections cash JOIN orders o ON o.id=cash.order_id JOIN riders r ON r.id=cash.delivery_agent_id WHERE o.uuid=$1 AND r.user_id=$2 AND o.status='DELIVERED' FOR UPDATE OF cash,o`, c.Param("id"), person(c).ID).Scan(&collection, &order, &restaurant, &rider, &payout, &expected, &due); err != nil {
			return unavailable(err)
		}
		if payout != nil {
			var amount int64
			if err = tx.QueryRow(c.Request().Context(), `SELECT amount_paise FROM restaurant_payouts WHERE id=$1`, *payout).Scan(&amount); err != nil {
				return err
			}
			if amount != input.Amount {
				return bad("handover already recorded with another amount")
			}
			return c.NoContent(204)
		}
		status := "PENDING"
		if input.Amount != expected || expected != due {
			status = "PENDING_REVIEW"
		}
		var id int64
		if err = tx.QueryRow(c.Request().Context(), `INSERT INTO restaurant_payouts(restaurant_id,delivery_agent_id,order_id,method,amount_paise,status) VALUES($1,$2,$3,'CASH_HANDOVER',$4,$5) RETURNING id`, restaurant, rider, order, input.Amount, status).Scan(&id); err != nil {
			return err
		}
		if _, err = tx.Exec(c.Request().Context(), `UPDATE cash_collections SET restaurant_payout_id=$2,status=$3 WHERE id=$1`, collection, id, status); err != nil {
			return err
		}
		if status == "PENDING_REVIEW" {
			if _, err = tx.Exec(c.Request().Context(), `INSERT INTO reconciliation_runs(source,status,difference_paise,period_start,period_end,summary_json) VALUES('CASH','EXCEPTIONS',$1,now(),now(),jsonb_build_object('order_uuid',$2::text,'reason','Gross cash handover mismatch'))`, input.Amount-expected, c.Param("id")); err != nil {
				return err
			}
		}
		return commit(c, tx)
	}, roles("DELIVERY_PARTNER"))
	partner.POST("/cash-handovers/:id/confirm", func(c echo.Context) error {
		if publicID(c.Param("id")) != nil {
			return bad("order UUID required")
		}
		var input struct {
			Amount int64 `json:"amount_paise"`
			Accept bool  `json:"accept"`
		}
		if c.Bind(&input) != nil {
			return bad("handover acknowledgement required")
		}
		tx, err := s.begin(c, "PAYOUT")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		var payout, order, amount, collected int64
		var status string
		if err = tx.QueryRow(c.Request().Context(), `SELECT p.id,p.order_id,p.amount_paise,cash.amount_collected_paise,p.status FROM restaurant_payouts p JOIN restaurants r ON r.id=p.restaurant_id JOIN orders o ON o.id=p.order_id JOIN cash_collections cash ON cash.restaurant_payout_id=p.id WHERE o.uuid=$1 AND r.owner_user_id=$2 AND p.method='CASH_HANDOVER' FOR UPDATE OF p,o,cash`, c.Param("id"), person(c).ID).Scan(&payout, &order, &amount, &collected, &status); err != nil {
			return unavailable(err)
		}
		if input.Amount != amount {
			return bad("acknowledgement must match submitted gross amount")
		}
		if status == "PAID" {
			if input.Accept {
				return c.NoContent(204)
			}
			return bad("paid handover requires reconciliation")
		}
		if !input.Accept {
			if _, err = tx.Exec(c.Request().Context(), `UPDATE restaurant_payouts SET status='PENDING_REVIEW' WHERE id=$1;`, payout); err != nil {
				return err
			}
			if _, err = tx.Exec(c.Request().Context(), `UPDATE cash_collections SET status='DISPUTED' WHERE restaurant_payout_id=$1`, payout); err != nil {
				return err
			}
			return commit(c, tx)
		}
		if status != "PENDING" || amount != collected {
			return bad("mismatched or disputed handover requires reconciliation")
		}
		if err = s.recognize(c, tx, order); err != nil {
			return err
		}
		var earning, net, allocated int64
		if err = tx.QueryRow(c.Request().Context(), `SELECT e.id,e.net_amount_paise,COALESCE((SELECT sum(a.amount_paise) FROM restaurant_payout_allocations a JOIN restaurant_payouts p ON p.id=a.restaurant_payout_id WHERE a.restaurant_earning_id=e.id AND p.status NOT IN ('FAILED','REVERSED')),0) FROM restaurant_earnings e WHERE e.order_id=$1 FOR UPDATE OF e`, order).Scan(&earning, &net, &allocated); err != nil {
			return err
		}
		if allocated != 0 {
			return bad("earnings are already reserved for another payout")
		}
		eligible := min(amount, net)
		if eligible > 0 {
			if _, err = tx.Exec(c.Request().Context(), `INSERT INTO restaurant_payout_allocations(restaurant_payout_id,restaurant_earning_id,amount_paise) VALUES($1,$2,$3)`, payout, earning, eligible); err != nil {
				return err
			}
		}
		if err = post(c, tx, fmt.Sprintf("handover/%d", payout), "PAYOUT", links{Order: ptr(order), RestaurantEarning: ptr(earning), RestaurantPayout: ptr(payout)}, posting{"CASH_IN_TRANSIT", -amount}, posting{"RESTAURANT_PAYABLE", eligible}, posting{"RESTAURANT_FEE_RECEIVABLE", amount - eligible}); err != nil {
			return err
		}
		if _, err = tx.Exec(c.Request().Context(), `UPDATE restaurant_payouts SET status='PAID',confirmed_by_restaurant_at=now(),paid_at=now() WHERE id=$1`, payout); err != nil {
			return err
		}
		if _, err = tx.Exec(c.Request().Context(), `UPDATE cash_collections SET status='HANDED_OVER' WHERE restaurant_payout_id=$1`, payout); err != nil {
			return err
		}
		if eligible == net {
			if _, err = tx.Exec(c.Request().Context(), `UPDATE restaurant_earnings SET status='SETTLED' WHERE id=$1`, earning); err != nil {
				return err
			}
		}
		return commit(c, tx)
	}, roles("RESTAURANT_OWNER"))
}

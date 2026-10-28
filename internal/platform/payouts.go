package platform

import (
	"fmt"

	"github.com/labstack/echo/v4"
)

type payoutKind struct{ Payout, Earning, Allocation, OwnerColumn, EarningColumn, PayoutColumn, Bank, BankOwner string }

func payoutTables(kind string) (payoutKind, bool) {
	if kind == "restaurant" {
		return payoutKind{"restaurant_payouts", "restaurant_earnings", "restaurant_payout_allocations", "restaurant_id", "restaurant_earning_id", "restaurant_payout_id", "restaurant_bank_accounts", "restaurant_id"}, true
	}
	if kind == "agent" {
		return payoutKind{"delivery_agent_payouts", "delivery_agent_earnings", "delivery_agent_payout_allocations", "delivery_agent_id", "delivery_agent_earning_id", "delivery_agent_payout_id", "delivery_agent_bank_details", "rider_id"}, true
	}
	return payoutKind{}, false
}
func (s *Service) registerPayouts(api *echo.Group) {
	admin := api.Group("/admin", roles("ADMIN"))
	admin.POST("/payouts/:kind", func(c echo.Context) error {
		cfg, ok := payoutTables(c.Param("kind"))
		if !ok {
			return bad("restaurant or agent payout required")
		}
		var input struct {
			Earnings []string `json:"earning_ids"`
			Bank     string   `json:"bank_id"`
			Reason   string   `json:"reason"`
		}
		if c.Bind(&input) != nil || len(input.Earnings) == 0 || len(input.Earnings) > 100 || publicID(input.Bank) != nil || !validName(input.Reason, 2000) {
			return bad("earnings, verified bank account and reason required")
		}
		tx, err := s.begin(c, "PAYOUT")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), `SELECT pg_advisory_xact_lock(729403),set_config('yember.reason',$1,true)`, input.Reason); err != nil {
			return err
		}
		var owner, bank int64
		if err = tx.QueryRow(c.Request().Context(), "SELECT id,"+cfg.BankOwner+" FROM "+cfg.Bank+" WHERE uuid=$1 AND verification_status='VERIFIED' AND deleted_at IS NULL FOR UPDATE", input.Bank).Scan(&bank, &owner); err != nil {
			return bad("verified current bank account required")
		}
		if cfg.OwnerColumn == "restaurant_id" {
			var held bool
			if err = tx.QueryRow(c.Request().Context(), `SELECT EXISTS(SELECT 1 FROM cash_collections WHERE restaurant_id=$1 AND status<>'HANDED_OVER')`, owner).Scan(&held); err != nil {
				return err
			}
			if held {
				return bad("unresolved cash collections must be reconciled first")
			}
		}
		type allocation struct{ ID, Amount int64 }
		allocations := []allocation{}
		seen := map[string]bool{}
		var amount int64
		for _, uid := range input.Earnings {
			if publicID(uid) != nil || seen[uid] {
				return bad("unique earning UUIDs required")
			}
			seen[uid] = true
			var id, net, allocated int64
			query := "SELECT e.id,e.net_amount_paise,COALESCE((SELECT sum(a.amount_paise) FROM " + cfg.Allocation + " a JOIN " + cfg.Payout + " p ON p.id=a." + cfg.PayoutColumn + " WHERE a." + cfg.EarningColumn + "=e.id AND p.status NOT IN ('FAILED','REVERSED')),0) FROM " + cfg.Earning + " e WHERE e.uuid=$1 AND e." + cfg.OwnerColumn + "=$2 AND e.status='AVAILABLE' AND e.deleted_at IS NULL FOR UPDATE OF e"
			if err = tx.QueryRow(c.Request().Context(), query, uid, owner).Scan(&id, &net, &allocated); err != nil {
				return bad("available earnings for the bank account owner required")
			}
			remaining := net - allocated
			if remaining <= 0 {
				return bad("earning is already fully allocated")
			}
			allocations = append(allocations, allocation{id, remaining})
			amount += remaining
		}
		var payout int64
		var uid string
		if cfg.Payout == "restaurant_payouts" {
			err = tx.QueryRow(c.Request().Context(), `INSERT INTO restaurant_payouts(restaurant_id,method,amount_paise,status) VALUES($1,'BANK_TRANSFER',$2,'PENDING') RETURNING id,uuid`, owner, amount).Scan(&payout, &uid)
		} else {
			err = tx.QueryRow(c.Request().Context(), `INSERT INTO delivery_agent_payouts(delivery_agent_id,bank_details_id,method,amount_paise,status) VALUES($1,$2,'BANK_TRANSFER',$3,'PENDING') RETURNING id,uuid`, owner, bank, amount).Scan(&payout, &uid)
		}
		if err != nil {
			return err
		}
		for _, a := range allocations {
			if _, err = tx.Exec(c.Request().Context(), "INSERT INTO "+cfg.Allocation+"("+cfg.PayoutColumn+","+cfg.EarningColumn+",amount_paise) VALUES($1,$2,$3)", payout, a.ID, a.Amount); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(c.Request().Context(), `INSERT INTO system_activity_logs(actor_type,actor_id,action,entity_type,entity_id,entity_uuid,after_json,occurred_at) VALUES('ADMIN',$1,'PAYOUT',$2,$3,$4,jsonb_build_object('bank_uuid',$5::text,'reason',$6::text,'bank_transfer_confirmed',false),now())`, person(c).ID, cfg.Payout, payout, uid, input.Bank, input.Reason); err != nil {
			return err
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return err
		}
		return c.JSON(201, map[string]any{"id": uid, "status": "PENDING", "amount_paise": amount})
	})
	admin.POST("/payouts/:kind/:id/confirm", func(c echo.Context) error {
		cfg, ok := payoutTables(c.Param("kind"))
		if !ok || publicID(c.Param("id")) != nil {
			return bad("payout UUID required")
		}
		var input struct {
			Reference string `json:"provider_transfer_id"`
			Status    string `json:"status"`
			Reason    string `json:"reason"`
		}
		if c.Bind(&input) != nil || !validName(input.Reference, 120) || len(input.Reference) < 8 || !validName(input.Reason, 2000) || (input.Status != "PAID" && input.Status != "FAILED") {
			return bad("confirmed bank result, transfer reference and audit reason required")
		}
		tx, err := s.begin(c, "PAYOUT")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), `SELECT pg_advisory_xact_lock(729403),set_config('yember.reason',$1,true)`, input.Reason); err != nil {
			return err
		}
		var payout, amount int64
		var status string
		var previous *string
		if err = tx.QueryRow(c.Request().Context(), "SELECT id,amount_paise,status,provider_transfer_id FROM "+cfg.Payout+" WHERE uuid=$1 AND method='BANK_TRANSFER' AND deleted_at IS NULL FOR UPDATE", c.Param("id")).Scan(&payout, &amount, &status, &previous); err != nil {
			return unavailable(err)
		}
		if status == "PAID" {
			if input.Status == "PAID" && previous != nil && *previous == input.Reference {
				return c.NoContent(204)
			}
			return bad("a paid payout requires a reversal workflow")
		}
		var creator int64
		if err = tx.QueryRow(c.Request().Context(), `SELECT actor_id FROM system_activity_logs WHERE entity_type=$1 AND entity_id=$2 AND after_json ? 'bank_uuid' ORDER BY id LIMIT 1`, cfg.Payout, payout).Scan(&creator); err != nil {
			return bad("original payout instruction is required")
		}
		if creator == person(c).ID {
			return bad("a second administrator must verify the bank result")
		}
		var duplicate bool
		if err = tx.QueryRow(c.Request().Context(), `SELECT EXISTS(SELECT 1 FROM restaurant_payouts WHERE provider_transfer_id=$1 UNION ALL SELECT 1 FROM delivery_agent_payouts WHERE provider_transfer_id=$1)`, input.Reference).Scan(&duplicate); err != nil {
			return err
		}
		if duplicate {
			return bad("bank transfer reference is already recorded")
		}
		if status != "PENDING" && status != "PROCESSING" {
			return bad("payout no longer awaits bank confirmation")
		}
		if input.Status == "PAID" {
			rows, err := tx.Query(c.Request().Context(), "SELECT "+cfg.EarningColumn+",amount_paise FROM "+cfg.Allocation+" WHERE "+cfg.PayoutColumn+"=$1 ORDER BY id", payout)
			if err != nil {
				return err
			}
			type allocation struct{ ID, Amount int64 }
			allocations := []allocation{}
			var sum int64
			for rows.Next() {
				var a allocation
				if err = rows.Scan(&a.ID, &a.Amount); err != nil {
					rows.Close()
					return err
				}
				allocations = append(allocations, a)
				sum += a.Amount
			}
			rows.Close()
			if err = rows.Err(); err != nil {
				return err
			}
			if sum != amount {
				return bad("allocation total differs from the payout")
			}
			for _, a := range allocations {
				ref := links{}
				account := "RESTAURANT_PAYABLE"
				if cfg.Payout == "restaurant_payouts" {
					ref.RestaurantPayout = ptr(payout)
					ref.RestaurantEarning = ptr(a.ID)
				} else {
					ref.AgentPayout = ptr(payout)
					ref.AgentEarning = ptr(a.ID)
					account = "AGENT_PAYABLE"
				}
				if err = post(c, tx, fmt.Sprintf("bank/%s/%d/%d", c.Param("kind"), payout, a.ID), "PAYOUT", ref, posting{account, a.Amount}, posting{"YEMBER_BANK", -a.Amount}); err != nil {
					return err
				}
			}
		}
		if _, err = tx.Exec(c.Request().Context(), "UPDATE "+cfg.Payout+" SET status=$2::text,provider_transfer_id=$3,paid_at=CASE WHEN $2::text='PAID' THEN now() ELSE NULL END WHERE id=$1", payout, input.Status, input.Reference); err != nil {
			return err
		}
		if input.Status == "PAID" {
			if _, err = tx.Exec(c.Request().Context(), "UPDATE "+cfg.Earning+" e SET status='SETTLED' WHERE e.id IN (SELECT "+cfg.EarningColumn+" FROM "+cfg.Allocation+" WHERE "+cfg.PayoutColumn+"=$1) AND e.net_amount_paise<=(SELECT sum(a.amount_paise) FROM "+cfg.Allocation+" a JOIN "+cfg.Payout+" p ON p.id=a."+cfg.PayoutColumn+" WHERE a."+cfg.EarningColumn+"=e.id AND p.status='PAID')", payout); err != nil {
				return err
			}
		}
		return commit(c, tx)
	})
}

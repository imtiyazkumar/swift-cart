package finance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"strings"
)

type CheckoutSession struct {
	ID            string `json:"id"`
	URL           string `json:"url"`
	Expires       int64  `json:"expires_at"`
	Amount        int64  `json:"amount_total"`
	Currency      string `json:"currency"`
	Status        string `json:"status"`
	PaymentStatus string `json:"payment_status"`
	Reference     string `json:"client_reference_id"`
	PaymentIntent string `json:"payment_intent"`
}

func (s *Service) applyConfirmedEvent(ctx context.Context, raw []byte) error {
	var event struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &event) != nil || event.ID == "" {
		return errors.New("invalid provider event")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	digest := sha256.Sum256(raw)
	tag, err := tx.Exec(ctx, `INSERT INTO finance.provider_events(provider,event_id,digest) VALUES('RAZORPAY',$1,$2) ON CONFLICT DO NOTHING`, event.ID, hex.EncodeToString(digest[:]))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var previous string
		if err = tx.QueryRow(ctx, `SELECT digest FROM finance.provider_events WHERE provider='RAZORPAY' AND event_id=$1`, event.ID).Scan(&previous); err != nil {
			return err
		}
		if previous != hex.EncodeToString(digest[:]) {
			return errors.New("provider event identity reused with a different payload")
		}
		return nil
	}
	switch event.Type {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		var session CheckoutSession
		if json.Unmarshal(event.Data.Object, &session) != nil {
			return errors.New("invalid checkout event")
		}
		var order, attempt, currency string
		err = tx.QueryRow(ctx, `SELECT order_id,id,currency FROM finance.payment_attempts WHERE provider='RAZORPAY' AND provider_id=$1`, session.ID).Scan(&order, &attempt, &currency)
		if err == pgx.ErrNoRows {
			// Ignore onboarding sessions; order sessions with an unknown reference must be retried/reconciled.
			var metadata struct {
				Metadata map[string]string `json:"metadata"`
			}
			_ = json.Unmarshal(event.Data.Object, &metadata)
			if metadata.Metadata["yember_finance"] == "order" {
				return errors.New("order payment session not yet persisted")
			}
			return nil
		}
		if err != nil {
			return err
		}
		if session.PaymentStatus != "paid" {
			return tx.Commit(ctx)
		}
		if session.Reference != order || !strings.EqualFold(session.Currency, currency) || session.PaymentIntent == "" {
			return errors.New("payment reference/currency mismatch")
		}
		// Serialize delivery, capture and refunds on the same order before any financial mutation.
		if _, err = tx.Exec(ctx, `SELECT id FROM public.orders WHERE id=$1 FOR UPDATE`, order); err != nil {
			return err
		}
		if err = capture(ctx, tx, order, attempt, session.PaymentIntent, session.Amount); err != nil {
			return err
		}
	case "refund.updated", "refund.created":
		var refund struct {
			ID            string `json:"id"`
			PaymentIntent string `json:"payment_intent"`
			Amount        int64  `json:"amount"`
			Status        string `json:"status"`
			Currency      string `json:"currency"`
		}
		if json.Unmarshal(event.Data.Object, &refund) != nil {
			return errors.New("invalid refund event")
		}
		if refund.Status != "succeeded" {
			return tx.Commit(ctx)
		}
		var order string
		if err = tx.QueryRow(ctx, `SELECT order_id FROM finance.payment_attempts WHERE payment_reference=$1 AND status='PAID'`, refund.PaymentIntent).Scan(&order); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `SELECT id FROM public.orders WHERE id=$1 FOR UPDATE`, order); err != nil {
			return err
		}
		var recognized bool
		var total, prior int64
		var currency string
		if err = tx.QueryRow(ctx, `SELECT total,currency,EXISTS(SELECT 1 FROM finance.journals WHERE order_id=$1 AND kind='EARN'),COALESCE((SELECT sum(e.amount) FROM finance.entries e JOIN finance.journals j ON j.id=e.journal_id WHERE j.order_id=$1 AND j.kind='REFUND' AND e.account='REFUND_EXPENSE'),0) FROM finance.order_financials WHERE order_id=$1`, order).Scan(&total, &currency, &recognized, &prior); err != nil {
			return err
		}
		if refund.Amount <= 0 || refund.Amount > total || !strings.EqualFold(currency, refund.Currency) {
			return errors.New("invalid refund amount/currency")
		}
		var already bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM finance.journals WHERE event_key=$1)`, "refund/"+refund.ID).Scan(&already); err != nil {
			return err
		}
		if !already && refund.Amount > total-prior {
			return errors.New("refund exceeds remaining captured amount")
		}
		// Provider refund IDs are immutable, unique journal keys.
		if err = post(ctx, tx, "refund/"+refund.ID, "REFUND", order, refund.ID, "Provider-confirmed refund", "", Line{"REFUND_EXPENSE", "", "refund", refund.Amount}, Line{"CASH", "", "refund", -refund.Amount}); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE finance.refund_requests SET status='SUCCEEDED',provider_id=$2 WHERE order_id=$1 AND provider_id=$2`, order, refund.ID); err != nil {
			return err
		}
		if recognized {
			_, err = tx.Exec(ctx, `INSERT INTO finance.discrepancies(order_id,event_key,kind,details) VALUES($1,$2,'REFUND_REQUIRES_ALLOCATION',jsonb_build_object('amount',$3::bigint,'reference',$4::text)) ON CONFLICT DO NOTHING`, order, "refund/"+refund.ID, refund.Amount, refund.ID)
		} else {
			if err = post(ctx, tx, "refund-release/"+refund.ID, "REFUND_RELEASE", order, refund.ID, "Unrecognized order receipt refunded", "", Line{"UNEARNED", "", "refund_release", refund.Amount}, Line{"REFUND_EXPENSE", "", "refund_release", -refund.Amount}); err != nil {
				return err
			}
		}
		if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(e.amount),0) FROM finance.entries e JOIN finance.journals j ON j.id=e.journal_id WHERE j.order_id=$1 AND j.kind='REFUND' AND e.account='REFUND_EXPENSE'`, order).Scan(&prior); err != nil {
			return err
		}
		if prior >= total {
			_, err = tx.Exec(ctx, `UPDATE public.orders SET status='REFUNDED',updated_at=now() WHERE id=$1`, order)
		}
	case "charge.dispute.created", "charge.dispute.closed":
		var dispute struct {
			ID            string `json:"id"`
			PaymentIntent string `json:"payment_intent"`
			Amount        int64  `json:"amount"`
			Status        string `json:"status"`
		}
		if json.Unmarshal(event.Data.Object, &dispute) != nil {
			return errors.New("invalid dispute")
		}
		_, err = tx.Exec(ctx, `INSERT INTO finance.discrepancies(order_id,event_key,kind,details) SELECT order_id,$2,'CHARGEBACK',jsonb_build_object('reference',$3::text,'amount',$4::bigint,'status',$5::text) FROM finance.payment_attempts WHERE payment_reference=$1 ON CONFLICT DO NOTHING`, dispute.PaymentIntent, event.ID, dispute.ID, dispute.Amount, dispute.Status)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

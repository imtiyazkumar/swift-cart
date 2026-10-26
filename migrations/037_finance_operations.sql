ALTER TABLE finance.bank_accounts ADD COLUMN provider_fund_account text;
ALTER TABLE finance.refund_requests ADD COLUMN approved_by uuid REFERENCES public.users(id);
ALTER TABLE public.users ADD COLUMN tokens_invalid_before timestamptz NOT NULL DEFAULT '-infinity';
CREATE FUNCTION finance.protect_payout() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.amount<>OLD.amount OR NEW.user_id<>OLD.user_id OR NEW.bank_account_id<>OLD.bank_account_id OR NEW.batch_id<>OLD.batch_id OR NEW.currency<>OLD.currency THEN RAISE EXCEPTION 'payout allocation is immutable'; END IF;
 IF OLD.status='SUCCEEDED' AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'confirmed payouts require a reversal, not an edit'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER payout_allocation_guard BEFORE UPDATE ON finance.payouts FOR EACH ROW EXECUTE FUNCTION finance.protect_payout();
CREATE TRIGGER payout_delete_guard BEFORE DELETE ON finance.payouts FOR EACH ROW EXECUTE FUNCTION finance.immutable();
CREATE TABLE finance.refund_policy (
 id boolean PRIMARY KEY DEFAULT true CHECK(id), version integer NOT NULL DEFAULT 1,
 unfulfilled_full_refund boolean NOT NULL DEFAULT true,
 delivered_requires_review boolean NOT NULL DEFAULT true,
 auto_clawback boolean NOT NULL DEFAULT false CHECK(NOT auto_clawback),
 description text NOT NULL DEFAULT 'Cancelled or rejected unfulfilled orders: full collected amount refundable. Delivered orders: finance-reviewed full or partial refund. Gateway fees borne by platform. No automatic partner clawback.'
);
INSERT INTO finance.refund_policy(id) VALUES(true);

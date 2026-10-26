-- Domain: payments
-- Applied through cmd/migrate using manifest.json; do not execute this file directly.
-- yember-step: 006_domain_schemas.sql/006

CREATE SCHEMA IF NOT EXISTS payments;
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/029


CREATE TABLE IF NOT EXISTS payments.payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES public.orders(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    method TEXT NOT NULL,
    amount BIGINT NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL DEFAULT 'INR',
    status TEXT NOT NULL,
    idempotency_key TEXT,
    provider_reference TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(provider, idempotency_key)
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/030


CREATE TABLE IF NOT EXISTS payments.refunds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id UUID NOT NULL REFERENCES payments.payments(id) ON DELETE CASCADE,
    amount BIGINT NOT NULL CHECK (amount > 0),
    status TEXT NOT NULL DEFAULT 'INITIATED',
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/031


CREATE TABLE IF NOT EXISTS payments.transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider TEXT NOT NULL,
    provider_event_id TEXT NOT NULL,
    status TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(provider, provider_event_id)
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/032


CREATE TABLE IF NOT EXISTS payments.settlements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID NOT NULL REFERENCES public.merchants(id),
    amount BIGINT NOT NULL CHECK (amount >= 0),
    status TEXT NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 026_onboarding_payments.sql/001
CREATE TABLE IF NOT EXISTS payments.onboarding_payments (
    merchant_id UUID PRIMARY KEY REFERENCES public.merchants(id),
    session_id TEXT NOT NULL UNIQUE,
    checkout_url TEXT NOT NULL,
    amount BIGINT NOT NULL CHECK (amount>0),
    currency CHAR(3) NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PAID')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at TIMESTAMPTZ
);
-- yember-end-step
-- yember-step: 026_onboarding_payments.sql/002

CREATE TABLE IF NOT EXISTS payments.onboarding_events (
    event_id TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- yember-end-step
-- yember-step: 036_central_finance.sql/001
CREATE SCHEMA finance;
-- yember-end-step
-- yember-step: 036_central_finance.sql/003

CREATE TABLE finance.permissions (
 user_id uuid REFERENCES public.users(id), permission text CHECK(permission IN ('READ','CONFIGURE','PREPARE','APPROVE','PAYOUT','RECONCILE','ADJUST')), PRIMARY KEY(user_id,permission)
);
-- yember-end-step
-- yember-step: 036_central_finance.sql/004

CREATE TABLE finance.policy (
 id boolean PRIMARY KEY DEFAULT true CHECK(id), interval_days integer NOT NULL DEFAULT 15 CHECK(interval_days BETWEEN 1 AND 90),
 anchor timestamptz NOT NULL DEFAULT '2026-01-01 00:00:00+00', delivery_charge bigint NOT NULL DEFAULT 0 CHECK(delivery_charge>=0),
 rider_base bigint NOT NULL DEFAULT 0 CHECK(rider_base>=0), enabled boolean NOT NULL DEFAULT false,
 updated_at timestamptz NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 036_central_finance.sql/005

INSERT INTO finance.policy(id) VALUES(true);
-- yember-end-step
-- yember-step: 036_central_finance.sql/006

CREATE TABLE finance.bank_accounts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES public.users(id),
 ciphertext bytea NOT NULL, key_id text NOT NULL, last_four text NOT NULL CHECK(length(last_four)=4),
 status text NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','VERIFIED','REJECTED','REPLACED')),
 verified_by uuid REFERENCES public.users(id), verification_reference text, created_at timestamptz NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 036_central_finance.sql/007

CREATE UNIQUE INDEX bank_current ON finance.bank_accounts(user_id) WHERE status IN ('PENDING','VERIFIED');
-- yember-end-step
-- yember-step: 036_central_finance.sql/008

CREATE TABLE finance.order_financials (
 order_id uuid PRIMARY KEY REFERENCES public.orders(id), restaurant_user uuid NOT NULL REFERENCES public.users(id),
 currency char(3) NOT NULL DEFAULT 'INR', gross bigint NOT NULL CHECK(gross>0), discount bigint NOT NULL DEFAULT 0 CHECK(discount>=0),
 commission_bps integer NOT NULL CHECK(commission_bps BETWEEN 0 AND 10000), commission bigint NOT NULL CHECK(commission>=0),
 delivery_charge bigint NOT NULL DEFAULT 0 CHECK(delivery_charge>=0), rider_base bigint NOT NULL DEFAULT 0 CHECK(rider_base>=0),
 tax bigint NOT NULL DEFAULT 0 CHECK(tax>=0), platform_charge bigint NOT NULL DEFAULT 0 CHECK(platform_charge>=0),
 total bigint NOT NULL CHECK(total>0), created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(discount<=gross), CHECK(commission<=gross), CHECK(total=gross-discount+delivery_charge+tax+platform_charge)
);
-- yember-end-step
-- yember-step: 036_central_finance.sql/009

CREATE TABLE finance.payment_attempts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), order_id uuid NOT NULL REFERENCES finance.order_financials(order_id),
 attempt integer NOT NULL DEFAULT 1, provider text NOT NULL CHECK(provider IN ('RAZORPAY','COD')),
 amount bigint NOT NULL CHECK(amount>0), currency char(3) NOT NULL, provider_id text UNIQUE, payment_reference text UNIQUE,
 checkout_url text, expires_at timestamptz, status text NOT NULL DEFAULT 'CREATING' CHECK(status IN ('CREATING','PENDING','PAID','EXPIRED','FAILED')),
 request_started_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(order_id,attempt)
);
-- yember-end-step
-- yember-step: 036_central_finance.sql/010

CREATE UNIQUE INDEX one_captured_order ON finance.payment_attempts(order_id) WHERE status='PAID';
-- yember-end-step
-- yember-step: 036_central_finance.sql/011

CREATE TABLE finance.journals (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), event_key text NOT NULL UNIQUE, kind text NOT NULL,
 order_id uuid REFERENCES finance.order_financials(order_id), reference text NOT NULL, reason text NOT NULL,
 actor uuid REFERENCES public.users(id), currency char(3) NOT NULL DEFAULT 'INR', created_at timestamptz NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 036_central_finance.sql/012

CREATE TABLE finance.entries (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), journal_id uuid NOT NULL REFERENCES finance.journals(id),
 account text NOT NULL CHECK(account IN ('CASH','UNEARNED','PARTNER_PAYABLE','COMMISSION','DELIVERY_REVENUE','TAX_PAYABLE','GATEWAY_FEE','REFUND_EXPENSE','ADJUSTMENT','PAYOUT_CLEARING')),
 user_id uuid REFERENCES public.users(id), component text NOT NULL, amount bigint NOT NULL CHECK(amount<>0),
 created_at timestamptz NOT NULL DEFAULT now(), CHECK((account='PARTNER_PAYABLE')=(user_id IS NOT NULL))
);
-- yember-end-step
-- yember-step: 036_central_finance.sql/013

CREATE INDEX entries_partner ON finance.entries(user_id,created_at,id) WHERE user_id IS NOT NULL;
-- yember-end-step
-- yember-step: 036_central_finance.sql/014

CREATE INDEX journals_order ON finance.journals(order_id,created_at);
-- yember-end-step
-- yember-step: 036_central_finance.sql/015

CREATE FUNCTION finance.immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'financial history is immutable'; END $$;
-- yember-end-step
-- yember-step: 036_central_finance.sql/016

CREATE TRIGGER immutable_financials BEFORE UPDATE OR DELETE ON finance.order_financials FOR EACH ROW EXECUTE FUNCTION finance.immutable();
-- yember-end-step
-- yember-step: 036_central_finance.sql/017

CREATE TRIGGER immutable_journals BEFORE UPDATE OR DELETE ON finance.journals FOR EACH ROW EXECUTE FUNCTION finance.immutable();
-- yember-end-step
-- yember-step: 036_central_finance.sql/018

CREATE TRIGGER immutable_entries BEFORE UPDATE OR DELETE ON finance.entries FOR EACH ROW EXECUTE FUNCTION finance.immutable();
-- yember-end-step
-- yember-step: 036_central_finance.sql/019

CREATE FUNCTION finance.balanced() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (SELECT count(*)<2 OR COALESCE(sum(amount),0)<>0 FROM finance.entries WHERE journal_id=NEW.id) THEN RAISE EXCEPTION 'unbalanced financial journal'; END IF;
 RETURN NULL;
END $$;
-- yember-end-step
-- yember-step: 036_central_finance.sql/020

CREATE CONSTRAINT TRIGGER balanced_journal AFTER INSERT ON finance.journals DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION finance.balanced();
-- yember-end-step
-- yember-step: 036_central_finance.sql/021

CREATE FUNCTION finance.entry_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM finance.journals WHERE id=NEW.journal_id AND xmin::text<>pg_current_xact_id()::text) THEN RAISE EXCEPTION 'cannot append to committed journal'; END IF;
 RETURN NEW;
END $$;
-- yember-end-step
-- yember-step: 036_central_finance.sql/022

CREATE TRIGGER entry_guard BEFORE INSERT ON finance.entries FOR EACH ROW EXECUTE FUNCTION finance.entry_guard();
-- yember-end-step
-- yember-step: 036_central_finance.sql/023

CREATE TABLE finance.batches (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), cutoff timestamptz NOT NULL UNIQUE,
 prepared_by uuid REFERENCES public.users(id), approved_by uuid REFERENCES public.users(id),
 status text NOT NULL DEFAULT 'REVIEW' CHECK(status IN ('REVIEW','APPROVED')),
 created_at timestamptz NOT NULL DEFAULT now(), approved_at timestamptz,
 CHECK(prepared_by IS NULL OR approved_by IS NULL OR prepared_by<>approved_by)
);
-- yember-end-step
-- yember-step: 036_central_finance.sql/024

CREATE TABLE finance.payouts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), batch_id uuid NOT NULL REFERENCES finance.batches(id),
 user_id uuid NOT NULL REFERENCES public.users(id), bank_account_id uuid NOT NULL REFERENCES finance.bank_accounts(id),
 amount bigint NOT NULL CHECK(amount>0), currency char(3) NOT NULL DEFAULT 'INR',
 status text NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','PROCESSING','SUCCEEDED','FAILED','UNKNOWN','REVERSED')),
 provider text NOT NULL DEFAULT 'BANK_RECONCILIATION', provider_id text UNIQUE, initiated_by uuid REFERENCES public.users(id),
 confirmed_by uuid REFERENCES public.users(id), evidence text, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(batch_id,user_id)
);
-- yember-end-step
-- yember-step: 036_central_finance.sql/025

CREATE TABLE finance.settlement_entries (
 entry_id uuid PRIMARY KEY REFERENCES finance.entries(id), payout_id uuid NOT NULL REFERENCES finance.payouts(id)
);
-- yember-end-step
-- yember-step: 036_central_finance.sql/026

CREATE TABLE finance.payout_history (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), payout_id uuid NOT NULL REFERENCES finance.payouts(id),
 status text NOT NULL, reference text NOT NULL, actor uuid REFERENCES public.users(id), created_at timestamptz NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 036_central_finance.sql/027

CREATE TRIGGER immutable_payout_history BEFORE UPDATE OR DELETE ON finance.payout_history FOR EACH ROW EXECUTE FUNCTION finance.immutable();
-- yember-end-step
-- yember-step: 036_central_finance.sql/028

CREATE TRIGGER immutable_settlement_entries BEFORE UPDATE OR DELETE ON finance.settlement_entries FOR EACH ROW EXECUTE FUNCTION finance.immutable();
-- yember-end-step
-- yember-step: 036_central_finance.sql/029

CREATE TABLE finance.provider_events (
 provider text NOT NULL, event_id text NOT NULL, digest text NOT NULL, received_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(provider,event_id)
);
-- yember-end-step
-- yember-step: 036_central_finance.sql/030

CREATE TABLE finance.refund_requests (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), order_id uuid NOT NULL REFERENCES finance.order_financials(order_id),
 request_key text NOT NULL UNIQUE, amount bigint NOT NULL CHECK(amount>0), reason text NOT NULL,
 requested_by uuid NOT NULL REFERENCES public.users(id), provider_id text UNIQUE, status text NOT NULL DEFAULT 'REQUESTED',
 created_at timestamptz NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 036_central_finance.sql/031

CREATE TABLE finance.discrepancies (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), order_id uuid REFERENCES finance.order_financials(order_id),
 event_key text NOT NULL UNIQUE, kind text NOT NULL, details jsonb NOT NULL, resolved_by uuid REFERENCES public.users(id),
 resolution text, created_at timestamptz NOT NULL DEFAULT now(), resolved_at timestamptz
);
-- yember-end-step
-- yember-step: 036_central_finance.sql/032

CREATE TABLE finance.reconciliations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), actor uuid REFERENCES public.users(id), reference text NOT NULL UNIQUE,
 details jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 036_central_finance.sql/033

CREATE TRIGGER immutable_reconciliations BEFORE UPDATE OR DELETE ON finance.reconciliations FOR EACH ROW EXECUTE FUNCTION finance.immutable();

-- yember-end-step
-- yember-step: 037_finance_operations.sql/001
ALTER TABLE finance.bank_accounts ADD COLUMN provider_fund_account text;
-- yember-end-step
-- yember-step: 037_finance_operations.sql/002

ALTER TABLE finance.refund_requests ADD COLUMN approved_by uuid REFERENCES public.users(id);
-- yember-end-step
-- yember-step: 037_finance_operations.sql/004

CREATE FUNCTION finance.protect_payout() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.amount<>OLD.amount OR NEW.user_id<>OLD.user_id OR NEW.bank_account_id<>OLD.bank_account_id OR NEW.batch_id<>OLD.batch_id OR NEW.currency<>OLD.currency THEN RAISE EXCEPTION 'payout allocation is immutable'; END IF;
 IF OLD.status='SUCCEEDED' AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'confirmed payouts require a reversal, not an edit'; END IF;
 RETURN NEW;
END $$;
-- yember-end-step
-- yember-step: 037_finance_operations.sql/005

CREATE TRIGGER payout_allocation_guard BEFORE UPDATE ON finance.payouts FOR EACH ROW EXECUTE FUNCTION finance.protect_payout();
-- yember-end-step
-- yember-step: 037_finance_operations.sql/006

CREATE TRIGGER payout_delete_guard BEFORE DELETE ON finance.payouts FOR EACH ROW EXECUTE FUNCTION finance.immutable();
-- yember-end-step
-- yember-step: 037_finance_operations.sql/007

CREATE TABLE finance.refund_policy (
 id boolean PRIMARY KEY DEFAULT true CHECK(id), version integer NOT NULL DEFAULT 1,
 unfulfilled_full_refund boolean NOT NULL DEFAULT true,
 delivered_requires_review boolean NOT NULL DEFAULT true,
 auto_clawback boolean NOT NULL DEFAULT false CHECK(NOT auto_clawback),
 description text NOT NULL DEFAULT 'Cancelled or rejected unfulfilled orders: full collected amount refundable. Delivered orders: finance-reviewed full or partial refund. Gateway fees borne by platform. No automatic partner clawback.'
);
-- yember-end-step
-- yember-step: 037_finance_operations.sql/008

INSERT INTO finance.refund_policy(id) VALUES(true);

-- yember-end-step
-- yember-step: 040_finance_refund_jobs.sql/001
CREATE TABLE finance.refund_jobs(order_id uuid PRIMARY KEY REFERENCES finance.order_financials(order_id),status text NOT NULL DEFAULT 'PENDING',last_error text,created_at timestamptz NOT NULL DEFAULT now());
-- yember-end-step
-- yember-step: 040_finance_refund_jobs.sql/002

CREATE FUNCTION finance.queue_cancellation_refund() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status::text IN ('CANCELLED','REJECTED') AND NEW.status IS DISTINCT FROM OLD.status THEN
 INSERT INTO finance.refund_jobs(order_id) SELECT NEW.id FROM finance.payment_attempts WHERE order_id=NEW.id AND status='PAID' AND provider='RAZORPAY' ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;
-- yember-end-step
-- yember-step: 041_launch_delivery_pricing.sql/001
UPDATE finance.policy SET delivery_charge=4000,rider_base=4000,updated_at=now();

-- yember-end-step
-- yember-step: 045_financial_integrity.sql/001
CREATE FUNCTION finance.protect_refund() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.order_id<>OLD.order_id OR NEW.request_key<>OLD.request_key OR NEW.amount<>OLD.amount OR NEW.reason<>OLD.reason OR NEW.requested_by<>OLD.requested_by THEN RAISE EXCEPTION 'refund request financial terms are immutable'; END IF;
 RETURN NEW;
END $$;
-- yember-end-step
-- yember-step: 045_financial_integrity.sql/002

CREATE TRIGGER refund_terms_guard BEFORE UPDATE ON finance.refund_requests FOR EACH ROW EXECUTE FUNCTION finance.protect_refund();
-- yember-end-step
-- yember-step: 045_financial_integrity.sql/003

CREATE TRIGGER refund_delete_guard BEFORE DELETE ON finance.refund_requests FOR EACH ROW EXECUTE FUNCTION finance.immutable();
-- yember-end-step
-- yember-step: 045_financial_integrity.sql/004

CREATE TRIGGER provider_event_history_guard BEFORE UPDATE OR DELETE ON finance.provider_events FOR EACH ROW EXECUTE FUNCTION finance.immutable();
-- yember-end-step
-- yember-step: 045_financial_integrity.sql/005

CREATE FUNCTION finance.protect_entry_currency() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.currency<>'INR' THEN RAISE EXCEPTION 'unsupported ledger currency'; END IF;
 RETURN NEW;
END $$;
-- yember-end-step
-- yember-step: 045_financial_integrity.sql/006

CREATE TRIGGER currency_guard BEFORE INSERT ON finance.journals FOR EACH ROW EXECUTE FUNCTION finance.protect_entry_currency();

-- yember-end-step
-- yember-step: 046_central_promotions.sql/001
ALTER TABLE finance.entries DROP CONSTRAINT entries_account_check;
-- yember-end-step
-- yember-step: 046_central_promotions.sql/002

ALTER TABLE finance.entries ADD CONSTRAINT entries_account_check CHECK(account IN ('CASH','UNEARNED','PARTNER_PAYABLE','COMMISSION','DELIVERY_REVENUE','TAX_PAYABLE','GATEWAY_FEE','REFUND_EXPENSE','ADJUSTMENT','PAYOUT_CLEARING','PROMOTION_EXPENSE'));
-- yember-end-step
-- yember-step: 046_central_promotions.sql/003

CREATE TABLE finance.coupon_redemptions(order_id uuid PRIMARY KEY REFERENCES finance.order_financials(order_id),coupon_id uuid NOT NULL REFERENCES orders.coupons(id),amount bigint NOT NULL CHECK(amount>0),funded_by text NOT NULL DEFAULT 'PLATFORM' CHECK(funded_by='PLATFORM'),created_at timestamptz NOT NULL DEFAULT now());
-- yember-end-step
-- yember-step: 046_central_promotions.sql/004

CREATE TRIGGER coupon_history_guard BEFORE UPDATE OR DELETE ON finance.coupon_redemptions FOR EACH ROW EXECUTE FUNCTION finance.immutable();

-- yember-end-step
-- yember-step: 047_settlement_recovery.sql/001
ALTER TABLE finance.payouts DROP CONSTRAINT payouts_status_check;
-- yember-end-step
-- yember-step: 047_settlement_recovery.sql/002

ALTER TABLE finance.payouts ADD CONSTRAINT payouts_status_check CHECK(status IN ('PENDING','PROCESSING','SUCCEEDED','FAILED','UNKNOWN','REVERSED','CANCELLED'));
-- yember-end-step
-- yember-step: 047_settlement_recovery.sql/003

-- Multiple historical reservations are retained through release/re-reservation events.
ALTER TABLE finance.settlement_entries DROP CONSTRAINT settlement_entries_pkey;
-- yember-end-step
-- yember-step: 047_settlement_recovery.sql/004

ALTER TABLE finance.settlement_entries ADD PRIMARY KEY(entry_id,payout_id);
-- yember-end-step
-- yember-step: 047_settlement_recovery.sql/005

CREATE TABLE finance.settlement_releases(entry_id uuid NOT NULL,payout_id uuid NOT NULL,actor uuid NOT NULL REFERENCES public.users(id),reason text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(entry_id,payout_id),FOREIGN KEY(entry_id,payout_id) REFERENCES finance.settlement_entries(entry_id,payout_id));
-- yember-end-step
-- yember-step: 047_settlement_recovery.sql/006

CREATE TRIGGER release_history_guard BEFORE UPDATE OR DELETE ON finance.settlement_releases FOR EACH ROW EXECUTE FUNCTION finance.immutable();
-- yember-end-step
-- yember-step: 047_settlement_recovery.sql/007

CREATE FUNCTION finance.reserve_once() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM pg_advisory_xact_lock(894213);
 IF EXISTS(SELECT 1 FROM finance.settlement_entries se WHERE se.entry_id=NEW.entry_id AND NOT EXISTS(SELECT 1 FROM finance.settlement_releases r WHERE r.entry_id=se.entry_id AND r.payout_id=se.payout_id)) THEN RAISE EXCEPTION 'entry already reserved'; END IF;
 RETURN NEW;
END $$;
-- yember-end-step
-- yember-step: 047_settlement_recovery.sql/008

CREATE TRIGGER reservation_guard BEFORE INSERT ON finance.settlement_entries FOR EACH ROW EXECUTE FUNCTION finance.reserve_once();

-- yember-end-step
-- yember-step: 048_payout_attempts.sql/001
ALTER TABLE finance.payouts ADD COLUMN attempt_no integer NOT NULL DEFAULT 0 CHECK(attempt_no>=0);
-- yember-end-step
-- yember-step: 048_payout_attempts.sql/002

CREATE TABLE finance.payout_attempts(payout_id uuid NOT NULL REFERENCES finance.payouts(id),attempt_no integer NOT NULL,idempotency_key uuid NOT NULL UNIQUE,provider_id text UNIQUE,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(payout_id,attempt_no));
-- yember-end-step
-- yember-step: 048_payout_attempts.sql/003

CREATE FUNCTION finance.protect_payout_attempt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.payout_id<>OLD.payout_id OR NEW.attempt_no<>OLD.attempt_no OR NEW.idempotency_key<>OLD.idempotency_key OR (OLD.provider_id IS NOT NULL AND NEW.provider_id IS DISTINCT FROM OLD.provider_id) THEN RAISE EXCEPTION 'payout attempt is immutable'; END IF;
 RETURN NEW;
END $$;
-- yember-end-step
-- yember-step: 048_payout_attempts.sql/004

CREATE TRIGGER payout_attempt_guard BEFORE UPDATE ON finance.payout_attempts FOR EACH ROW EXECUTE FUNCTION finance.protect_payout_attempt();
-- yember-end-step
-- yember-step: 048_payout_attempts.sql/005

CREATE TRIGGER payout_attempt_delete_guard BEFORE DELETE ON finance.payout_attempts FOR EACH ROW EXECUTE FUNCTION finance.immutable();

-- yember-end-step
-- yember-step: 049_delivery_evidence.sql/003

CREATE TABLE finance.cod_custody(order_id uuid PRIMARY KEY REFERENCES finance.order_financials(order_id),partner_id uuid NOT NULL REFERENCES delivery.delivery_partners(id),amount bigint NOT NULL CHECK(amount>0),created_at timestamptz NOT NULL DEFAULT now());
-- yember-end-step
-- yember-step: 049_delivery_evidence.sql/004

CREATE TRIGGER cod_custody_guard BEFORE UPDATE OR DELETE ON finance.cod_custody FOR EACH ROW EXECUTE FUNCTION finance.immutable();
-- yember-end-step
-- yember-step: 052_razorpay_onboarding.sql/001
CREATE TABLE finance.onboarding_attempts(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),merchant_id uuid NOT NULL REFERENCES public.merchants(id),user_id uuid NOT NULL REFERENCES public.users(id),amount bigint NOT NULL CHECK(amount>0),currency char(3) NOT NULL,status text NOT NULL DEFAULT 'CREATING' CHECK(status IN ('CREATING','PENDING','PAID','EXPIRED')),provider_id text UNIQUE,payment_reference text UNIQUE,checkout_url text,expires_at timestamptz,created_at timestamptz NOT NULL DEFAULT now());
-- yember-end-step
-- yember-step: 052_razorpay_onboarding.sql/002

CREATE UNIQUE INDEX one_onboarding_paid ON finance.onboarding_attempts(merchant_id) WHERE status='PAID';
-- yember-end-step
-- yember-step: 052_razorpay_onboarding.sql/003

ALTER TABLE finance.entries DROP CONSTRAINT entries_account_check;
-- yember-end-step
-- yember-step: 052_razorpay_onboarding.sql/004

ALTER TABLE finance.entries ADD CONSTRAINT entries_account_check CHECK(account IN ('CASH','UNEARNED','PARTNER_PAYABLE','COMMISSION','DELIVERY_REVENUE','TAX_PAYABLE','GATEWAY_FEE','REFUND_EXPENSE','ADJUSTMENT','PAYOUT_CLEARING','PROMOTION_EXPENSE','ONBOARDING_REVENUE'));

-- yember-end-step

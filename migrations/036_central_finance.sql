CREATE SCHEMA finance;
ALTER TABLE public.merchants ADD COLUMN commission_bps integer NOT NULL DEFAULT 1500 CHECK(commission_bps BETWEEN 0 AND 10000);
CREATE TABLE finance.permissions (
 user_id uuid REFERENCES public.users(id), permission text CHECK(permission IN ('READ','CONFIGURE','PREPARE','APPROVE','PAYOUT','RECONCILE','ADJUST')), PRIMARY KEY(user_id,permission)
);
CREATE TABLE finance.policy (
 id boolean PRIMARY KEY DEFAULT true CHECK(id), interval_days integer NOT NULL DEFAULT 15 CHECK(interval_days BETWEEN 1 AND 90),
 anchor timestamptz NOT NULL DEFAULT '2026-01-01 00:00:00+00', delivery_charge bigint NOT NULL DEFAULT 0 CHECK(delivery_charge>=0),
 rider_base bigint NOT NULL DEFAULT 0 CHECK(rider_base>=0), enabled boolean NOT NULL DEFAULT false,
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO finance.policy(id) VALUES(true);
CREATE TABLE finance.bank_accounts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES public.users(id),
 ciphertext bytea NOT NULL, key_id text NOT NULL, last_four text NOT NULL CHECK(length(last_four)=4),
 status text NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','VERIFIED','REJECTED','REPLACED')),
 verified_by uuid REFERENCES public.users(id), verification_reference text, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX bank_current ON finance.bank_accounts(user_id) WHERE status IN ('PENDING','VERIFIED');
CREATE TABLE finance.order_financials (
 order_id uuid PRIMARY KEY REFERENCES public.orders(id), restaurant_user uuid NOT NULL REFERENCES public.users(id),
 currency char(3) NOT NULL DEFAULT 'INR', gross bigint NOT NULL CHECK(gross>0), discount bigint NOT NULL DEFAULT 0 CHECK(discount>=0),
 commission_bps integer NOT NULL CHECK(commission_bps BETWEEN 0 AND 10000), commission bigint NOT NULL CHECK(commission>=0),
 delivery_charge bigint NOT NULL DEFAULT 0 CHECK(delivery_charge>=0), rider_base bigint NOT NULL DEFAULT 0 CHECK(rider_base>=0),
 tax bigint NOT NULL DEFAULT 0 CHECK(tax>=0), platform_charge bigint NOT NULL DEFAULT 0 CHECK(platform_charge>=0),
 total bigint NOT NULL CHECK(total>0), created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(discount<=gross), CHECK(commission<=gross), CHECK(total=gross-discount+delivery_charge+tax+platform_charge)
);
CREATE TABLE finance.payment_attempts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), order_id uuid NOT NULL REFERENCES finance.order_financials(order_id),
 attempt integer NOT NULL DEFAULT 1, provider text NOT NULL CHECK(provider IN ('RAZORPAY','COD')),
 amount bigint NOT NULL CHECK(amount>0), currency char(3) NOT NULL, provider_id text UNIQUE, payment_reference text UNIQUE,
 checkout_url text, expires_at timestamptz, status text NOT NULL DEFAULT 'CREATING' CHECK(status IN ('CREATING','PENDING','PAID','EXPIRED','FAILED')),
 request_started_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(order_id,attempt)
);
CREATE UNIQUE INDEX one_captured_order ON finance.payment_attempts(order_id) WHERE status='PAID';
CREATE TABLE finance.journals (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), event_key text NOT NULL UNIQUE, kind text NOT NULL,
 order_id uuid REFERENCES finance.order_financials(order_id), reference text NOT NULL, reason text NOT NULL,
 actor uuid REFERENCES public.users(id), currency char(3) NOT NULL DEFAULT 'INR', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE finance.entries (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), journal_id uuid NOT NULL REFERENCES finance.journals(id),
 account text NOT NULL CHECK(account IN ('CASH','UNEARNED','PARTNER_PAYABLE','COMMISSION','DELIVERY_REVENUE','TAX_PAYABLE','GATEWAY_FEE','REFUND_EXPENSE','ADJUSTMENT','PAYOUT_CLEARING')),
 user_id uuid REFERENCES public.users(id), component text NOT NULL, amount bigint NOT NULL CHECK(amount<>0),
 created_at timestamptz NOT NULL DEFAULT now(), CHECK((account='PARTNER_PAYABLE')=(user_id IS NOT NULL))
);
CREATE INDEX entries_partner ON finance.entries(user_id,created_at,id) WHERE user_id IS NOT NULL;
CREATE INDEX journals_order ON finance.journals(order_id,created_at);
CREATE FUNCTION finance.immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'financial history is immutable'; END $$;
CREATE TRIGGER immutable_financials BEFORE UPDATE OR DELETE ON finance.order_financials FOR EACH ROW EXECUTE FUNCTION finance.immutable();
CREATE TRIGGER immutable_journals BEFORE UPDATE OR DELETE ON finance.journals FOR EACH ROW EXECUTE FUNCTION finance.immutable();
CREATE TRIGGER immutable_entries BEFORE UPDATE OR DELETE ON finance.entries FOR EACH ROW EXECUTE FUNCTION finance.immutable();
CREATE FUNCTION finance.balanced() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (SELECT count(*)<2 OR COALESCE(sum(amount),0)<>0 FROM finance.entries WHERE journal_id=NEW.id) THEN RAISE EXCEPTION 'unbalanced financial journal'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER balanced_journal AFTER INSERT ON finance.journals DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION finance.balanced();
CREATE FUNCTION finance.entry_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM finance.journals WHERE id=NEW.journal_id AND xmin::text<>pg_current_xact_id()::text) THEN RAISE EXCEPTION 'cannot append to committed journal'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER entry_guard BEFORE INSERT ON finance.entries FOR EACH ROW EXECUTE FUNCTION finance.entry_guard();
CREATE TABLE finance.batches (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), cutoff timestamptz NOT NULL UNIQUE,
 prepared_by uuid REFERENCES public.users(id), approved_by uuid REFERENCES public.users(id),
 status text NOT NULL DEFAULT 'REVIEW' CHECK(status IN ('REVIEW','APPROVED')),
 created_at timestamptz NOT NULL DEFAULT now(), approved_at timestamptz,
 CHECK(prepared_by IS NULL OR approved_by IS NULL OR prepared_by<>approved_by)
);
CREATE TABLE finance.payouts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), batch_id uuid NOT NULL REFERENCES finance.batches(id),
 user_id uuid NOT NULL REFERENCES public.users(id), bank_account_id uuid NOT NULL REFERENCES finance.bank_accounts(id),
 amount bigint NOT NULL CHECK(amount>0), currency char(3) NOT NULL DEFAULT 'INR',
 status text NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','PROCESSING','SUCCEEDED','FAILED','UNKNOWN','REVERSED')),
 provider text NOT NULL DEFAULT 'BANK_RECONCILIATION', provider_id text UNIQUE, initiated_by uuid REFERENCES public.users(id),
 confirmed_by uuid REFERENCES public.users(id), evidence text, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(batch_id,user_id)
);
CREATE TABLE finance.settlement_entries (
 entry_id uuid PRIMARY KEY REFERENCES finance.entries(id), payout_id uuid NOT NULL REFERENCES finance.payouts(id)
);
CREATE TABLE finance.payout_history (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), payout_id uuid NOT NULL REFERENCES finance.payouts(id),
 status text NOT NULL, reference text NOT NULL, actor uuid REFERENCES public.users(id), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER immutable_payout_history BEFORE UPDATE OR DELETE ON finance.payout_history FOR EACH ROW EXECUTE FUNCTION finance.immutable();
CREATE TRIGGER immutable_settlement_entries BEFORE UPDATE OR DELETE ON finance.settlement_entries FOR EACH ROW EXECUTE FUNCTION finance.immutable();
CREATE TABLE finance.provider_events (
 provider text NOT NULL, event_id text NOT NULL, digest text NOT NULL, received_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(provider,event_id)
);
CREATE TABLE finance.refund_requests (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), order_id uuid NOT NULL REFERENCES finance.order_financials(order_id),
 request_key text NOT NULL UNIQUE, amount bigint NOT NULL CHECK(amount>0), reason text NOT NULL,
 requested_by uuid NOT NULL REFERENCES public.users(id), provider_id text UNIQUE, status text NOT NULL DEFAULT 'REQUESTED',
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE finance.discrepancies (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), order_id uuid REFERENCES finance.order_financials(order_id),
 event_key text NOT NULL UNIQUE, kind text NOT NULL, details jsonb NOT NULL, resolved_by uuid REFERENCES public.users(id),
 resolution text, created_at timestamptz NOT NULL DEFAULT now(), resolved_at timestamptz
);
CREATE TABLE finance.reconciliations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), actor uuid REFERENCES public.users(id), reference text NOT NULL UNIQUE,
 details jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER immutable_reconciliations BEFORE UPDATE OR DELETE ON finance.reconciliations FOR EACH ROW EXECUTE FUNCTION finance.immutable();

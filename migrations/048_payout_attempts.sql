ALTER TABLE finance.payouts ADD COLUMN attempt_no integer NOT NULL DEFAULT 0 CHECK(attempt_no>=0);
CREATE TABLE finance.payout_attempts(payout_id uuid NOT NULL REFERENCES finance.payouts(id),attempt_no integer NOT NULL,idempotency_key uuid NOT NULL UNIQUE,provider_id text UNIQUE,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(payout_id,attempt_no));
CREATE FUNCTION finance.protect_payout_attempt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.payout_id<>OLD.payout_id OR NEW.attempt_no<>OLD.attempt_no OR NEW.idempotency_key<>OLD.idempotency_key OR (OLD.provider_id IS NOT NULL AND NEW.provider_id IS DISTINCT FROM OLD.provider_id) THEN RAISE EXCEPTION 'payout attempt is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER payout_attempt_guard BEFORE UPDATE ON finance.payout_attempts FOR EACH ROW EXECUTE FUNCTION finance.protect_payout_attempt();
CREATE TRIGGER payout_attempt_delete_guard BEFORE DELETE ON finance.payout_attempts FOR EACH ROW EXECUTE FUNCTION finance.immutable();

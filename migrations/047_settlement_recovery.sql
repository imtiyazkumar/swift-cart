ALTER TABLE finance.payouts DROP CONSTRAINT payouts_status_check;
ALTER TABLE finance.payouts ADD CONSTRAINT payouts_status_check CHECK(status IN ('PENDING','PROCESSING','SUCCEEDED','FAILED','UNKNOWN','REVERSED','CANCELLED'));
-- Multiple historical reservations are retained through release/re-reservation events.
ALTER TABLE finance.settlement_entries DROP CONSTRAINT settlement_entries_pkey;
ALTER TABLE finance.settlement_entries ADD PRIMARY KEY(entry_id,payout_id);
CREATE TABLE finance.settlement_releases(entry_id uuid NOT NULL,payout_id uuid NOT NULL,actor uuid NOT NULL REFERENCES public.users(id),reason text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(entry_id,payout_id),FOREIGN KEY(entry_id,payout_id) REFERENCES finance.settlement_entries(entry_id,payout_id));
CREATE TRIGGER release_history_guard BEFORE UPDATE OR DELETE ON finance.settlement_releases FOR EACH ROW EXECUTE FUNCTION finance.immutable();
CREATE FUNCTION finance.reserve_once() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM pg_advisory_xact_lock(894213);
 IF EXISTS(SELECT 1 FROM finance.settlement_entries se WHERE se.entry_id=NEW.entry_id AND NOT EXISTS(SELECT 1 FROM finance.settlement_releases r WHERE r.entry_id=se.entry_id AND r.payout_id=se.payout_id)) THEN RAISE EXCEPTION 'entry already reserved'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER reservation_guard BEFORE INSERT ON finance.settlement_entries FOR EACH ROW EXECUTE FUNCTION finance.reserve_once();

CREATE FUNCTION finance.protect_refund() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.order_id<>OLD.order_id OR NEW.request_key<>OLD.request_key OR NEW.amount<>OLD.amount OR NEW.reason<>OLD.reason OR NEW.requested_by<>OLD.requested_by THEN RAISE EXCEPTION 'refund request financial terms are immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER refund_terms_guard BEFORE UPDATE ON finance.refund_requests FOR EACH ROW EXECUTE FUNCTION finance.protect_refund();
CREATE TRIGGER refund_delete_guard BEFORE DELETE ON finance.refund_requests FOR EACH ROW EXECUTE FUNCTION finance.immutable();
CREATE TRIGGER provider_event_history_guard BEFORE UPDATE OR DELETE ON finance.provider_events FOR EACH ROW EXECUTE FUNCTION finance.immutable();
CREATE FUNCTION finance.protect_entry_currency() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.currency<>'INR' THEN RAISE EXCEPTION 'unsupported ledger currency'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER currency_guard BEFORE INSERT ON finance.journals FOR EACH ROW EXECUTE FUNCTION finance.protect_entry_currency();

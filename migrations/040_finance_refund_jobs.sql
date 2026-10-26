CREATE TABLE finance.refund_jobs(order_id uuid PRIMARY KEY REFERENCES finance.order_financials(order_id),status text NOT NULL DEFAULT 'PENDING',last_error text,created_at timestamptz NOT NULL DEFAULT now());
CREATE FUNCTION finance.queue_cancellation_refund() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status::text IN ('CANCELLED','REJECTED') AND NEW.status IS DISTINCT FROM OLD.status THEN
 INSERT INTO finance.refund_jobs(order_id) SELECT NEW.id FROM finance.payment_attempts WHERE order_id=NEW.id AND status='PAID' AND provider='RAZORPAY' ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER queue_cancellation_refund AFTER UPDATE OF status ON public.orders FOR EACH ROW EXECUTE FUNCTION finance.queue_cancellation_refund();

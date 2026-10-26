CREATE TABLE orders.order_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id uuid NOT NULL REFERENCES public.orders(id) ON DELETE CASCADE,
    event_type text NOT NULL,
    status order_status NOT NULL,
    changed_by uuid REFERENCES public.users(id),
    notes text,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    source_history_id uuid UNIQUE REFERENCES orders.order_status_history(id),
    capture_transaction bigint
);
CREATE INDEX order_events_timeline ON orders.order_events(order_id,created_at,id);

-- Preserve known timestamps and actor details from existing history.
INSERT INTO orders.order_events(id,order_id,event_type,status,changed_by,notes,created_at,source_history_id)
SELECT id,order_id,CASE WHEN status='CREATED' THEN 'ORDER_PLACED' WHEN notes='Delivery partner accepted assignment' THEN 'DELIVERY_ASSIGNED' ELSE 'STATUS_CHANGED' END,status,changed_by,notes,created_at,id
FROM orders.order_status_history;
INSERT INTO orders.order_events(order_id,event_type,status,created_at,notes)
SELECT o.id,'ORDER_PLACED','CREATED',o.created_at,'Recovered from order creation timestamp'
FROM public.orders o WHERE NOT EXISTS(SELECT 1 FROM orders.order_events e WHERE e.order_id=o.id AND e.event_type='ORDER_PLACED');
INSERT INTO orders.order_events(order_id,event_type,status,created_at,notes)
SELECT o.id,'STATUS_RECORDED',o.status::order_status,GREATEST(o.updated_at,o.created_at),'Existing status at migration; intermediate transition times were not recorded'
FROM public.orders o WHERE NOT EXISTS(SELECT 1 FROM orders.order_events e WHERE e.order_id=o.id AND e.status::text=o.status::text);

CREATE FUNCTION orders.capture_order_event() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='INSERT' THEN
        INSERT INTO orders.order_events(order_id,event_type,status,created_at,capture_transaction)
        VALUES(NEW.id,'ORDER_PLACED','CREATED',NEW.created_at,txid_current());
        IF NEW.status<>'CREATED' THEN
            INSERT INTO orders.order_events(order_id,event_type,status,created_at,capture_transaction)
            VALUES(NEW.id,'STATUS_RECORDED',NEW.status::order_status,GREATEST(NEW.updated_at,NEW.created_at),txid_current());
        END IF;
    ELSIF NEW.status IS DISTINCT FROM OLD.status THEN
        INSERT INTO orders.order_events(order_id,event_type,status,capture_transaction)
        VALUES(NEW.id,'STATUS_CHANGED',NEW.status::order_status,txid_current());
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER capture_order_events AFTER INSERT OR UPDATE OF status ON public.orders FOR EACH ROW EXECUTE FUNCTION orders.capture_order_event();

-- Existing writers supply actor/reason after updating the order in the same transaction.
-- Enrich that automatic event rather than displaying the transition twice.
CREATE FUNCTION orders.capture_order_history_event() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE captured uuid;
BEGIN
    SELECT id INTO captured FROM orders.order_events
    WHERE order_id=NEW.order_id AND status=NEW.status AND source_history_id IS NULL
        AND capture_transaction=txid_current()
    ORDER BY created_at DESC,id DESC LIMIT 1 FOR UPDATE;
    IF captured IS NOT NULL THEN
        UPDATE orders.order_events SET changed_by=NEW.changed_by,notes=NEW.notes,source_history_id=NEW.id WHERE id=captured;
    ELSE
        INSERT INTO orders.order_events(order_id,event_type,status,changed_by,notes,created_at,source_history_id)
        VALUES(NEW.order_id,CASE WHEN NEW.notes='Delivery partner accepted assignment' THEN 'DELIVERY_ASSIGNED' ELSE 'STATUS_RECORDED' END,NEW.status,NEW.changed_by,NEW.notes,NEW.created_at,NEW.id);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER capture_order_history_events AFTER INSERT ON orders.order_status_history FOR EACH ROW EXECUTE FUNCTION orders.capture_order_history_event();

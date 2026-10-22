ALTER TABLE delivery.offers ADD COLUMN station_id uuid REFERENCES public.stations(id);
ALTER TABLE delivery.delivery_assignments ADD COLUMN station_id uuid REFERENCES public.stations(id);
ALTER TABLE delivery.rider_earnings ADD COLUMN station_id uuid REFERENCES public.stations(id);
ALTER TABLE public.partner_support_tickets ADD COLUMN station_id uuid REFERENCES public.stations(id);

-- Historical delivery records inherit the order snapshot, never today's partner station.
UPDATE delivery.offers r SET station_id=o.station_id FROM public.orders o WHERE o.id=r.order_id AND o.station_id IS NOT NULL;
UPDATE delivery.delivery_assignments r SET station_id=o.station_id FROM public.orders o WHERE o.id=r.order_id AND o.station_id IS NOT NULL;
UPDATE delivery.rider_earnings r SET station_id=o.station_id FROM public.orders o WHERE o.id=r.order_id AND o.station_id IS NOT NULL;
UPDATE public.partner_support_tickets r SET station_id=o.station_id FROM public.orders o WHERE o.id=r.order_id AND o.station_id IS NOT NULL;

CREATE INDEX order_station_history ON public.orders(station_id,created_at DESC);
CREATE INDEX offer_station_history ON delivery.offers(station_id,created_at DESC);
CREATE INDEX assignment_station_history ON delivery.delivery_assignments(station_id,created_at DESC);
CREATE INDEX earnings_station_history ON delivery.rider_earnings(station_id,earned_at DESC);
CREATE INDEX support_station_history ON public.partner_support_tickets(station_id,created_at DESC);

CREATE FUNCTION public.guard_order_station_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.station_id IS NOT NULL AND (NEW.station_id IS DISTINCT FROM OLD.station_id OR NEW.merchant_id IS DISTINCT FROM OLD.merchant_id) THEN
        RAISE EXCEPTION 'An order station is historical and cannot be reassigned.';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER order_station_snapshot BEFORE UPDATE OF station_id,merchant_id ON public.orders FOR EACH ROW EXECUTE FUNCTION public.guard_order_station_snapshot();

CREATE FUNCTION public.inherit_order_station() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE expected uuid;
BEGIN
    SELECT station_id INTO expected FROM public.orders WHERE id=NEW.order_id;
    IF TG_OP='UPDATE' AND (NEW.order_id IS DISTINCT FROM OLD.order_id OR (OLD.station_id IS NOT NULL AND NEW.station_id IS DISTINCT FROM OLD.station_id)) THEN
        RAISE EXCEPTION 'Historical order/station relation cannot be reassigned.';
    END IF;
    IF NEW.station_id IS NOT NULL AND NEW.station_id IS DISTINCT FROM expected THEN
        RAISE EXCEPTION 'Delivery records must use the station recorded on their order.';
    END IF;
    NEW.station_id:=expected;
    RETURN NEW;
END $$;
CREATE TRIGGER offer_order_station BEFORE INSERT OR UPDATE OF order_id,station_id ON delivery.offers FOR EACH ROW EXECUTE FUNCTION public.inherit_order_station();
CREATE TRIGGER assignment_order_station BEFORE INSERT OR UPDATE OF order_id,station_id ON delivery.delivery_assignments FOR EACH ROW EXECUTE FUNCTION public.inherit_order_station();
CREATE TRIGGER earnings_order_station BEFORE INSERT OR UPDATE OF order_id,station_id ON delivery.rider_earnings FOR EACH ROW EXECUTE FUNCTION public.inherit_order_station();

CREATE FUNCTION public.support_station_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE expected uuid;
BEGIN
    IF TG_OP='UPDATE' AND OLD.station_id IS NOT NULL THEN
        IF NEW.station_id IS DISTINCT FROM OLD.station_id OR NEW.order_id IS DISTINCT FROM OLD.order_id OR NEW.user_id IS DISTINCT FROM OLD.user_id THEN
            RAISE EXCEPTION 'Historical support station cannot be reassigned.';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.order_id IS NOT NULL THEN
        SELECT station_id INTO expected FROM public.orders WHERE id=NEW.order_id;
    ELSIF TG_OP='INSERT' THEN
        SELECT station_id INTO expected FROM public.merchants WHERE owner_id=NEW.user_id AND deleted_at IS NULL ORDER BY created_at,id LIMIT 1;
        IF expected IS NULL THEN
            SELECT station_id INTO expected FROM delivery.delivery_partners WHERE user_id=NEW.user_id AND deleted_at IS NULL ORDER BY created_at,id LIMIT 1;
        END IF;
    END IF;
    IF NEW.station_id IS NOT NULL AND NEW.station_id IS DISTINCT FROM expected THEN
        RAISE EXCEPTION 'Support station must match its linked order or partner context.';
    END IF;
    NEW.station_id:=expected;
    RETURN NEW;
END $$;
CREATE TRIGGER support_station_context BEFORE INSERT OR UPDATE OF station_id,order_id,user_id ON public.partner_support_tickets FOR EACH ROW EXECUTE FUNCTION public.support_station_snapshot();

-- A later recovery of a genuinely known legacy order station repairs its children.
CREATE FUNCTION public.propagate_order_station() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.station_id IS NULL AND NEW.station_id IS NOT NULL THEN
        UPDATE delivery.offers SET station_id=NEW.station_id WHERE order_id=NEW.id AND station_id IS NULL;
        UPDATE delivery.delivery_assignments SET station_id=NEW.station_id WHERE order_id=NEW.id AND station_id IS NULL;
        UPDATE delivery.rider_earnings SET station_id=NEW.station_id WHERE order_id=NEW.id AND station_id IS NULL;
        UPDATE public.partner_support_tickets SET station_id=NEW.station_id WHERE order_id=NEW.id AND station_id IS NULL;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER recovered_order_station AFTER UPDATE OF station_id ON public.orders FOR EACH ROW EXECUTE FUNCTION public.propagate_order_station();

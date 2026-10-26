-- Domain: delivery
-- Applied through cmd/migrate using manifest.json; do not execute this file directly.
-- yember-step: 006_domain_schemas.sql/005

CREATE SCHEMA IF NOT EXISTS delivery;
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/027


CREATE TABLE IF NOT EXISTS delivery.delivery_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES public.orders(id) ON DELETE CASCADE,
    partner_id UUID NOT NULL REFERENCES delivery.delivery_partners(id),
    status TEXT NOT NULL DEFAULT 'ASSIGNED',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(order_id, partner_id)
);
-- yember-end-step
-- yember-step: 024_delivery_offers.sql/003


CREATE TABLE IF NOT EXISTS delivery.offers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES public.orders(id),
    partner_id UUID NOT NULL REFERENCES delivery.delivery_partners(id),
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','ACCEPTED','REJECTED','EXPIRED')),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(order_id,partner_id)
);
-- yember-end-step
-- yember-step: 024_delivery_offers.sql/004

CREATE UNIQUE INDEX IF NOT EXISTS one_pending_offer_per_order ON delivery.offers(order_id) WHERE status='PENDING';
-- yember-end-step
-- yember-step: 024_delivery_offers.sql/005

CREATE UNIQUE INDEX IF NOT EXISTS one_pending_offer_per_partner ON delivery.offers(partner_id) WHERE status='PENDING';
-- yember-end-step
-- yember-step: 024_delivery_offers.sql/006

-- Existing conflicting assignments must be reviewed by an operator; never delete them automatically.
CREATE UNIQUE INDEX IF NOT EXISTS one_active_assignment_per_order ON delivery.delivery_assignments(order_id) WHERE status IN ('ASSIGNED','PICKED_UP','OUT_FOR_DELIVERY');
-- yember-end-step
-- yember-step: 024_delivery_offers.sql/007

CREATE UNIQUE INDEX IF NOT EXISTS one_active_assignment_per_partner ON delivery.delivery_assignments(partner_id) WHERE status IN ('ASSIGNED','PICKED_UP','OUT_FOR_DELIVERY');
-- yember-end-step
-- yember-step: 024_delivery_offers.sql/009

CREATE INDEX IF NOT EXISTS offer_expiration ON delivery.offers(expires_at) WHERE status='PENDING';

-- yember-end-step
-- yember-step: 027_ratings.sql/002

CREATE TABLE IF NOT EXISTS delivery.partner_ratings (
    order_id UUID PRIMARY KEY REFERENCES public.orders(id),
    partner_id UUID NOT NULL REFERENCES delivery.delivery_partners(id),
    customer_id UUID NOT NULL REFERENCES public.customers(id),
    rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- yember-end-step
-- yember-step: 033_operational_stations.sql/023


CREATE FUNCTION public.guard_assignment_station() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE assigned uuid; operational uuid; available text;
BEGIN
    PERFORM pg_advisory_xact_lock_shared(894215);
    SELECT station_id INTO operational FROM public.orders WHERE id=NEW.order_id;
    SELECT station_id,status INTO assigned,available FROM delivery.delivery_partners WHERE id=NEW.partner_id FOR UPDATE;
    IF operational IS NULL OR assigned IS NULL OR operational<>assigned OR available<>'ONLINE' OR NOT public.partner_in_station(NEW.partner_id) THEN
        RAISE EXCEPTION 'Partner must be available with a fresh location inside the order station.';
    END IF;
    RETURN NEW;
END $$;
-- yember-end-step
-- yember-step: 033_operational_stations.sql/024

-- Only creation is guarded: crossing the boundary cannot block an accepted delivery.
CREATE TRIGGER assignment_station_guard BEFORE INSERT ON delivery.delivery_assignments FOR EACH ROW EXECUTE FUNCTION public.guard_assignment_station();
-- yember-end-step
-- yember-step: 034_station_relations.sql/001
ALTER TABLE delivery.offers ADD COLUMN station_id uuid REFERENCES public.stations(id);
-- yember-end-step
-- yember-step: 034_station_relations.sql/002

ALTER TABLE delivery.delivery_assignments ADD COLUMN station_id uuid REFERENCES public.stations(id);
-- yember-end-step
-- yember-step: 034_station_relations.sql/005


-- Historical delivery records inherit the order snapshot, never today's partner station.
UPDATE delivery.offers r SET station_id=o.station_id FROM public.orders o WHERE o.id=r.order_id AND o.station_id IS NOT NULL;
-- yember-end-step
-- yember-step: 034_station_relations.sql/006

UPDATE delivery.delivery_assignments r SET station_id=o.station_id FROM public.orders o WHERE o.id=r.order_id AND o.station_id IS NOT NULL;
-- yember-end-step
-- yember-step: 034_station_relations.sql/010

CREATE INDEX offer_station_history ON delivery.offers(station_id,created_at DESC);
-- yember-end-step
-- yember-step: 034_station_relations.sql/011

CREATE INDEX assignment_station_history ON delivery.delivery_assignments(station_id,created_at DESC);
-- yember-end-step
-- yember-step: 034_station_relations.sql/016


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
-- yember-end-step
-- yember-step: 034_station_relations.sql/017

CREATE TRIGGER offer_order_station BEFORE INSERT OR UPDATE OF order_id,station_id ON delivery.offers FOR EACH ROW EXECUTE FUNCTION public.inherit_order_station();
-- yember-end-step
-- yember-step: 034_station_relations.sql/018

CREATE TRIGGER assignment_order_station BEFORE INSERT OR UPDATE OF order_id,station_id ON delivery.delivery_assignments FOR EACH ROW EXECUTE FUNCTION public.inherit_order_station();
-- yember-end-step
-- yember-step: 042_push_delivery.sql/007

CREATE TRIGGER offer_push AFTER INSERT ON delivery.offers FOR EACH ROW EXECUTE FUNCTION notifications.offer_push();

-- yember-end-step
-- yember-step: 049_delivery_evidence.sql/001
CREATE TABLE delivery.completion_challenges(order_id uuid PRIMARY KEY REFERENCES public.orders(id),code_hash text NOT NULL,attempts integer NOT NULL DEFAULT 0,expires_at timestamptz NOT NULL,consumed_at timestamptz);
-- yember-end-step
-- yember-step: 049_delivery_evidence.sql/002

CREATE TABLE delivery.completion_proofs(order_id uuid PRIMARY KEY REFERENCES public.orders(id),partner_id uuid NOT NULL REFERENCES delivery.delivery_partners(id),method text NOT NULL DEFAULT 'CUSTOMER_CODE',created_at timestamptz NOT NULL DEFAULT now());
-- yember-end-step
-- yember-step: 049_delivery_evidence.sql/005

CREATE TRIGGER delivery_proof_guard BEFORE UPDATE OR DELETE ON delivery.completion_proofs FOR EACH ROW EXECUTE FUNCTION finance.immutable();

-- yember-end-step
-- yember-step: 053_delivery_code_recovery.sql/001
ALTER TABLE delivery.completion_challenges ADD COLUMN sent_at timestamptz NOT NULL DEFAULT now(), ADD COLUMN resend_count integer NOT NULL DEFAULT 0 CHECK(resend_count BETWEEN 0 AND 3);

-- yember-end-step

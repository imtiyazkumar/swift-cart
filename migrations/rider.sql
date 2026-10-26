-- Domain: rider
-- Applied through cmd/migrate using manifest.json; do not execute this file directly.
-- yember-step: 006_domain_schemas.sql/025


CREATE TABLE IF NOT EXISTS delivery.delivery_partners (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    phone TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'OFFLINE',
    approval_status TEXT NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/026


CREATE TABLE IF NOT EXISTS delivery.vehicles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id UUID NOT NULL REFERENCES delivery.delivery_partners(id) ON DELETE CASCADE,
    vehicle_type TEXT NOT NULL,
    registration_number TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true
);
-- yember-end-step
-- yember-step: 009_multi_app_ecosystem.sql/007


-- Rider Earnings Log
CREATE TABLE IF NOT EXISTS delivery.rider_earnings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id UUID NOT NULL REFERENCES delivery.delivery_partners(id) ON DELETE CASCADE,
    order_id UUID NOT NULL REFERENCES public.orders(id) ON DELETE CASCADE,
    delivery_fee BIGINT NOT NULL DEFAULT 0,
    tip_amount BIGINT NOT NULL DEFAULT 0,
    earned_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 009_multi_app_ecosystem.sql/012

CREATE INDEX IF NOT EXISTS idx_rider_earnings_partner ON delivery.rider_earnings(partner_id, earned_at DESC);
-- yember-end-step
-- yember-step: 021_platform_operations.sql/006

ALTER TABLE delivery.delivery_partners ADD COLUMN IF NOT EXISTS rejection_reason TEXT NOT NULL DEFAULT '';
-- yember-end-step
-- yember-step: 021_platform_operations.sql/007

ALTER TABLE delivery.delivery_partners ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;
-- yember-end-step
-- yember-step: 024_delivery_offers.sql/001
ALTER TABLE delivery.delivery_partners ADD COLUMN IF NOT EXISTS last_offered_at TIMESTAMPTZ;
-- yember-end-step
-- yember-step: 024_delivery_offers.sql/002

ALTER TABLE delivery.delivery_partners ADD COLUMN IF NOT EXISTS eligibility_expires_at TIMESTAMPTZ;
-- yember-end-step
-- yember-step: 033_operational_stations.sql/003

ALTER TABLE delivery.delivery_partners ADD COLUMN station_id uuid REFERENCES public.stations(id);
-- yember-end-step
-- yember-step: 033_operational_stations.sql/010

CREATE INDEX partner_station ON delivery.delivery_partners(station_id);
-- yember-end-step
-- yember-step: 033_operational_stations.sql/014

CREATE FUNCTION public.partner_in_station(partner uuid)
RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT COALESCE((SELECT s.active AND l.permission_granted AND NOT l.mocked
        AND l.observed_at BETWEEN now()-interval '2 minutes' AND now()+interval '30 seconds'
        AND l.recorded_at >= now()-interval '2 minutes' AND l.accuracy_m BETWEEN 0 AND 100
        AND l.latitude BETWEEN -90 AND 90 AND l.longitude BETWEEN -180 AND 180
        AND public.station_distance_m(s.latitude,s.longitude,l.latitude::float8,l.longitude::float8)+l.accuracy_m<=s.radius_m
        FROM delivery.delivery_partners p JOIN public.stations s ON s.id=p.station_id
        JOIN LATERAL(SELECT * FROM delivery.live_locations WHERE partner_id=p.id ORDER BY recorded_at DESC,id DESC LIMIT 1) l ON true
        WHERE p.id=partner),false);
$$;
-- yember-end-step
-- yember-step: 033_operational_stations.sql/016

UPDATE delivery.delivery_partners SET status='OFFLINE' WHERE station_id IS NULL;
-- yember-end-step
-- yember-step: 033_operational_stations.sql/019


CREATE FUNCTION public.guard_partner_station() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock_shared(894215);
    IF TG_OP='INSERT' AND NOT EXISTS(SELECT 1 FROM public.stations WHERE id=NEW.station_id AND active) THEN
        RAISE EXCEPTION 'An assigned active station is required.';
    END IF;
    IF TG_OP='UPDATE' AND NEW.station_id IS DISTINCT FROM OLD.station_id AND NOT EXISTS(
        SELECT 1 FROM public.station_change_requests r WHERE r.resource_id=NEW.id AND r.kind='delivery' AND r.station_id=NEW.station_id AND r.status='PENDING' AND r.reviewed_by IS NOT NULL) THEN
        RAISE EXCEPTION 'An authorized station-change approval is required.';
    END IF;
    IF NEW.approval_status='APPROVED' AND (TG_OP='INSERT' OR NEW.approval_status IS DISTINCT FROM OLD.approval_status OR NEW.station_id IS DISTINCT FROM OLD.station_id) AND NOT EXISTS(SELECT 1 FROM public.stations WHERE id=NEW.station_id AND active) THEN
        RAISE EXCEPTION 'An assigned active station is required before approval.';
    END IF;
    IF NEW.status='ONLINE' AND NOT public.partner_in_station(NEW.id) THEN
        RAISE EXCEPTION 'A recent accurate GPS location inside your assigned active station is required. Enable location access and return to your station.';
    END IF;
    RETURN NEW;
END $$;
-- yember-end-step
-- yember-step: 033_operational_stations.sql/020

CREATE TRIGGER partner_station_guard BEFORE INSERT OR UPDATE ON delivery.delivery_partners FOR EACH ROW EXECUTE FUNCTION public.guard_partner_station();
-- yember-end-step
-- yember-step: 034_station_relations.sql/003

ALTER TABLE delivery.rider_earnings ADD COLUMN station_id uuid REFERENCES public.stations(id);
-- yember-end-step
-- yember-step: 034_station_relations.sql/007

UPDATE delivery.rider_earnings r SET station_id=o.station_id FROM public.orders o WHERE o.id=r.order_id AND o.station_id IS NOT NULL;
-- yember-end-step
-- yember-step: 034_station_relations.sql/012

CREATE INDEX earnings_station_history ON delivery.rider_earnings(station_id,earned_at DESC);
-- yember-end-step
-- yember-step: 034_station_relations.sql/019

CREATE TRIGGER earnings_order_station BEFORE INSERT OR UPDATE OF order_id,station_id ON delivery.rider_earnings FOR EACH ROW EXECUTE FUNCTION public.inherit_order_station();
-- yember-end-step

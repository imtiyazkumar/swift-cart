CREATE TABLE public.stations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(trim(name)) BETWEEN 2 AND 160),
    latitude double precision NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude double precision NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    radius_m double precision NOT NULL CHECK (radius_m > 0 AND radius_m <= 100000),
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE public.merchants ADD COLUMN station_id uuid REFERENCES public.stations(id),
    ADD COLUMN latitude double precision CHECK (latitude BETWEEN -90 AND 90),
    ADD COLUMN longitude double precision CHECK (longitude BETWEEN -180 AND 180);
ALTER TABLE delivery.delivery_partners ADD COLUMN station_id uuid REFERENCES public.stations(id);
ALTER TABLE public.orders ADD COLUMN station_id uuid REFERENCES public.stations(id);
ALTER TABLE orders.carts ADD COLUMN delivery_address_id uuid REFERENCES users.customer_addresses(id);
ALTER TABLE delivery.live_locations ADD COLUMN observed_at timestamptz,
    ADD COLUMN accuracy_m double precision CHECK (accuracy_m >= 0 AND accuracy_m <= 10000),
    ADD COLUMN permission_granted boolean NOT NULL DEFAULT false,
    ADD COLUMN mocked boolean NOT NULL DEFAULT false;
CREATE TABLE public.station_change_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind text NOT NULL CHECK (kind IN ('restaurant','delivery')),
    resource_id uuid NOT NULL,
    requested_by uuid NOT NULL REFERENCES public.users(id),
    station_id uuid NOT NULL REFERENCES public.stations(id),
    latitude double precision CHECK (latitude BETWEEN -90 AND 90),
    longitude double precision CHECK (longitude BETWEEN -180 AND 180),
    reason text NOT NULL CHECK (length(trim(reason)) BETWEEN 3 AND 2000),
    status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','APPROVED','REJECTED')),
    reviewed_by uuid REFERENCES public.users(id),
    review_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    reviewed_at timestamptz
);
CREATE UNIQUE INDEX station_change_pending ON public.station_change_requests(kind,resource_id) WHERE status='PENDING';
CREATE INDEX merchant_station ON public.merchants(station_id);
CREATE INDEX partner_station ON delivery.delivery_partners(station_id);

CREATE FUNCTION public.station_distance_m(lat1 double precision, lon1 double precision, lat2 double precision, lon2 double precision)
RETURNS double precision LANGUAGE sql IMMUTABLE STRICT AS $$
    SELECT 6371008.8 * 2 * asin(sqrt(least(1.0,greatest(0.0,
        power(sin(radians(lat2-lat1)/2),2) + cos(radians(lat1))*cos(radians(lat2))*power(sin(radians(lon2-lon1)/2),2)))));
$$;
CREATE FUNCTION public.station_covers(station uuid, lat double precision, lon double precision)
RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT lat BETWEEN -90 AND 90 AND lon BETWEEN -180 AND 180 AND EXISTS(
        SELECT 1 FROM public.stations s WHERE s.id=station AND s.active
        AND public.station_distance_m(s.latitude,s.longitude,lat,lon)<=s.radius_m);
$$;
CREATE FUNCTION public.resolve_station(lat double precision, lon double precision)
RETURNS uuid LANGUAGE sql STABLE AS $$
    SELECT s.id FROM public.stations s WHERE s.active AND lat BETWEEN -90 AND 90 AND lon BETWEEN -180 AND 180
    AND public.station_distance_m(s.latitude,s.longitude,lat,lon)<=s.radius_m
    ORDER BY public.station_distance_m(s.latitude,s.longitude,lat,lon),s.id LIMIT 1;
$$;
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
-- No invented stations or default coverage: unassigned legacy accounts fail closed.
UPDATE public.merchants SET is_active=false WHERE station_id IS NULL;
UPDATE delivery.delivery_partners SET status='OFFLINE' WHERE station_id IS NULL;

CREATE FUNCTION public.guard_restaurant_station() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock_shared(894215);
    IF TG_OP='INSERT' THEN
        NEW.station_id := public.resolve_station(NEW.latitude,NEW.longitude);
        IF NEW.station_id IS NULL THEN RAISE EXCEPTION 'Yember does not serve this area yet. Valid restaurant coordinates within an active station are required.'; END IF;
    ELSIF NEW.station_id IS DISTINCT FROM OLD.station_id THEN
        IF NOT EXISTS(SELECT 1 FROM public.station_change_requests r WHERE r.resource_id=NEW.id AND r.kind='restaurant' AND r.station_id=NEW.station_id AND r.status='PENDING' AND r.reviewed_by IS NOT NULL) THEN
            RAISE EXCEPTION 'An authorized station-change approval is required.';
        END IF;
    END IF;
    IF TG_OP='INSERT' OR NEW.latitude IS DISTINCT FROM OLD.latitude OR NEW.longitude IS DISTINCT FROM OLD.longitude OR NEW.station_id IS DISTINCT FROM OLD.station_id
       OR (NEW.approval_status='APPROVED' AND NEW.approval_status IS DISTINCT FROM OLD.approval_status) OR NEW.is_active THEN
        IF NOT COALESCE(public.station_covers(NEW.station_id,NEW.latitude,NEW.longitude),false) THEN
            RAISE EXCEPTION 'Restaurant location must be inside its assigned active station.';
        END IF;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER restaurant_station_guard BEFORE INSERT OR UPDATE ON public.merchants FOR EACH ROW EXECUTE FUNCTION public.guard_restaurant_station();

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
CREATE TRIGGER partner_station_guard BEFORE INSERT OR UPDATE ON delivery.delivery_partners FOR EACH ROW EXECUTE FUNCTION public.guard_partner_station();

CREATE FUNCTION public.guard_order_station() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE resolved uuid; assigned uuid;
BEGIN
    PERFORM pg_advisory_xact_lock_shared(894215);
    resolved:=public.resolve_station((NEW.delivery_address->>'latitude')::float8,(NEW.delivery_address->>'longitude')::float8);
    SELECT station_id INTO assigned FROM public.merchants WHERE id=NEW.merchant_id AND public.station_covers(station_id,latitude,longitude) FOR SHARE;
    IF resolved IS NULL THEN RAISE EXCEPTION 'Delivery is unavailable at this location. Choose another delivery address.'; END IF;
    IF assigned IS NULL OR assigned<>resolved THEN RAISE EXCEPTION 'Restaurant does not deliver to the selected station.'; END IF;
    NEW.station_id:=resolved;
    RETURN NEW;
END $$;
CREATE TRIGGER order_station_guard BEFORE INSERT ON public.orders FOR EACH ROW EXECUTE FUNCTION public.guard_order_station();

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
-- Only creation is guarded: crossing the boundary cannot block an accepted delivery.
CREATE TRIGGER assignment_station_guard BEFORE INSERT ON delivery.delivery_assignments FOR EACH ROW EXECUTE FUNCTION public.guard_assignment_station();

ALTER TABLE users.customer_addresses ADD CONSTRAINT valid_address_coordinates CHECK (
    (latitude IS NULL AND longitude IS NULL) OR
    (latitude IS NOT NULL AND longitude IS NOT NULL AND latitude BETWEEN -90 AND 90 AND longitude BETWEEN -180 AND 180)
) NOT VALID;

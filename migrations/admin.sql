-- Domain: admin
-- Applied through cmd/migrate using manifest.json; do not execute this file directly.
-- yember-step: 009_multi_app_ecosystem.sql/008


-- Admin Audit Logs
CREATE TABLE IF NOT EXISTS public.admin_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id UUID NOT NULL REFERENCES public.users(id),
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT,
    details JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 009_multi_app_ecosystem.sql/013

CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_admin ON public.admin_audit_logs(admin_id, created_at DESC);

-- yember-end-step
-- yember-step: 021_platform_operations.sql/008


CREATE TABLE IF NOT EXISTS public.platform_settings (
    key TEXT PRIMARY KEY,
    value JSONB NOT NULL,
    updated_by UUID REFERENCES public.users(id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 022_partner_onboarding.sql/002

CREATE TABLE IF NOT EXISTS public.private_documents (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES public.users(id),
    document_type TEXT NOT NULL,
    content_type TEXT NOT NULL,
    byte_size BIGINT NOT NULL CHECK (byte_size BETWEEN 1 AND 10485760),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 022_partner_onboarding.sql/003

CREATE TABLE IF NOT EXISTS public.partner_applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES public.users(id),
    kind TEXT NOT NULL CHECK (kind IN ('restaurant','delivery')),
    resource_id UUID NOT NULL,
    data JSONB NOT NULL,
    document_ids UUID[] NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id,kind)
);
-- yember-end-step
-- yember-step: 022_partner_onboarding.sql/004

CREATE INDEX IF NOT EXISTS partner_applications_resource ON public.partner_applications(resource_id);

-- yember-end-step
-- yember-step: 028_document_backend.sql/001
ALTER TABLE public.private_documents ADD COLUMN IF NOT EXISTS backend TEXT NOT NULL DEFAULT 'local';

-- yember-end-step
-- yember-step: 032_partner_workspaces.sql/001
CREATE TABLE IF NOT EXISTS public.partner_support_tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES public.users(id),
    order_id UUID REFERENCES public.orders(id),
    category TEXT NOT NULL CHECK (category IN ('ORDER','ACCOUNT','EARNINGS','MENU','OTHER')),
    subject TEXT NOT NULL CHECK (length(subject) BETWEEN 3 AND 160),
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','IN_PROGRESS','RESOLVED')),
    request_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id,request_key)
);
-- yember-end-step
-- yember-step: 032_partner_workspaces.sql/002

CREATE TABLE IF NOT EXISTS public.partner_support_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id UUID NOT NULL REFERENCES public.partner_support_tickets(id),
    author_id UUID NOT NULL REFERENCES public.users(id),
    body TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 4000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 032_partner_workspaces.sql/003

CREATE INDEX IF NOT EXISTS partner_support_owner ON public.partner_support_tickets(user_id,created_at DESC);
-- yember-end-step
-- yember-step: 032_partner_workspaces.sql/004

CREATE INDEX IF NOT EXISTS partner_support_thread ON public.partner_support_messages(ticket_id,created_at);
-- yember-end-step
-- yember-step: 033_operational_stations.sql/001
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
-- yember-end-step
-- yember-step: 033_operational_stations.sql/007

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
-- yember-end-step
-- yember-step: 033_operational_stations.sql/008

CREATE UNIQUE INDEX station_change_pending ON public.station_change_requests(kind,resource_id) WHERE status='PENDING';
-- yember-end-step
-- yember-step: 033_operational_stations.sql/011


CREATE FUNCTION public.station_distance_m(lat1 double precision, lon1 double precision, lat2 double precision, lon2 double precision)
RETURNS double precision LANGUAGE sql IMMUTABLE STRICT AS $$
    SELECT 6371008.8 * 2 * asin(sqrt(least(1.0,greatest(0.0,
        power(sin(radians(lat2-lat1)/2),2) + cos(radians(lat1))*cos(radians(lat2))*power(sin(radians(lon2-lon1)/2),2)))));
$$;
-- yember-end-step
-- yember-step: 033_operational_stations.sql/012

CREATE FUNCTION public.station_covers(station uuid, lat double precision, lon double precision)
RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT lat BETWEEN -90 AND 90 AND lon BETWEEN -180 AND 180 AND EXISTS(
        SELECT 1 FROM public.stations s WHERE s.id=station AND s.active
        AND public.station_distance_m(s.latitude,s.longitude,lat,lon)<=s.radius_m);
$$;
-- yember-end-step
-- yember-step: 033_operational_stations.sql/013

CREATE FUNCTION public.resolve_station(lat double precision, lon double precision)
RETURNS uuid LANGUAGE sql STABLE AS $$
    SELECT s.id FROM public.stations s WHERE s.active AND lat BETWEEN -90 AND 90 AND lon BETWEEN -180 AND 180
    AND public.station_distance_m(s.latitude,s.longitude,lat,lon)<=s.radius_m
    ORDER BY public.station_distance_m(s.latitude,s.longitude,lat,lon),s.id LIMIT 1;
$$;
-- yember-end-step
-- yember-step: 034_station_relations.sql/004

ALTER TABLE public.partner_support_tickets ADD COLUMN station_id uuid REFERENCES public.stations(id);
-- yember-end-step
-- yember-step: 034_station_relations.sql/008

UPDATE public.partner_support_tickets r SET station_id=o.station_id FROM public.orders o WHERE o.id=r.order_id AND o.station_id IS NOT NULL;
-- yember-end-step
-- yember-step: 034_station_relations.sql/013

CREATE INDEX support_station_history ON public.partner_support_tickets(station_id,created_at DESC);
-- yember-end-step
-- yember-step: 034_station_relations.sql/020


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
-- yember-end-step
-- yember-step: 034_station_relations.sql/021

CREATE TRIGGER support_station_context BEFORE INSERT OR UPDATE OF station_id,order_id,user_id ON public.partner_support_tickets FOR EACH ROW EXECUTE FUNCTION public.support_station_snapshot();
-- yember-end-step

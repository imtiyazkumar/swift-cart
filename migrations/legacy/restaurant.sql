-- Domain: restaurant
-- Applied through cmd/migrate using manifest.json; do not execute this file directly.
-- yember-step: 004_merchants.sql/001
CREATE TABLE IF NOT EXISTS merchants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE,
    phone TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- yember-end-step
-- yember-step: 006_domain_schemas.sql/002

CREATE SCHEMA IF NOT EXISTS merchants;
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/009


DO $$ BEGIN
    CREATE TYPE merchant_type AS ENUM ('RESTAURANT','GROCERY','PHARMACY','PET_STORE','FLOWER_SHOP');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/013


CREATE TABLE IF NOT EXISTS merchants.merchant_branches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID NOT NULL REFERENCES public.merchants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    address TEXT NOT NULL,
    latitude NUMERIC(10,7),
    longitude NUMERIC(10,7),
    is_active BOOLEAN NOT NULL DEFAULT false,
    created_by UUID REFERENCES public.users(id),
    updated_by UUID REFERENCES public.users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/014


CREATE TABLE IF NOT EXISTS merchants.merchant_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID NOT NULL REFERENCES public.merchants(id) ON DELETE CASCADE,
    document_type TEXT NOT NULL,
    storage_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/015


CREATE TABLE IF NOT EXISTS merchants.merchant_operating_hours (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id UUID NOT NULL REFERENCES merchants.merchant_branches(id) ON DELETE CASCADE,
    day_of_week SMALLINT NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    opens_at TIME NOT NULL,
    closes_at TIME NOT NULL,
    is_closed BOOLEAN NOT NULL DEFAULT false,
    UNIQUE(branch_id, day_of_week)
);
-- yember-end-step
-- yember-step: 008_public_core_enrichment.sql/003


ALTER TABLE public.merchants
    ADD COLUMN IF NOT EXISTS merchant_type TEXT NOT NULL DEFAULT 'RESTAURANT',
    ADD COLUMN IF NOT EXISTS onboarding_status TEXT NOT NULL DEFAULT 'PROFILE_CREATED',
    ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'PENDING',
    ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
-- yember-end-step
-- yember-step: 008_public_core_enrichment.sql/008

CREATE INDEX IF NOT EXISTS idx_merchants_type ON public.merchants(merchant_type);

-- yember-end-step
-- yember-step: 009_multi_app_ecosystem.sql/002


-- Add owner_id and is_active to merchants table
ALTER TABLE public.merchants
    ADD COLUMN IF NOT EXISTS owner_id UUID REFERENCES public.users(id),
    ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;
-- yember-end-step
-- yember-step: 009_multi_app_ecosystem.sql/006


-- Restaurant Reviews & Ratings
CREATE TABLE IF NOT EXISTS merchants.restaurant_reviews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID REFERENCES public.orders(id) ON DELETE SET NULL,
    customer_id UUID NOT NULL REFERENCES public.customers(id) ON DELETE CASCADE,
    merchant_id UUID NOT NULL REFERENCES public.merchants(id) ON DELETE CASCADE,
    rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 009_multi_app_ecosystem.sql/011

CREATE INDEX IF NOT EXISTS idx_restaurant_reviews_merchant ON merchants.restaurant_reviews(merchant_id);
-- yember-end-step
-- yember-step: 021_platform_operations.sql/003

ALTER TABLE public.merchants ADD COLUMN IF NOT EXISTS rejection_reason TEXT NOT NULL DEFAULT '';
-- yember-end-step
-- yember-step: 021_platform_operations.sql/004

ALTER TABLE public.merchants ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;
-- yember-end-step
-- yember-step: 021_platform_operations.sql/005

ALTER TABLE public.merchants ADD COLUMN IF NOT EXISTS fee_payment_status TEXT NOT NULL DEFAULT 'UNPAID';
-- yember-end-step
-- yember-step: 027_ratings.sql/001
CREATE UNIQUE INDEX IF NOT EXISTS one_restaurant_review_per_order ON merchants.restaurant_reviews(order_id,customer_id) WHERE order_id IS NOT NULL;
-- yember-end-step
-- yember-step: 030_restaurant_profile.sql/001
ALTER TABLE public.merchants ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

-- yember-end-step
-- yember-step: 031_review_moderation.sql/001
ALTER TABLE merchants.restaurant_reviews ADD COLUMN IF NOT EXISTS hidden_at TIMESTAMPTZ;
-- yember-end-step
-- yember-step: 031_review_moderation.sql/002

ALTER TABLE merchants.restaurant_reviews ADD COLUMN IF NOT EXISTS hidden_by UUID REFERENCES public.users(id);
-- yember-end-step
-- yember-step: 031_review_moderation.sql/003

ALTER TABLE merchants.restaurant_reviews ADD COLUMN IF NOT EXISTS moderation_reason TEXT NOT NULL DEFAULT '';

-- yember-end-step
-- yember-step: 032_partner_workspaces.sql/005

ALTER TABLE merchants.restaurant_reviews ADD COLUMN IF NOT EXISTS reply TEXT NOT NULL DEFAULT '';
-- yember-end-step
-- yember-step: 032_partner_workspaces.sql/006

ALTER TABLE merchants.restaurant_reviews ADD COLUMN IF NOT EXISTS replied_at TIMESTAMPTZ;

-- yember-end-step
-- yember-step: 033_operational_stations.sql/002

ALTER TABLE public.merchants ADD COLUMN station_id uuid REFERENCES public.stations(id),
    ADD COLUMN latitude double precision CHECK (latitude BETWEEN -90 AND 90),
    ADD COLUMN longitude double precision CHECK (longitude BETWEEN -180 AND 180);
-- yember-end-step
-- yember-step: 033_operational_stations.sql/009

CREATE INDEX merchant_station ON public.merchants(station_id);
-- yember-end-step
-- yember-step: 033_operational_stations.sql/015

-- No invented stations or default coverage: unassigned legacy accounts fail closed.
UPDATE public.merchants SET is_active=false WHERE station_id IS NULL;
-- yember-end-step
-- yember-step: 033_operational_stations.sql/017


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
-- yember-end-step
-- yember-step: 033_operational_stations.sql/018

CREATE TRIGGER restaurant_station_guard BEFORE INSERT OR UPDATE ON public.merchants FOR EACH ROW EXECUTE FUNCTION public.guard_restaurant_station();
-- yember-end-step
-- yember-step: 036_central_finance.sql/002

ALTER TABLE public.merchants ADD COLUMN commission_bps integer NOT NULL DEFAULT 1500 CHECK(commission_bps BETWEEN 0 AND 10000);
-- yember-end-step
-- yember-step: 039_operating_hours.sql/001
-- An absent schedule preserves existing manually managed availability.
-- Configured hours use the operating market's Asia/Kolkata timezone; overnight shifts use the previous day's record.
CREATE FUNCTION public.restaurant_open_at(merchant uuid,instant timestamptz) RETURNS boolean LANGUAGE sql STABLE AS $$
 WITH local AS (SELECT instant AT TIME ZONE 'Asia/Kolkata' AS ts),
 hours AS (SELECT h.* FROM merchants.merchant_operating_hours h JOIN merchants.merchant_branches b ON b.id=h.branch_id WHERE b.merchant_id=merchant AND b.deleted_at IS NULL)
 SELECT NOT EXISTS(SELECT 1 FROM hours) OR EXISTS(
  SELECT 1 FROM hours,local WHERE NOT is_closed AND (
   (day_of_week=extract(dow FROM ts)::int AND opens_at<closes_at AND ts::time>=opens_at AND ts::time<closes_at)
   OR (opens_at>closes_at AND ((day_of_week=extract(dow FROM ts)::int AND ts::time>=opens_at) OR (day_of_week=(extract(dow FROM ts)::int+6)%7 AND ts::time<closes_at)))
  )
 )
$$;

-- yember-end-step

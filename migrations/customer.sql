-- Domain: customer
-- Applied through cmd/migrate using manifest.json; do not execute this file directly.
-- yember-step: 003_customers.sql/001
CREATE TABLE IF NOT EXISTS customers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE,
    phone TEXT,
    address TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- yember-end-step
-- yember-step: 006_domain_schemas.sql/001
CREATE SCHEMA IF NOT EXISTS users;
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/011


CREATE TABLE IF NOT EXISTS users.customer_addresses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES public.customers(id) ON DELETE CASCADE,
    label TEXT NOT NULL,
    address_line1 TEXT NOT NULL,
    address_line2 TEXT,
    city TEXT NOT NULL,
    state TEXT NOT NULL,
    postal_code TEXT NOT NULL,
    latitude NUMERIC(10,7),
    longitude NUMERIC(10,7),
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/012


CREATE TABLE IF NOT EXISTS users.saved_locations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES public.customers(id) ON DELETE CASCADE,
    label TEXT NOT NULL,
    latitude NUMERIC(10,7) NOT NULL,
    longitude NUMERIC(10,7) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
-- yember-end-step
-- yember-step: 008_public_core_enrichment.sql/002


ALTER TABLE public.customers
    ADD COLUMN IF NOT EXISTS preferences JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS wallet_balance BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
-- yember-end-step
-- yember-step: 008_public_core_enrichment.sql/007

CREATE INDEX IF NOT EXISTS idx_customers_deleted_at ON public.customers(deleted_at);
-- yember-end-step
-- yember-step: 009_multi_app_ecosystem.sql/003


-- Add user_id reference to customers table
ALTER TABLE public.customers
    ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES public.users(id);
-- yember-end-step
-- yember-step: 009_multi_app_ecosystem.sql/005


-- Customer Favorite Restaurants
CREATE TABLE IF NOT EXISTS users.customer_favorites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES public.customers(id) ON DELETE CASCADE,
    merchant_id UUID NOT NULL REFERENCES public.merchants(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(customer_id, merchant_id)
);
-- yember-end-step
-- yember-step: 009_multi_app_ecosystem.sql/010

CREATE INDEX IF NOT EXISTS idx_customer_favorites_customer ON users.customer_favorites(customer_id);
-- yember-end-step
-- yember-step: 012_nullable_email_and_phone.sql/002

ALTER TABLE public.customers ALTER COLUMN email DROP NOT NULL;
-- yember-end-step
-- yember-step: 012_nullable_email_and_phone.sql/004

UPDATE public.customers SET email = NULL WHERE email LIKE '%@yember.app' AND phone IS NOT NULL AND phone != '';
-- yember-end-step
-- yember-step: 012_nullable_email_and_phone.sql/006

CREATE UNIQUE INDEX IF NOT EXISTS idx_customers_phone ON public.customers(phone) WHERE phone IS NOT NULL AND phone != '';

-- yember-end-step
-- yember-step: 013_add_gender_and_profile.sql/002

ALTER TABLE public.customers ADD COLUMN IF NOT EXISTS gender TEXT;

-- yember-end-step
-- yember-step: 014_enforce_unique_email_and_phone.sql/003

UPDATE public.customers SET email = NULL WHERE email = '';
-- yember-end-step
-- yember-step: 014_enforce_unique_email_and_phone.sql/004

UPDATE public.customers SET phone = NULL WHERE phone = '';
-- yember-end-step
-- yember-step: 014_enforce_unique_email_and_phone.sql/007

DROP INDEX IF EXISTS public.idx_customers_email_unique;
-- yember-end-step
-- yember-step: 014_enforce_unique_email_and_phone.sql/008

DROP INDEX IF EXISTS public.idx_customers_phone_unique;
-- yember-end-step
-- yember-step: 014_enforce_unique_email_and_phone.sql/010

DROP INDEX IF EXISTS public.idx_customers_phone;
-- yember-end-step
-- yember-step: 014_enforce_unique_email_and_phone.sql/013


-- Create unique partial indexes for public.customers
CREATE UNIQUE INDEX idx_customers_email_unique ON public.customers (LOWER(email)) WHERE email IS NOT NULL AND email != '';
-- yember-end-step
-- yember-step: 014_enforce_unique_email_and_phone.sql/014

CREATE UNIQUE INDEX idx_customers_phone_unique ON public.customers (phone) WHERE phone IS NOT NULL AND phone != '';

-- yember-end-step
-- yember-step: 033_operational_stations.sql/025


ALTER TABLE users.customer_addresses ADD CONSTRAINT valid_address_coordinates CHECK (
    (latitude IS NULL AND longitude IS NULL) OR
    (latitude IS NOT NULL AND longitude IS NOT NULL AND latitude BETWEEN -90 AND 90 AND longitude BETWEEN -180 AND 180)
) NOT VALID;

-- yember-end-step
-- yember-step: 038_auth_and_operations.sql/004

CREATE TABLE users.lifecycle_requests(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),user_id uuid NOT NULL REFERENCES public.users(id),kind text NOT NULL CHECK(kind IN ('EXPORT','DELETE')),status text NOT NULL DEFAULT 'PENDING',reason text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now(),resolved_at timestamptz);
-- yember-end-step
-- yember-step: 038_auth_and_operations.sql/005

CREATE UNIQUE INDEX one_pending_lifecycle ON users.lifecycle_requests(user_id,kind) WHERE status='PENDING';

-- yember-end-step

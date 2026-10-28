-- Domain: auth
-- Applied through cmd/migrate using manifest.json; do not execute this file directly.
-- yember-step: 001_init.sql/001
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
-- yember-end-step
-- yember-step: 001_init.sql/002


CREATE SCHEMA IF NOT EXISTS auth;
-- yember-end-step
-- yember-step: 001_init.sql/003


CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'customer',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- yember-end-step
-- yember-step: 002_auth_tokens.sql/001
-- Migration for refresh tokens and sessions
CREATE TABLE IF NOT EXISTS auth.refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 002_auth_tokens.sql/002


CREATE INDEX IF NOT EXISTS idx_refresh_user ON auth.refresh_tokens(user_id);
-- yember-end-step
-- yember-step: 002_auth_tokens.sql/003


CREATE TABLE IF NOT EXISTS auth.sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_info TEXT,
    ip_address TEXT,
    user_agent TEXT,
    last_seen TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- yember-end-step
-- yember-step: 006_domain_schemas.sql/008


DO $$ BEGIN
    CREATE TYPE user_role AS ENUM ('CUSTOMER','MERCHANT_OWNER','DELIVERY_PARTNER','ADMIN','SUPPORT_AGENT');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
-- yember-end-step
-- yember-step: 008_public_core_enrichment.sql/001
ALTER TABLE public.users
    ADD COLUMN IF NOT EXISTS phone TEXT,
    ADD COLUMN IF NOT EXISTS full_name TEXT,
    ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
-- yember-end-step
-- yember-step: 008_public_core_enrichment.sql/006

CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON public.users(deleted_at);
-- yember-end-step
-- yember-step: 011_otp_codes.sql/001
-- Migration 011: Add auth.otp_codes table for OTP creation and verification

CREATE TABLE IF NOT EXISTS auth.otp_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    identifier TEXT NOT NULL,
    otp_code TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    is_used BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 011_otp_codes.sql/002


CREATE INDEX IF NOT EXISTS idx_otp_identifier ON auth.otp_codes(identifier, is_used, expires_at);

-- yember-end-step
-- yember-step: 012_nullable_email_and_phone.sql/001
-- Migration 012: Allow nullable email for phone-authenticated users
ALTER TABLE public.users ALTER COLUMN email DROP NOT NULL;
-- yember-end-step
-- yember-step: 012_nullable_email_and_phone.sql/003


-- Clean up any placeholder emails generated for phone signups
UPDATE public.users SET email = NULL WHERE email LIKE '%@yember.app' AND phone IS NOT NULL AND phone != '';
-- yember-end-step
-- yember-step: 012_nullable_email_and_phone.sql/005


-- Create unique partial index on phone number when present
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_phone ON public.users(phone) WHERE phone IS NOT NULL AND phone != '';
-- yember-end-step
-- yember-step: 013_add_gender_and_profile.sql/001
-- Migration 013: Add gender column to users and customers tables
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS gender TEXT;
-- yember-end-step
-- yember-step: 014_enforce_unique_email_and_phone.sql/001
-- Migration 014: Enforce strict uniqueness for non-empty email and phone across users and customers

-- Clean up any empty strings to NULL to prevent empty string uniqueness collisions
UPDATE public.users SET email = NULL WHERE email = '';
-- yember-end-step
-- yember-step: 014_enforce_unique_email_and_phone.sql/002

UPDATE public.users SET phone = NULL WHERE phone = '';
-- yember-end-step
-- yember-step: 014_enforce_unique_email_and_phone.sql/005


-- Drop old non-partial or partial indexes if any exist
DROP INDEX IF EXISTS public.idx_users_email_unique;
-- yember-end-step
-- yember-step: 014_enforce_unique_email_and_phone.sql/006

DROP INDEX IF EXISTS public.idx_users_phone_unique;
-- yember-end-step
-- yember-step: 014_enforce_unique_email_and_phone.sql/009

DROP INDEX IF EXISTS public.idx_users_phone;
-- yember-end-step
-- yember-step: 014_enforce_unique_email_and_phone.sql/011


-- Create unique partial indexes for public.users
CREATE UNIQUE INDEX idx_users_email_unique ON public.users (LOWER(email)) WHERE email IS NOT NULL AND email != '';
-- yember-end-step
-- yember-step: 014_enforce_unique_email_and_phone.sql/012

CREATE UNIQUE INDEX idx_users_phone_unique ON public.users (phone) WHERE phone IS NOT NULL AND phone != '';
-- yember-end-step
-- yember-step: 015_customer_checkout.sql/002


UPDATE public.users SET role = CASE LOWER(role)
    WHEN 'customer' THEN 'CUSTOMER'
    WHEN 'admin' THEN 'ADMIN'
    WHEN 'merchant_owner' THEN 'MERCHANT_OWNER'
    WHEN 'restaurant_owner' THEN 'MERCHANT_OWNER'
    WHEN 'delivery_partner' THEN 'DELIVERY_PARTNER'
    WHEN 'support_agent' THEN 'SUPPORT_AGENT'
    ELSE role
END
WHERE LOWER(role) IN ('customer','admin','merchant_owner','restaurant_owner','delivery_partner','support_agent');
-- yember-end-step
-- yember-step: 017_password_reset_tokens.sql/001
CREATE TABLE IF NOT EXISTS public.password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 017_password_reset_tokens.sql/002


CREATE INDEX IF NOT EXISTS password_reset_tokens_user_created_idx
    ON public.password_reset_tokens(user_id, created_at DESC);

-- yember-end-step
-- yember-step: 021_platform_operations.sql/001
-- Additive migration. Approval does not imply payment or activation.
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMPTZ;
-- yember-end-step
-- yember-step: 021_platform_operations.sql/002

ALTER TABLE public.users ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMPTZ;
-- yember-end-step
-- yember-step: 021_platform_operations.sql/011


CREATE TABLE IF NOT EXISTS auth.refresh_sessions (
    token_hash TEXT PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 021_platform_operations.sql/012

CREATE INDEX IF NOT EXISTS refresh_sessions_user ON auth.refresh_sessions(user_id);

-- yember-end-step
-- yember-step: 022_partner_onboarding.sql/001
CREATE TABLE IF NOT EXISTS auth.email_verification_tokens (
    token_hash TEXT PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ
);
-- yember-end-step
-- yember-step: 037_finance_operations.sql/003

ALTER TABLE public.users ADD COLUMN tokens_invalid_before timestamptz NOT NULL DEFAULT '-infinity';
-- yember-end-step
-- yember-step: 038_auth_and_operations.sql/001
CREATE TABLE auth.admin_mfa_steps(user_id uuid PRIMARY KEY REFERENCES public.users(id),last_step bigint NOT NULL);
-- yember-end-step
-- yember-step: 038_auth_and_operations.sql/002

ALTER TABLE auth.otp_codes ADD COLUMN failed_attempts integer NOT NULL DEFAULT 0;
-- yember-end-step
-- yember-step: 043_secure_otp.sql/001
-- Existing plaintext development challenges must not remain usable.
DELETE FROM auth.otp_codes;

-- yember-end-step
-- yember-step: 044_rate_limits.sql/001
CREATE TABLE auth.rate_windows(scope text NOT NULL,client text NOT NULL,window_start timestamptz NOT NULL,count integer NOT NULL,PRIMARY KEY(scope,client,window_start));
-- yember-end-step
-- yember-step: 044_rate_limits.sql/002

CREATE INDEX rate_window_expiry ON auth.rate_windows(window_start);

-- yember-end-step
-- yember-step: 050_refresh_families.sql/001
ALTER TABLE auth.refresh_sessions ADD COLUMN family_id uuid NOT NULL DEFAULT gen_random_uuid();
-- yember-end-step
-- yember-step: 050_refresh_families.sql/002

ALTER TABLE auth.refresh_sessions ADD COLUMN family_expires_at timestamptz NOT NULL DEFAULT now()+interval '30 days';
-- yember-end-step
-- yember-step: 050_refresh_families.sql/003

ALTER TABLE auth.refresh_sessions ADD COLUMN parent_hash text;
-- yember-end-step
-- yember-step: 050_refresh_families.sql/004

CREATE INDEX refresh_family ON auth.refresh_sessions(family_id);

-- yember-end-step
-- yember-step: 20240101_create_auth.sql/001
-- Removed duplicate auth migration (handled by 001_init.sql)

-- yember-end-step

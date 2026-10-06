-- Migration 014: Enforce strict uniqueness for non-empty email and phone across users and customers

-- Clean up any empty strings to NULL to prevent empty string uniqueness collisions
UPDATE public.users SET email = NULL WHERE email = '';
UPDATE public.users SET phone = NULL WHERE phone = '';
UPDATE public.customers SET email = NULL WHERE email = '';
UPDATE public.customers SET phone = NULL WHERE phone = '';

-- Drop old non-partial or partial indexes if any exist
DROP INDEX IF EXISTS public.idx_users_email_unique;
DROP INDEX IF EXISTS public.idx_users_phone_unique;
DROP INDEX IF EXISTS public.idx_customers_email_unique;
DROP INDEX IF EXISTS public.idx_customers_phone_unique;
DROP INDEX IF EXISTS public.idx_users_phone;
DROP INDEX IF EXISTS public.idx_customers_phone;

-- Create unique partial indexes for public.users
CREATE UNIQUE INDEX idx_users_email_unique ON public.users (LOWER(email)) WHERE email IS NOT NULL AND email != '';
CREATE UNIQUE INDEX idx_users_phone_unique ON public.users (phone) WHERE phone IS NOT NULL AND phone != '';

-- Create unique partial indexes for public.customers
CREATE UNIQUE INDEX idx_customers_email_unique ON public.customers (LOWER(email)) WHERE email IS NOT NULL AND email != '';
CREATE UNIQUE INDEX idx_customers_phone_unique ON public.customers (phone) WHERE phone IS NOT NULL AND phone != '';

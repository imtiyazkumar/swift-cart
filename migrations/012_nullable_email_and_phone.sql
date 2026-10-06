-- Migration 012: Allow nullable email for phone-authenticated users
ALTER TABLE public.users ALTER COLUMN email DROP NOT NULL;
ALTER TABLE public.customers ALTER COLUMN email DROP NOT NULL;

-- Clean up any placeholder emails generated for phone signups
UPDATE public.users SET email = NULL WHERE email LIKE '%@yember.app' AND phone IS NOT NULL AND phone != '';
UPDATE public.customers SET email = NULL WHERE email LIKE '%@yember.app' AND phone IS NOT NULL AND phone != '';

-- Create unique partial index on phone number when present
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_phone ON public.users(phone) WHERE phone IS NOT NULL AND phone != '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_customers_phone ON public.customers(phone) WHERE phone IS NOT NULL AND phone != '';

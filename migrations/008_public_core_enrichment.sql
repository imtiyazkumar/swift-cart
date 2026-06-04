ALTER TABLE public.users
    ADD COLUMN IF NOT EXISTS phone TEXT,
    ADD COLUMN IF NOT EXISTS full_name TEXT,
    ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

ALTER TABLE public.customers
    ADD COLUMN IF NOT EXISTS preferences JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS wallet_balance BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

ALTER TABLE public.merchants
    ADD COLUMN IF NOT EXISTS merchant_type TEXT NOT NULL DEFAULT 'RESTAURANT',
    ADD COLUMN IF NOT EXISTS onboarding_status TEXT NOT NULL DEFAULT 'PROFILE_CREATED',
    ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'PENDING',
    ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

ALTER TABLE public.orders
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT,
    ADD COLUMN IF NOT EXISTS tax_amount BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS platform_fee BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS delivery_fee BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS coupon_code TEXT,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_idempotency_key ON public.orders(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON public.users(deleted_at);
CREATE INDEX IF NOT EXISTS idx_customers_deleted_at ON public.customers(deleted_at);
CREATE INDEX IF NOT EXISTS idx_merchants_type ON public.merchants(merchant_type);

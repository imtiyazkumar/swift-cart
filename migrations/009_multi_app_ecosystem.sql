-- Migration 009: Multi-App Ecosystem Extensions

-- Drop unused inventory schema if present
DROP SCHEMA IF EXISTS inventory CASCADE;

-- Add owner_id and is_active to merchants table
ALTER TABLE public.merchants
    ADD COLUMN IF NOT EXISTS owner_id UUID REFERENCES public.users(id),
    ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;

-- Add user_id reference to customers table
ALTER TABLE public.customers
    ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES public.users(id);

-- Coupons table for promo code discounts
CREATE TABLE IF NOT EXISTS orders.coupons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE,
    description TEXT,
    discount_type TEXT NOT NULL CHECK (discount_type IN ('PERCENTAGE', 'FLAT')),
    discount_value BIGINT NOT NULL CHECK (discount_value > 0),
    min_order_amount BIGINT NOT NULL DEFAULT 0,
    max_discount_amount BIGINT,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_until TIMESTAMPTZ,
    usage_limit INTEGER,
    used_count INTEGER NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Customer Favorite Restaurants
CREATE TABLE IF NOT EXISTS users.customer_favorites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES public.customers(id) ON DELETE CASCADE,
    merchant_id UUID NOT NULL REFERENCES public.merchants(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(customer_id, merchant_id)
);

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

-- Rider Earnings Log
CREATE TABLE IF NOT EXISTS delivery.rider_earnings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id UUID NOT NULL REFERENCES delivery.delivery_partners(id) ON DELETE CASCADE,
    order_id UUID NOT NULL REFERENCES public.orders(id) ON DELETE CASCADE,
    delivery_fee BIGINT NOT NULL DEFAULT 0,
    tip_amount BIGINT NOT NULL DEFAULT 0,
    earned_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

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

-- Performance Indexes
CREATE INDEX IF NOT EXISTS idx_coupons_code ON orders.coupons(code) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_customer_favorites_customer ON users.customer_favorites(customer_id);
CREATE INDEX IF NOT EXISTS idx_restaurant_reviews_merchant ON merchants.restaurant_reviews(merchant_id);
CREATE INDEX IF NOT EXISTS idx_rider_earnings_partner ON delivery.rider_earnings(partner_id, earned_at DESC);
CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_admin ON public.admin_audit_logs(admin_id, created_at DESC);

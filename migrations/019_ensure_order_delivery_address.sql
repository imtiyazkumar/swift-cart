-- Checkout stores the selected address as a snapshot on each order.
-- Keep this as a standalone repair migration for databases that missed 015.
ALTER TABLE public.orders
    ADD COLUMN IF NOT EXISTS delivery_address JSONB NOT NULL DEFAULT '{}'::jsonb;

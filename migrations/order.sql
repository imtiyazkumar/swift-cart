-- Domain: order
-- Applied through cmd/migrate using manifest.json; do not execute this file directly.
-- yember-step: 005_orders.sql/001
-- 005_orders.sql
CREATE TABLE IF NOT EXISTS orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    total_price NUMERIC(12,2) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);

-- yember-end-step
-- yember-step: 006_domain_schemas.sql/004

CREATE SCHEMA IF NOT EXISTS orders;
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/010


DO $$ BEGIN
    CREATE TYPE order_status AS ENUM ('CREATED','PAYMENT_PENDING','CONFIRMED','PREPARING','READY_FOR_PICKUP','PICKED_UP','OUT_FOR_DELIVERY','DELIVERED','CANCELLED','REFUNDED');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/021


CREATE TABLE IF NOT EXISTS orders.carts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES public.customers(id) ON DELETE CASCADE,
    merchant_id UUID NOT NULL REFERENCES public.merchants(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(customer_id, merchant_id, status)
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/022


CREATE TABLE IF NOT EXISTS orders.cart_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cart_id UUID NOT NULL REFERENCES orders.carts(id) ON DELETE CASCADE,
    item_id UUID NOT NULL REFERENCES catalog.items(id),
    variant_id UUID REFERENCES catalog.item_variants(id),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price BIGINT NOT NULL CHECK (unit_price >= 0),
    metadata JSONB NOT NULL DEFAULT '{}'
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/023


CREATE TABLE IF NOT EXISTS orders.order_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES public.orders(id) ON DELETE CASCADE,
    item_id UUID REFERENCES catalog.items(id),
    name_snapshot TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price BIGINT NOT NULL CHECK (unit_price >= 0),
    total_price BIGINT NOT NULL CHECK (total_price >= 0),
    metadata JSONB NOT NULL DEFAULT '{}'
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/024


CREATE TABLE IF NOT EXISTS orders.order_status_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES public.orders(id) ON DELETE CASCADE,
    status order_status NOT NULL,
    notes TEXT,
    changed_by UUID REFERENCES public.users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/037

CREATE INDEX IF NOT EXISTS idx_orders_customer ON public.orders(customer_id);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/038

CREATE INDEX IF NOT EXISTS idx_orders_merchant ON public.orders(merchant_id);
-- yember-end-step
-- yember-step: 008_public_core_enrichment.sql/004


ALTER TABLE public.orders
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT,
    ADD COLUMN IF NOT EXISTS tax_amount BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS platform_fee BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS delivery_fee BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS coupon_code TEXT,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
-- yember-end-step
-- yember-step: 008_public_core_enrichment.sql/005


CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_idempotency_key ON public.orders(idempotency_key) WHERE idempotency_key IS NOT NULL;
-- yember-end-step
-- yember-step: 009_multi_app_ecosystem.sql/004


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
-- yember-end-step
-- yember-step: 009_multi_app_ecosystem.sql/009


-- Performance Indexes
CREATE INDEX IF NOT EXISTS idx_coupons_code ON orders.coupons(code) WHERE is_active = true;
-- yember-end-step
-- yember-step: 015_customer_checkout.sql/001
ALTER TABLE public.orders
    ADD COLUMN IF NOT EXISTS delivery_address JSONB NOT NULL DEFAULT '{}'::jsonb;
-- yember-end-step
-- yember-step: 015_customer_checkout.sql/003


CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_customer_idempotency
    ON public.orders(customer_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
-- yember-end-step
-- yember-step: 015_customer_checkout.sql/004


WITH duplicates AS (
    SELECT ctid, cart_id, item_id, quantity,
           ROW_NUMBER() OVER (PARTITION BY cart_id, item_id ORDER BY ctid) AS row_num,
           SUM(quantity) OVER (PARTITION BY cart_id, item_id) AS total_quantity
    FROM orders.cart_items
)
UPDATE orders.cart_items ci SET quantity=d.total_quantity
FROM duplicates d WHERE ci.ctid=d.ctid AND d.row_num=1;
-- yember-end-step
-- yember-step: 015_customer_checkout.sql/005


WITH duplicates AS (
    SELECT ctid, ROW_NUMBER() OVER (PARTITION BY cart_id, item_id ORDER BY ctid) AS row_num
    FROM orders.cart_items
)
DELETE FROM orders.cart_items ci USING duplicates d WHERE ci.ctid=d.ctid AND d.row_num>1;
-- yember-end-step
-- yember-step: 015_customer_checkout.sql/006


CREATE UNIQUE INDEX IF NOT EXISTS idx_cart_items_cart_item ON orders.cart_items(cart_id,item_id);

-- yember-end-step
-- yember-step: 018_repair_cart_item_conflict_index.sql/001
-- Ensure checkout's ON CONFLICT (cart_id, item_id) target exists even on
-- databases that were migrated before the cart-item index was introduced.
WITH duplicates AS (
    SELECT ctid,
           ROW_NUMBER() OVER (PARTITION BY cart_id, item_id ORDER BY ctid) AS row_num,
           SUM(quantity) OVER (PARTITION BY cart_id, item_id) AS total_quantity
    FROM orders.cart_items
)
UPDATE orders.cart_items AS item
SET quantity = duplicates.total_quantity
FROM duplicates
WHERE item.ctid = duplicates.ctid
  AND duplicates.row_num = 1;
-- yember-end-step
-- yember-step: 018_repair_cart_item_conflict_index.sql/002


WITH duplicates AS (
    SELECT ctid,
           ROW_NUMBER() OVER (PARTITION BY cart_id, item_id ORDER BY ctid) AS row_num
    FROM orders.cart_items
)
DELETE FROM orders.cart_items AS item
USING duplicates
WHERE item.ctid = duplicates.ctid
  AND duplicates.row_num > 1;
-- yember-end-step
-- yember-step: 018_repair_cart_item_conflict_index.sql/003


CREATE UNIQUE INDEX IF NOT EXISTS idx_cart_items_cart_item
    ON orders.cart_items (cart_id, item_id);

-- yember-end-step
-- yember-step: 019_ensure_order_delivery_address.sql/001
-- Checkout stores the selected address as a snapshot on each order.
-- Keep this as a standalone repair migration for databases that missed 015.
ALTER TABLE public.orders
    ADD COLUMN IF NOT EXISTS delivery_address JSONB NOT NULL DEFAULT '{}'::jsonb;

-- yember-end-step
-- yember-step: 023_order_lifecycle.sql/001
ALTER TYPE order_status ADD VALUE IF NOT EXISTS 'ACCEPTED';
-- yember-end-step
-- yember-step: 023_order_lifecycle.sql/002

ALTER TYPE order_status ADD VALUE IF NOT EXISTS 'REJECTED';

-- yember-end-step
-- yember-step: 024_delivery_offers.sql/008

CREATE INDEX IF NOT EXISTS dispatch_order_status ON public.orders(status,created_at);
-- yember-end-step
-- yember-step: 025_order_amount_compatibility.sql/001
-- The legacy customer checkout stores major units in total_price; operations
-- APIs use integer minor units. Keep both representations consistent.
ALTER TABLE public.orders ADD COLUMN IF NOT EXISTS total_amount BIGINT NOT NULL DEFAULT 0;
-- yember-end-step
-- yember-step: 025_order_amount_compatibility.sql/002

ALTER TABLE public.orders ADD COLUMN IF NOT EXISTS currency CHAR(3) NOT NULL DEFAULT 'INR';
-- yember-end-step
-- yember-step: 025_order_amount_compatibility.sql/003

UPDATE public.orders SET total_amount=round(total_price*100)::bigint;
-- yember-end-step
-- yember-step: 025_order_amount_compatibility.sql/004

CREATE OR REPLACE FUNCTION public.sync_order_minor_amount() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.total_amount := round(NEW.total_price*100)::bigint;
    RETURN NEW;
END;
$$;
-- yember-end-step
-- yember-step: 025_order_amount_compatibility.sql/005

DROP TRIGGER IF EXISTS sync_order_minor_amount ON public.orders;
-- yember-end-step
-- yember-step: 025_order_amount_compatibility.sql/006

CREATE TRIGGER sync_order_minor_amount BEFORE INSERT OR UPDATE OF total_price ON public.orders
    FOR EACH ROW EXECUTE FUNCTION public.sync_order_minor_amount();

-- yember-end-step
-- yember-step: 029_order_notifications.sql/002

DROP TRIGGER IF EXISTS record_order_status_notification ON orders.order_status_history;
-- yember-end-step
-- yember-step: 029_order_notifications.sql/003

CREATE TRIGGER record_order_status_notification AFTER INSERT ON orders.order_status_history
    FOR EACH ROW EXECUTE FUNCTION notifications.record_order_status();

-- yember-end-step
-- yember-step: 033_operational_stations.sql/004

ALTER TABLE public.orders ADD COLUMN station_id uuid REFERENCES public.stations(id);
-- yember-end-step
-- yember-step: 033_operational_stations.sql/005

ALTER TABLE orders.carts ADD COLUMN delivery_address_id uuid REFERENCES users.customer_addresses(id);
-- yember-end-step
-- yember-step: 033_operational_stations.sql/021


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
-- yember-end-step
-- yember-step: 033_operational_stations.sql/022

CREATE TRIGGER order_station_guard BEFORE INSERT ON public.orders FOR EACH ROW EXECUTE FUNCTION public.guard_order_station();
-- yember-end-step
-- yember-step: 034_station_relations.sql/009


CREATE INDEX order_station_history ON public.orders(station_id,created_at DESC);
-- yember-end-step
-- yember-step: 034_station_relations.sql/014


CREATE FUNCTION public.guard_order_station_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.station_id IS NOT NULL AND (NEW.station_id IS DISTINCT FROM OLD.station_id OR NEW.merchant_id IS DISTINCT FROM OLD.merchant_id) THEN
        RAISE EXCEPTION 'An order station is historical and cannot be reassigned.';
    END IF;
    RETURN NEW;
END $$;
-- yember-end-step
-- yember-step: 034_station_relations.sql/015

CREATE TRIGGER order_station_snapshot BEFORE UPDATE OF station_id,merchant_id ON public.orders FOR EACH ROW EXECUTE FUNCTION public.guard_order_station_snapshot();
-- yember-end-step
-- yember-step: 034_station_relations.sql/022


-- A later recovery of a genuinely known legacy order station repairs its children.
CREATE FUNCTION public.propagate_order_station() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.station_id IS NULL AND NEW.station_id IS NOT NULL THEN
        UPDATE delivery.offers SET station_id=NEW.station_id WHERE order_id=NEW.id AND station_id IS NULL;
        UPDATE delivery.delivery_assignments SET station_id=NEW.station_id WHERE order_id=NEW.id AND station_id IS NULL;
        UPDATE delivery.rider_earnings SET station_id=NEW.station_id WHERE order_id=NEW.id AND station_id IS NULL;
        UPDATE public.partner_support_tickets SET station_id=NEW.station_id WHERE order_id=NEW.id AND station_id IS NULL;
    END IF;
    RETURN NEW;
END $$;
-- yember-end-step
-- yember-step: 034_station_relations.sql/023

CREATE TRIGGER recovered_order_station AFTER UPDATE OF station_id ON public.orders FOR EACH ROW EXECUTE FUNCTION public.propagate_order_station();

-- yember-end-step
-- yember-step: 040_finance_refund_jobs.sql/003

CREATE TRIGGER queue_cancellation_refund AFTER UPDATE OF status ON public.orders FOR EACH ROW EXECUTE FUNCTION finance.queue_cancellation_refund();

-- yember-end-step
-- yember-step: 042_push_delivery.sql/005

CREATE TRIGGER order_push AFTER INSERT OR UPDATE OF status ON public.orders FOR EACH ROW EXECUTE FUNCTION notifications.order_push();
-- yember-end-step

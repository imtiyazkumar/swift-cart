CREATE SCHEMA IF NOT EXISTS inventory;

CREATE TABLE IF NOT EXISTS inventory.warehouses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID REFERENCES public.merchants(id),
    name TEXT NOT NULL,
    warehouse_type TEXT NOT NULL DEFAULT 'BRANCH',
    address TEXT,
    latitude NUMERIC(10,7),
    longitude NUMERIC(10,7),
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS inventory.stock_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    warehouse_id UUID NOT NULL REFERENCES inventory.warehouses(id) ON DELETE CASCADE,
    item_id UUID NOT NULL REFERENCES catalog.items(id) ON DELETE CASCADE,
    variant_id UUID REFERENCES catalog.item_variants(id) ON DELETE CASCADE,
    available_quantity INTEGER NOT NULL DEFAULT 0 CHECK (available_quantity >= 0),
    reserved_quantity INTEGER NOT NULL DEFAULT 0 CHECK (reserved_quantity >= 0),
    expires_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(warehouse_id, item_id, variant_id)
);

CREATE TABLE IF NOT EXISTS inventory.stock_reservations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES public.orders(id) ON DELETE CASCADE,
    stock_item_id UUID NOT NULL REFERENCES inventory.stock_items(id) ON DELETE CASCADE,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    status TEXT NOT NULL DEFAULT 'HELD',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS inventory.substitution_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES public.customers(id) ON DELETE CASCADE,
    item_id UUID NOT NULL REFERENCES catalog.items(id) ON DELETE CASCADE,
    preference TEXT NOT NULL DEFAULT 'CONTACT_ME',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(customer_id, item_id)
);

CREATE INDEX IF NOT EXISTS idx_inventory_stock_item ON inventory.stock_items(item_id);
CREATE INDEX IF NOT EXISTS idx_inventory_reservation_order ON inventory.stock_reservations(order_id);

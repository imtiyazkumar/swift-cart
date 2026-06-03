-- 005_orders.sql
CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID NOT NULL REFERENCES merchants.merchants(id) ON DELETE CASCADE,
    customer_id UUID NOT NULL REFERENCES users.customers(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    total_price NUMERIC(12,2) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);

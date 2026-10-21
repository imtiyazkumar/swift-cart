ALTER TABLE public.orders
    ADD COLUMN IF NOT EXISTS delivery_address JSONB NOT NULL DEFAULT '{}'::jsonb;

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

CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_customer_idempotency
    ON public.orders(customer_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

WITH duplicates AS (
    SELECT ctid, cart_id, item_id, quantity,
           ROW_NUMBER() OVER (PARTITION BY cart_id, item_id ORDER BY ctid) AS row_num,
           SUM(quantity) OVER (PARTITION BY cart_id, item_id) AS total_quantity
    FROM orders.cart_items
)
UPDATE orders.cart_items ci SET quantity=d.total_quantity
FROM duplicates d WHERE ci.ctid=d.ctid AND d.row_num=1;

WITH duplicates AS (
    SELECT ctid, ROW_NUMBER() OVER (PARTITION BY cart_id, item_id ORDER BY ctid) AS row_num
    FROM orders.cart_items
)
DELETE FROM orders.cart_items ci USING duplicates d WHERE ci.ctid=d.ctid AND d.row_num>1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_cart_items_cart_item ON orders.cart_items(cart_id,item_id);

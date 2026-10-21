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

WITH duplicates AS (
    SELECT ctid,
           ROW_NUMBER() OVER (PARTITION BY cart_id, item_id ORDER BY ctid) AS row_num
    FROM orders.cart_items
)
DELETE FROM orders.cart_items AS item
USING duplicates
WHERE item.ctid = duplicates.ctid
  AND duplicates.row_num > 1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_cart_items_cart_item
    ON orders.cart_items (cart_id, item_id);

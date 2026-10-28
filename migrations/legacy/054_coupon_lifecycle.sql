-- Preserve codes and immutable redemption history after an administrator deletes a coupon.
ALTER TABLE orders.coupons ADD COLUMN deleted_at timestamptz;
CREATE UNIQUE INDEX coupons_code_case_insensitive ON orders.coupons (upper(code));
CREATE INDEX coupon_redemptions_coupon ON finance.coupon_redemptions(coupon_id, created_at DESC);
ALTER TABLE orders.coupons ADD CONSTRAINT coupon_usage_limit_positive CHECK (usage_limit IS NULL OR usage_limit > 0);
ALTER TABLE orders.coupons ADD CONSTRAINT coupon_window_valid CHECK (valid_until IS NULL OR valid_until > valid_from);

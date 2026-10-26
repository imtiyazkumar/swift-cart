ALTER TABLE finance.entries DROP CONSTRAINT entries_account_check;
ALTER TABLE finance.entries ADD CONSTRAINT entries_account_check CHECK(account IN ('CASH','UNEARNED','PARTNER_PAYABLE','COMMISSION','DELIVERY_REVENUE','TAX_PAYABLE','GATEWAY_FEE','REFUND_EXPENSE','ADJUSTMENT','PAYOUT_CLEARING','PROMOTION_EXPENSE'));
CREATE TABLE finance.coupon_redemptions(order_id uuid PRIMARY KEY REFERENCES finance.order_financials(order_id),coupon_id uuid NOT NULL REFERENCES orders.coupons(id),amount bigint NOT NULL CHECK(amount>0),funded_by text NOT NULL DEFAULT 'PLATFORM' CHECK(funded_by='PLATFORM'),created_at timestamptz NOT NULL DEFAULT now());
CREATE TRIGGER coupon_history_guard BEFORE UPDATE OR DELETE ON finance.coupon_redemptions FOR EACH ROW EXECUTE FUNCTION finance.immutable();

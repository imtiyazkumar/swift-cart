ALTER TABLE finance.entries DROP CONSTRAINT entries_account_check;
ALTER TABLE finance.entries ADD CONSTRAINT entries_account_check CHECK(account IN ('CASH','UNEARNED','PARTNER_PAYABLE','COMMISSION','DELIVERY_REVENUE','TAX_PAYABLE','GATEWAY_FEE','REFUND_EXPENSE','ADJUSTMENT','PAYOUT_CLEARING','PROMOTION_EXPENSE','ONBOARDING_REVENUE','CASH_IN_TRANSIT','RESTAURANT_FEE_RECEIVABLE'));
CREATE TABLE finance.cash_handovers (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
 order_id uuid NOT NULL UNIQUE REFERENCES finance.cod_custody(order_id),
 amount_paise bigint NOT NULL CHECK(amount_paise>0),
 status text NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','PAID','DISPUTED')),
 submitted_by uuid NOT NULL REFERENCES public.users(id),
 confirmed_by uuid REFERENCES public.users(id),
 confirmed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 deleted_at timestamptz,
 CHECK((status='PAID')=(confirmed_by IS NOT NULL AND confirmed_at IS NOT NULL))
);
CREATE INDEX cash_handovers_status ON finance.cash_handovers(status,created_at);

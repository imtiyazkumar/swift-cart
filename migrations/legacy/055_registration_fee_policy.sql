ALTER TABLE finance.onboarding_attempts ALTER COLUMN merchant_id DROP NOT NULL;
ALTER TABLE finance.onboarding_attempts ADD COLUMN delivery_agent_id uuid REFERENCES delivery.delivery_partners(id);
ALTER TABLE finance.onboarding_attempts ADD CONSTRAINT registration_applicant_exclusive CHECK ((merchant_id IS NOT NULL)::int + (delivery_agent_id IS NOT NULL)::int = 1);
CREATE UNIQUE INDEX one_agent_registration_paid ON finance.onboarding_attempts(delivery_agent_id) WHERE status='PAID';
INSERT INTO public.platform_settings(key,value) VALUES
 ('onboarding_fee','{"amount":20000,"currency":"INR","terms":"Restaurant registration fee of INR 200. Non-refundable. Payment does not imply approval."}'),
 ('delivery_registration_fee','{"amount":15000,"currency":"INR","terms":"Delivery agent registration fee of INR 150. Non-refundable. Payment does not imply approval."}')
ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=now();

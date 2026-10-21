ALTER TABLE merchants.restaurant_reviews ADD COLUMN IF NOT EXISTS hidden_at TIMESTAMPTZ;
ALTER TABLE merchants.restaurant_reviews ADD COLUMN IF NOT EXISTS hidden_by UUID REFERENCES public.users(id);
ALTER TABLE merchants.restaurant_reviews ADD COLUMN IF NOT EXISTS moderation_reason TEXT NOT NULL DEFAULT '';

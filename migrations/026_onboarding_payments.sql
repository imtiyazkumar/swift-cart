CREATE TABLE IF NOT EXISTS payments.onboarding_payments (
    merchant_id UUID PRIMARY KEY REFERENCES public.merchants(id),
    session_id TEXT NOT NULL UNIQUE,
    checkout_url TEXT NOT NULL,
    amount BIGINT NOT NULL CHECK (amount>0),
    currency CHAR(3) NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PAID')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS payments.onboarding_events (
    event_id TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

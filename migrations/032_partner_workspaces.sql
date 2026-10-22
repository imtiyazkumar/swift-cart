CREATE TABLE IF NOT EXISTS public.partner_support_tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES public.users(id),
    order_id UUID REFERENCES public.orders(id),
    category TEXT NOT NULL CHECK (category IN ('ORDER','ACCOUNT','EARNINGS','MENU','OTHER')),
    subject TEXT NOT NULL CHECK (length(subject) BETWEEN 3 AND 160),
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','IN_PROGRESS','RESOLVED')),
    request_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id,request_key)
);
CREATE TABLE IF NOT EXISTS public.partner_support_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id UUID NOT NULL REFERENCES public.partner_support_tickets(id),
    author_id UUID NOT NULL REFERENCES public.users(id),
    body TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 4000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS partner_support_owner ON public.partner_support_tickets(user_id,created_at DESC);
CREATE INDEX IF NOT EXISTS partner_support_thread ON public.partner_support_messages(ticket_id,created_at);
ALTER TABLE merchants.restaurant_reviews ADD COLUMN IF NOT EXISTS reply TEXT NOT NULL DEFAULT '';
ALTER TABLE merchants.restaurant_reviews ADD COLUMN IF NOT EXISTS replied_at TIMESTAMPTZ;

ALTER TABLE delivery.delivery_partners ADD COLUMN IF NOT EXISTS last_offered_at TIMESTAMPTZ;
ALTER TABLE delivery.delivery_partners ADD COLUMN IF NOT EXISTS eligibility_expires_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS delivery.offers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES public.orders(id),
    partner_id UUID NOT NULL REFERENCES delivery.delivery_partners(id),
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','ACCEPTED','REJECTED','EXPIRED')),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(order_id,partner_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS one_pending_offer_per_order ON delivery.offers(order_id) WHERE status='PENDING';
CREATE UNIQUE INDEX IF NOT EXISTS one_pending_offer_per_partner ON delivery.offers(partner_id) WHERE status='PENDING';
-- Existing conflicting assignments must be reviewed by an operator; never delete them automatically.
CREATE UNIQUE INDEX IF NOT EXISTS one_active_assignment_per_order ON delivery.delivery_assignments(order_id) WHERE status IN ('ASSIGNED','PICKED_UP','OUT_FOR_DELIVERY');
CREATE UNIQUE INDEX IF NOT EXISTS one_active_assignment_per_partner ON delivery.delivery_assignments(partner_id) WHERE status IN ('ASSIGNED','PICKED_UP','OUT_FOR_DELIVERY');
CREATE INDEX IF NOT EXISTS dispatch_order_status ON public.orders(status,created_at);
CREATE INDEX IF NOT EXISTS offer_expiration ON delivery.offers(expires_at) WHERE status='PENDING';

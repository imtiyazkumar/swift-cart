CREATE UNIQUE INDEX IF NOT EXISTS one_restaurant_review_per_order ON merchants.restaurant_reviews(order_id,customer_id) WHERE order_id IS NOT NULL;
CREATE TABLE IF NOT EXISTS delivery.partner_ratings (
    order_id UUID PRIMARY KEY REFERENCES public.orders(id),
    partner_id UUID NOT NULL REFERENCES delivery.delivery_partners(id),
    customer_id UUID NOT NULL REFERENCES public.customers(id),
    rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

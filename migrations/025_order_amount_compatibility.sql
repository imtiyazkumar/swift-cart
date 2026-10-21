-- The legacy customer checkout stores major units in total_price; operations
-- APIs use integer minor units. Keep both representations consistent.
ALTER TABLE public.orders ADD COLUMN IF NOT EXISTS total_amount BIGINT NOT NULL DEFAULT 0;
ALTER TABLE public.orders ADD COLUMN IF NOT EXISTS currency CHAR(3) NOT NULL DEFAULT 'INR';
UPDATE public.orders SET total_amount=round(total_price*100)::bigint;
CREATE OR REPLACE FUNCTION public.sync_order_minor_amount() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.total_amount := round(NEW.total_price*100)::bigint;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS sync_order_minor_amount ON public.orders;
CREATE TRIGGER sync_order_minor_amount BEFORE INSERT OR UPDATE OF total_price ON public.orders
    FOR EACH ROW EXECUTE FUNCTION public.sync_order_minor_amount();

-- Remove the unused legacy auth implementation. Active sessions use the rotated,
-- revocable hash-only refresh_sessions relation, renamed below.
DROP TABLE auth.refresh_tokens;
DROP TABLE auth.sessions;
ALTER TABLE auth.refresh_sessions SET SCHEMA public;
ALTER TABLE public.refresh_sessions RENAME TO refresh_tokens;
ALTER TABLE auth.otp_codes SET SCHEMA public;
ALTER TABLE public.otp_codes RENAME TO otp_challenges;
ALTER TABLE public.otp_challenges RENAME COLUMN otp_code TO otp_hash;
ALTER TABLE users.customer_addresses SET SCHEMA public;
ALTER TABLE public.customer_addresses RENAME TO user_addresses;
ALTER TABLE orders.carts SET SCHEMA public;
ALTER TABLE orders.cart_items SET SCHEMA public;
ALTER TABLE orders.order_items SET SCHEMA public;
ALTER TABLE orders.order_status_history SET SCHEMA public;
ALTER TABLE delivery.delivery_partners SET SCHEMA public;
ALTER TABLE public.delivery_partners RENAME TO riders;
ALTER TABLE delivery.delivery_assignments SET SCHEMA public;
ALTER TABLE delivery.live_locations SET SCHEMA public;
ALTER TABLE public.live_locations RENAME TO rider_locations;

DO $$
DECLARE f record; source text; mapping text[]; mappings text[][]:=ARRAY[
 ['auth.refresh_sessions','public.refresh_tokens'],['auth.otp_codes','public.otp_challenges'],
 ['users.customer_addresses','public.user_addresses'],['orders.carts','public.carts'],
 ['orders.cart_items','public.cart_items'],['orders.order_items','public.order_items'],
 ['orders.order_status_history','public.order_status_history'],['delivery.delivery_partners','public.riders'],
 ['delivery.delivery_assignments','public.delivery_assignments'],['delivery.live_locations','public.rider_locations']];
BEGIN
 FOR f IN SELECT p.prosrc,pg_get_functiondef(p.oid) AS definition
          FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
          WHERE n.nspname IN ('public','auth','catalog','orders','delivery','finance','payments','notifications','restaurants','users') AND p.prokind='f'
 LOOP
  source:=f.prosrc;
  FOREACH mapping SLICE 1 IN ARRAY mappings LOOP source:=replace(source,mapping[1],mapping[2]);END LOOP;
  IF source<>f.prosrc THEN EXECUTE replace(f.definition,f.prosrc,source);END IF;
 END LOOP;
END $$;

ALTER SCHEMA merchants RENAME TO restaurants;
ALTER TABLE restaurants.merchant_branches RENAME TO restaurant_branches;
ALTER TABLE restaurants.merchant_operating_hours RENAME TO restaurant_operating_hours;
UPDATE public.users SET role='RESTAURANT_OWNER' WHERE role='MERCHANT_OWNER';

DO $$
DECLARE f record; source text;
BEGIN
 FOR f IN SELECT p.prosrc,pg_get_functiondef(p.oid) AS definition
          FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
          WHERE n.nspname IN ('public','auth','catalog','orders','delivery','finance','payments','notifications','restaurants','users')
          AND p.prokind='f' AND (p.prosrc LIKE '%merchants.%' OR p.prosrc LIKE '%MERCHANT_OWNER%')
 LOOP
  source:=replace(replace(replace(replace(f.prosrc,'merchants.','restaurants.'),'merchant_branches','restaurant_branches'),'merchant_operating_hours','restaurant_operating_hours'),'MERCHANT_OWNER','RESTAURANT_OWNER');
  EXECUTE replace(f.definition,f.prosrc,source);
 END LOOP;
END $$;

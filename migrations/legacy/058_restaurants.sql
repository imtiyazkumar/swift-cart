-- Rename the existing entity in place; UUIDs, rows and FK relationships survive.
ALTER TABLE public.merchants RENAME TO restaurants;

-- PostgreSQL tracks FK dependencies across a rename, but PL/pgSQL SQL text is
-- stored as source. Rebind only application functions that mention this table.
DO $$
DECLARE f record; source text;
BEGIN
 FOR f IN SELECT p.oid,p.prosrc,pg_get_functiondef(p.oid) AS definition
          FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
          WHERE n.nspname IN ('public','auth','catalog','orders','delivery','finance','notifications','merchants','users')
          AND p.prokind='f' AND p.prosrc LIKE '%public.merchants%'
 LOOP
  source := replace(f.prosrc,'public.merchants','public.restaurants');
  EXECUTE replace(f.definition,f.prosrc,source);
 END LOOP;
END $$;

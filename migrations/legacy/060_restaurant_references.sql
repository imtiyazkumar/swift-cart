-- Use restaurant_id consistently across all existing restaurant relationships.
DO $$
DECLARE c record; f record; source text;
BEGIN
 FOR c IN SELECT table_schema,table_name FROM information_schema.columns
          WHERE column_name='merchant_id' AND table_schema IN ('public','merchants','catalog','orders','finance','payments','delivery','notifications','users')
 LOOP
  EXECUTE format('ALTER TABLE %I.%I RENAME COLUMN merchant_id TO restaurant_id',c.table_schema,c.table_name);
 END LOOP;
 FOR f IN SELECT p.prosrc,pg_get_functiondef(p.oid) AS definition
          FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
          WHERE n.nspname IN ('public','auth','catalog','orders','delivery','finance','payments','notifications','merchants','users')
          AND p.prokind='f' AND p.prosrc LIKE '%merchant_id%'
 LOOP
  source:=replace(f.prosrc,'merchant_id','restaurant_id');
  EXECUTE replace(f.definition,f.prosrc,source);
 END LOOP;
END $$;

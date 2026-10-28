ALTER TABLE catalog.categories SET SCHEMA public;
ALTER TABLE public.categories RENAME TO menu_categories;
ALTER TABLE public.menu_categories DROP COLUMN merchant_id;
ALTER TABLE public.menu_categories ADD COLUMN description text NOT NULL DEFAULT '';
ALTER TABLE public.menu_categories ADD COLUMN image_url text;
ALTER TABLE catalog.items SET SCHEMA public;
ALTER TABLE public.items RENAME TO menu_items;
ALTER TABLE catalog.item_variants SET SCHEMA public;

DO $$
DECLARE f record; source text;
BEGIN
 FOR f IN SELECT p.prosrc,pg_get_functiondef(p.oid) AS definition
          FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
          WHERE n.nspname IN ('public','auth','catalog','orders','delivery','finance','notifications','merchants','users')
          AND p.prokind='f' AND (p.prosrc LIKE '%catalog.items%' OR p.prosrc LIKE '%catalog.categories%' OR p.prosrc LIKE '%catalog.item_variants%')
 LOOP
  source := replace(replace(replace(f.prosrc,'catalog.items','public.menu_items'),'catalog.categories','public.menu_categories'),'catalog.item_variants','public.item_variants');
  EXECUTE replace(f.definition,f.prosrc,source);
 END LOOP;
END $$;

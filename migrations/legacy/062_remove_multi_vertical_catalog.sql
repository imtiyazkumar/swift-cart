DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM public.restaurants WHERE merchant_type<>'RESTAURANT') THEN
  RAISE EXCEPTION 'Non-restaurant business records must be archived with their financial references before this restaurant-only migration';
 END IF;
END $$;
ALTER TABLE public.restaurants DROP COLUMN merchant_type;
-- This old unused enum also included the removed support-agent role.
DROP TYPE public.user_role;

-- An absent schedule preserves existing manually managed availability.
-- Configured hours use the operating market's Asia/Kolkata timezone; overnight shifts use the previous day's record.
CREATE FUNCTION public.restaurant_open_at(merchant uuid,instant timestamptz) RETURNS boolean LANGUAGE sql STABLE AS $$
 WITH local AS (SELECT instant AT TIME ZONE 'Asia/Kolkata' AS ts),
 hours AS (SELECT h.* FROM merchants.merchant_operating_hours h JOIN merchants.merchant_branches b ON b.id=h.branch_id WHERE b.merchant_id=merchant AND b.deleted_at IS NULL)
 SELECT NOT EXISTS(SELECT 1 FROM hours) OR EXISTS(
  SELECT 1 FROM hours,local WHERE NOT is_closed AND (
   (day_of_week=extract(dow FROM ts)::int AND opens_at<closes_at AND ts::time>=opens_at AND ts::time<closes_at)
   OR (opens_at>closes_at AND ((day_of_week=extract(dow FROM ts)::int AND ts::time>=opens_at) OR (day_of_week=(extract(dow FROM ts)::int+6)%7 AND ts::time<closes_at)))
  )
 )
$$;

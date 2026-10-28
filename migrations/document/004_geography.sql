CREATE OR REPLACE FUNCTION station_covers(station bigint,latitude numeric,longitude numeric) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT COALESCE((SELECT s.status='ACTIVE' AND s.deleted_at IS NULL AND 6371*2*asin(sqrt(least(1::double precision,power(sin(radians(($2-s.latitude)::double precision)/2),2)+cos(radians($2::double precision))*cos(radians(s.latitude::double precision))*power(sin(radians(($3-s.longitude)::double precision)/2),2))))<=s.radius_km FROM stations s WHERE s.id=$1),false)
$$;

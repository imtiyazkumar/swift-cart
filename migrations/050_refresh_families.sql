ALTER TABLE auth.refresh_sessions ADD COLUMN family_id uuid NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE auth.refresh_sessions ADD COLUMN family_expires_at timestamptz NOT NULL DEFAULT now()+interval '30 days';
ALTER TABLE auth.refresh_sessions ADD COLUMN parent_hash text;
CREATE INDEX refresh_family ON auth.refresh_sessions(family_id);

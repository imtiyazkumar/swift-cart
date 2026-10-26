CREATE TABLE auth.admin_mfa_steps(user_id uuid PRIMARY KEY REFERENCES public.users(id),last_step bigint NOT NULL);
ALTER TABLE auth.otp_codes ADD COLUMN failed_attempts integer NOT NULL DEFAULT 0;
CREATE TABLE users.notification_preferences(user_id uuid PRIMARY KEY REFERENCES public.users(id),order_updates boolean NOT NULL DEFAULT true,marketing boolean NOT NULL DEFAULT false,updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE users.lifecycle_requests(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),user_id uuid NOT NULL REFERENCES public.users(id),kind text NOT NULL CHECK(kind IN ('EXPORT','DELETE')),status text NOT NULL DEFAULT 'PENDING',reason text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now(),resolved_at timestamptz);
CREATE UNIQUE INDEX one_pending_lifecycle ON users.lifecycle_requests(user_id,kind) WHERE status='PENDING';

-- Domain: notification
-- Applied through cmd/migrate using manifest.json; do not execute this file directly.
-- yember-step: 006_domain_schemas.sql/007

CREATE SCHEMA IF NOT EXISTS notifications;
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/033


CREATE TABLE IF NOT EXISTS notifications.notification_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES public.users(id),
    channel TEXT NOT NULL,
    template_key TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/034


CREATE TABLE IF NOT EXISTS notifications.push_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    UNIQUE(provider, token_hash)
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/040

CREATE INDEX IF NOT EXISTS idx_notifications_user_created ON notifications.notification_logs(user_id, created_at DESC);

-- yember-end-step
-- yember-step: 021_platform_operations.sql/009


CREATE TABLE IF NOT EXISTS notifications.email_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','SENDING','SENT','FAILED')),
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at TIMESTAMPTZ
);
-- yember-end-step
-- yember-step: 021_platform_operations.sql/010

CREATE INDEX IF NOT EXISTS email_outbox_pending ON notifications.email_outbox(next_attempt_at) WHERE status IN ('PENDING','SENDING');
-- yember-end-step
-- yember-step: 029_order_notifications.sql/001
CREATE OR REPLACE FUNCTION notifications.record_order_status() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO notifications.notification_logs(user_id,channel,template_key,payload,status)
    SELECT DISTINCT recipient,'IN_APP','order_status',jsonb_build_object(
        'title','Order status update','message','Order ' || left(NEW.order_id::text,8) || ': ' || NEW.status::text,
        'order_id',NEW.order_id,'order_status',NEW.status),'UNREAD'
    FROM (
        SELECT u.id AS recipient FROM public.orders o JOIN public.customers c ON c.id=o.customer_id
        JOIN public.users u ON u.id=COALESCE(c.user_id,c.id) WHERE o.id=NEW.order_id
        UNION
        SELECT m.owner_id FROM public.orders o JOIN public.merchants m ON m.id=o.merchant_id WHERE o.id=NEW.order_id
        UNION
        SELECT p.user_id FROM delivery.delivery_assignments a JOIN delivery.delivery_partners p ON p.id=a.partner_id WHERE a.order_id=NEW.order_id
    ) recipients WHERE recipient IS NOT NULL;
    RETURN NEW;
END;
$$;
-- yember-end-step
-- yember-step: 038_auth_and_operations.sql/003

CREATE TABLE users.notification_preferences(user_id uuid PRIMARY KEY REFERENCES public.users(id),order_updates boolean NOT NULL DEFAULT true,marketing boolean NOT NULL DEFAULT false,updated_at timestamptz NOT NULL DEFAULT now());
-- yember-end-step
-- yember-step: 042_push_delivery.sql/001
CREATE TABLE notifications.devices(token text PRIMARY KEY,user_id uuid NOT NULL REFERENCES public.users(id),app text NOT NULL CHECK(app IN ('customer','restaurant','delivery')),updated_at timestamptz NOT NULL DEFAULT now(),disabled_at timestamptz);
-- yember-end-step
-- yember-step: 042_push_delivery.sql/002

CREATE TABLE notifications.push_outbox(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),user_id uuid NOT NULL REFERENCES public.users(id),event_key text NOT NULL UNIQUE,title text NOT NULL,body text NOT NULL,data jsonb NOT NULL DEFAULT '{}',status text NOT NULL DEFAULT 'PENDING',attempts integer NOT NULL DEFAULT 0,next_attempt_at timestamptz NOT NULL DEFAULT now(),last_error text,created_at timestamptz NOT NULL DEFAULT now());
-- yember-end-step
-- yember-step: 042_push_delivery.sql/003

CREATE TABLE notifications.push_tickets(ticket_id text PRIMARY KEY,token text NOT NULL REFERENCES notifications.devices(token),outbox_id uuid NOT NULL REFERENCES notifications.push_outbox(id),checked_at timestamptz,created_at timestamptz NOT NULL DEFAULT now());
-- yember-end-step
-- yember-step: 042_push_delivery.sql/004

CREATE FUNCTION notifications.order_push() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' OR NEW.status IS DISTINCT FROM OLD.status THEN
  INSERT INTO notifications.push_outbox(user_id,event_key,title,body,data) VALUES(NEW.customer_id,'order/'||NEW.id||'/'||NEW.status,'Yember order update','Your order status is '||NEW.status,jsonb_build_object('order_id',NEW.id)) ON CONFLICT DO NOTHING;
  IF NEW.status IN ('CREATED','CONFIRMED') THEN
   INSERT INTO notifications.push_outbox(user_id,event_key,title,body,data) SELECT owner_id,'restaurant/'||NEW.id||'/'||NEW.status,'New Yember order','Open Yember Restaurant to review the new order.',jsonb_build_object('order_id',NEW.id) FROM public.merchants WHERE id=NEW.merchant_id ON CONFLICT DO NOTHING;
  END IF;
 END IF;
 RETURN NEW;
END $$;
-- yember-end-step
-- yember-step: 042_push_delivery.sql/006

CREATE FUNCTION notifications.offer_push() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO notifications.push_outbox(user_id,event_key,title,body,data) SELECT user_id,'offer/'||NEW.id,'New delivery offer','Open Yember Delivery to respond before this offer expires.',jsonb_build_object('order_id',NEW.order_id) FROM delivery.delivery_partners WHERE id=NEW.partner_id ON CONFLICT DO NOTHING;
 RETURN NEW;
END $$;
-- yember-end-step
-- yember-step: 051_textbee_sms.sql/001
CREATE TABLE notifications.sms_outbox(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),challenge_id uuid REFERENCES auth.otp_codes(id) ON DELETE SET NULL,recipient text NOT NULL,body text NOT NULL,provider_reference text,status text NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','SENDING','QUEUED','DELIVERED','FAILED','UNKNOWN','EXPIRED')),next_attempt_at timestamptz NOT NULL DEFAULT now(),expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL DEFAULT now());

-- yember-end-step

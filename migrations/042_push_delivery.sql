CREATE TABLE notifications.devices(token text PRIMARY KEY,user_id uuid NOT NULL REFERENCES public.users(id),app text NOT NULL CHECK(app IN ('customer','restaurant','delivery')),updated_at timestamptz NOT NULL DEFAULT now(),disabled_at timestamptz);
CREATE TABLE notifications.push_outbox(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),user_id uuid NOT NULL REFERENCES public.users(id),event_key text NOT NULL UNIQUE,title text NOT NULL,body text NOT NULL,data jsonb NOT NULL DEFAULT '{}',status text NOT NULL DEFAULT 'PENDING',attempts integer NOT NULL DEFAULT 0,next_attempt_at timestamptz NOT NULL DEFAULT now(),last_error text,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE notifications.push_tickets(ticket_id text PRIMARY KEY,token text NOT NULL REFERENCES notifications.devices(token),outbox_id uuid NOT NULL REFERENCES notifications.push_outbox(id),checked_at timestamptz,created_at timestamptz NOT NULL DEFAULT now());
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
CREATE TRIGGER order_push AFTER INSERT OR UPDATE OF status ON public.orders FOR EACH ROW EXECUTE FUNCTION notifications.order_push();
CREATE FUNCTION notifications.offer_push() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO notifications.push_outbox(user_id,event_key,title,body,data) SELECT user_id,'offer/'||NEW.id,'New delivery offer','Open Yember Delivery to respond before this offer expires.',jsonb_build_object('order_id',NEW.order_id) FROM delivery.delivery_partners WHERE id=NEW.partner_id ON CONFLICT DO NOTHING;
 RETURN NEW;
END $$;
CREATE TRIGGER offer_push AFTER INSERT ON delivery.offers FOR EACH ROW EXECUTE FUNCTION notifications.offer_push();

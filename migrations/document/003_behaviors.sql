-- Current behavior definitions. Safe to reapply to an existing installation.

CREATE OR REPLACE FUNCTION touch_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN NEW.updated_at=now(); RETURN NEW; END $$;

CREATE OR REPLACE FUNCTION prevent_history_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'financial and activity history is append-only'; END $$;

CREATE OR REPLACE FUNCTION sanitize_activity_json(value jsonb) RETURNS jsonb LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE result jsonb; entry record;
BEGIN
 IF jsonb_typeof(value)='object' THEN
  result='{}'::jsonb;
  FOR entry IN SELECT * FROM jsonb_each(value) LOOP
   IF entry.key~*'(password|token|hash|secret|encrypted|account_holder|account_number|bank_name|ifsc|phone|email|name|address|latitude|longitude|file_url|payload|user_agent|ip_address|before_json|after_json)' THEN CONTINUE; END IF;
   IF jsonb_typeof(entry.value)='number' AND (entry.key='id' OR entry.key~'_id$') THEN CONTINUE; END IF;
   result=result||jsonb_build_object(entry.key,sanitize_activity_json(entry.value));
  END LOOP;
  RETURN result;
 ELSIF jsonb_typeof(value)='array' THEN
  SELECT COALESCE(jsonb_agg(sanitize_activity_json(element)),'[]') INTO result FROM jsonb_array_elements(value) AS items(element);
  RETURN result;
 END IF;
 RETURN value;
END $$;

CREATE OR REPLACE FUNCTION record_system_activity() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE actor bigint; actor_role text; previous jsonb; current jsonb; row_data jsonb; reason text;
BEGIN
 actor=NULLIF(current_setting('yember.actor_id',true),'')::bigint;
 SELECT role INTO actor_role FROM users WHERE id=actor;
 IF TG_OP<>'INSERT' THEN previous=to_jsonb(OLD); END IF;
 IF TG_OP<>'DELETE' THEN current=to_jsonb(NEW); END IF;
 row_data=COALESCE(current,previous);
 previous=sanitize_activity_json(COALESCE(previous,'{}'));
 current=sanitize_activity_json(COALESCE(current,'{}'));
 reason=NULLIF(current_setting('yember.reason',true),'');
 IF reason IS NOT NULL THEN current=current||jsonb_build_object('change_reason',reason); END IF;
 INSERT INTO system_activity_logs(actor_type,actor_id,action,entity_type,entity_id,entity_uuid,before_json,after_json,request_id,ip_address,occurred_at)
 VALUES(CASE actor_role WHEN 'ADMIN' THEN 'ADMIN' WHEN 'RESTAURANT_OWNER' THEN 'RESTAURANT' WHEN 'DELIVERY_PARTNER' THEN 'AGENT' WHEN 'CUSTOMER' THEN 'USER' ELSE 'SYSTEM' END,actor,COALESCE(NULLIF(current_setting('yember.action',true),''),TG_OP),upper(TG_TABLE_NAME),(row_data->>'id')::bigint,(row_data->>'uuid')::uuid,previous,current,NULLIF(current_setting('yember.request_id',true),''),NULLIF(current_setting('yember.ip_address',true),'')::inet,now());
 RETURN COALESCE(NEW,OLD);
END $$;

CREATE OR REPLACE FUNCTION validate_menu_reference() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE restaurant bigint; item_restaurant bigint; selected_item bigint;
BEGIN
 IF TG_TABLE_NAME='cart_items' THEN SELECT restaurant_id INTO restaurant FROM carts WHERE id=NEW.cart_id;
 ELSE SELECT restaurant_id INTO restaurant FROM orders WHERE id=NEW.order_id; END IF;
 SELECT restaurant_id INTO item_restaurant FROM menu_items WHERE id=NEW.menu_item_id;
 IF restaurant IS DISTINCT FROM item_restaurant THEN RAISE EXCEPTION 'menu item belongs to another restaurant'; END IF;
 IF NEW.variant_id IS NOT NULL THEN
  SELECT menu_item_id INTO selected_item FROM item_variants WHERE id=NEW.variant_id;
  IF selected_item IS DISTINCT FROM NEW.menu_item_id THEN RAISE EXCEPTION 'variant belongs to another menu item'; END IF;
 END IF;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION protect_historical_address() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM orders WHERE address_id=OLD.id) AND (to_jsonb(NEW)-'updated_at'-'deleted_at'-'is_default') IS DISTINCT FROM (to_jsonb(OLD)-'updated_at'-'deleted_at'-'is_default') THEN RAISE EXCEPTION 'create a new address version for changes to an address used by an order'; END IF;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION record_order_status_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' THEN
  INSERT INTO order_status_history(order_id,from_status,to_status,changed_by,changed_at,note)
  VALUES(NEW.id,NULL,NEW.status,NULLIF(current_setting('yember.actor_id',true),'')::bigint,now(),NULLIF(current_setting('yember.reason',true),''));
 ELSIF NEW.status IS DISTINCT FROM OLD.status THEN
  INSERT INTO order_status_history(order_id,from_status,to_status,changed_by,changed_at,note)
  VALUES(NEW.id,OLD.status,NEW.status,NULLIF(current_setting('yember.actor_id',true),'')::bigint,now(),NULLIF(current_setting('yember.reason',true),''));
 END IF;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION check_balanced_financial_event() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE event text;
BEGIN
 event=regexp_replace(NEW.event_key,':[^:]+$','');
 IF (SELECT COALESCE(sum(CASE direction WHEN 'DEBIT' THEN amount_paise ELSE -amount_paise END),0) FROM financial_transactions WHERE regexp_replace(event_key,':[^:]+$','')=event)<>0 THEN RAISE EXCEPTION 'unbalanced financial event'; END IF;
 RETURN NULL;
END $$;

CREATE OR REPLACE FUNCTION protect_order_line_terms() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE old_name text; new_name text; old_variant text; new_variant text;
BEGIN
 IF NEW.order_id IS DISTINCT FROM OLD.order_id OR NEW.quantity IS DISTINCT FROM OLD.quantity OR NEW.unit_price IS DISTINCT FROM OLD.unit_price OR NEW.special_instructions IS DISTINCT FROM OLD.special_instructions THEN RAISE EXCEPTION 'placed order line terms are immutable'; END IF;
 IF NEW.menu_item_id IS DISTINCT FROM OLD.menu_item_id THEN
  SELECT name INTO old_name FROM menu_items WHERE id=OLD.menu_item_id;
  SELECT name INTO new_name FROM menu_items WHERE id=NEW.menu_item_id;
  IF old_name IS DISTINCT FROM new_name THEN RAISE EXCEPTION 'historical menu label must be preserved'; END IF;
 END IF;
 IF NEW.variant_id IS DISTINCT FROM OLD.variant_id THEN
  SELECT name INTO old_variant FROM item_variants WHERE id=OLD.variant_id;
  SELECT name INTO new_variant FROM item_variants WHERE id=NEW.variant_id;
  IF old_variant IS DISTINCT FROM new_variant THEN RAISE EXCEPTION 'historical variant label must be preserved'; END IF;
 END IF;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION version_referenced_menu() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE archived bigint; variant record; archived_variant bigint;
BEGIN
 IF (NEW.name,NEW.description,NEW.base_price,NEW.category_id,NEW.restaurant_id) IS NOT DISTINCT FROM (OLD.name,OLD.description,OLD.base_price,OLD.category_id,OLD.restaurant_id) THEN RETURN NEW; END IF;
 IF NOT EXISTS(SELECT 1 FROM order_items WHERE menu_item_id=OLD.id) THEN RETURN NEW; END IF;
 IF NEW.restaurant_id<>OLD.restaurant_id THEN RAISE EXCEPTION 'restaurant ownership of an ordered item is immutable'; END IF;
 INSERT INTO menu_items(restaurant_id,category_id,name,description,base_price,image_url,preparation_minutes,is_available,created_at,updated_at,deleted_at)
 VALUES(OLD.restaurant_id,OLD.category_id,OLD.name,OLD.description,OLD.base_price,OLD.image_url,OLD.preparation_minutes,false,OLD.created_at,OLD.updated_at,now()) RETURNING id INTO archived;
 FOR variant IN SELECT DISTINCT v.* FROM item_variants v JOIN order_items line ON line.variant_id=v.id WHERE line.menu_item_id=OLD.id LOOP
  INSERT INTO item_variants(menu_item_id,name,price_delta,is_available,created_at,updated_at,deleted_at)
  VALUES(archived,variant.name,variant.price_delta,false,variant.created_at,variant.updated_at,now()) RETURNING id INTO archived_variant;
  UPDATE order_items SET menu_item_id=archived,variant_id=archived_variant WHERE menu_item_id=OLD.id AND variant_id=variant.id;
 END LOOP;
 UPDATE order_items SET menu_item_id=archived WHERE menu_item_id=OLD.id AND variant_id IS NULL;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION version_referenced_variant() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE archived bigint;
BEGIN
 IF (NEW.name,NEW.price_delta,NEW.menu_item_id) IS NOT DISTINCT FROM (OLD.name,OLD.price_delta,OLD.menu_item_id) THEN RETURN NEW; END IF;
 IF NOT EXISTS(SELECT 1 FROM order_items WHERE variant_id=OLD.id) THEN RETURN NEW; END IF;
 INSERT INTO item_variants(menu_item_id,name,price_delta,is_available,created_at,updated_at,deleted_at)
 VALUES(OLD.menu_item_id,OLD.name,OLD.price_delta,false,OLD.created_at,OLD.updated_at,now()) RETURNING id INTO archived;
 UPDATE order_items SET variant_id=archived WHERE variant_id=OLD.id;
 RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS immutable_financial_transactions ON financial_transactions;
CREATE TRIGGER immutable_financial_transactions BEFORE UPDATE OR DELETE ON financial_transactions FOR EACH ROW EXECUTE FUNCTION prevent_history_mutation();
DROP TRIGGER IF EXISTS immutable_activity_logs ON system_activity_logs;
CREATE TRIGGER immutable_activity_logs BEFORE UPDATE OR DELETE ON system_activity_logs FOR EACH ROW EXECUTE FUNCTION prevent_history_mutation();
DROP TRIGGER IF EXISTS cart_reference_guard ON cart_items;
CREATE TRIGGER cart_reference_guard BEFORE INSERT OR UPDATE ON cart_items FOR EACH ROW EXECUTE FUNCTION validate_menu_reference();
DROP TRIGGER IF EXISTS order_reference_guard ON order_items;
CREATE TRIGGER order_reference_guard BEFORE INSERT OR UPDATE ON order_items FOR EACH ROW EXECUTE FUNCTION validate_menu_reference();
DROP TRIGGER IF EXISTS address_history_guard ON user_addresses;
CREATE TRIGGER address_history_guard BEFORE UPDATE ON user_addresses FOR EACH ROW EXECUTE FUNCTION protect_historical_address();
DROP TRIGGER IF EXISTS order_status_history ON orders;
CREATE TRIGGER order_status_history AFTER INSERT OR UPDATE OF status ON orders FOR EACH ROW EXECUTE FUNCTION record_order_status_history();
DROP TRIGGER IF EXISTS balanced_financial_event ON financial_transactions;
CREATE CONSTRAINT TRIGGER balanced_financial_event AFTER INSERT ON financial_transactions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_balanced_financial_event();
DROP TRIGGER IF EXISTS order_terms_guard ON order_items;
CREATE TRIGGER order_terms_guard BEFORE UPDATE ON order_items FOR EACH ROW EXECUTE FUNCTION protect_order_line_terms();
DROP TRIGGER IF EXISTS menu_version_history ON menu_items;
CREATE TRIGGER menu_version_history BEFORE UPDATE ON menu_items FOR EACH ROW EXECUTE FUNCTION version_referenced_menu();
DROP TRIGGER IF EXISTS variant_version_history ON item_variants;
CREATE TRIGGER variant_version_history BEFORE UPDATE ON item_variants FOR EACH ROW EXECUTE FUNCTION version_referenced_variant();
DROP TRIGGER IF EXISTS immutable_coupon_usage ON coupon_usages;
CREATE TRIGGER immutable_coupon_usage BEFORE UPDATE OR DELETE ON coupon_usages FOR EACH ROW EXECUTE FUNCTION prevent_history_mutation();
DROP TRIGGER IF EXISTS immutable_order_status_history ON order_status_history;
CREATE TRIGGER immutable_order_status_history BEFORE UPDATE OR DELETE ON order_status_history FOR EACH ROW EXECUTE FUNCTION prevent_history_mutation();

DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['users','otp_challenges','refresh_tokens','user_addresses','stations','restaurants','restaurant_documents','restaurant_bank_accounts','menu_categories','menu_items','item_variants','carts','cart_items','orders','order_items','order_status_history','riders','rider_documents','delivery_agent_bank_details','delivery_assignments','rider_locations','payment_intents','payments','registration_fees','restaurant_earnings','delivery_agent_earnings','restaurant_payouts','delivery_agent_payouts','restaurant_payout_allocations','delivery_agent_payout_allocations','cash_collections','financial_transactions','payment_webhook_events','refunds','reconciliation_runs','coupons','coupon_usages'] LOOP
  EXECUTE format('DROP TRIGGER IF EXISTS update_timestamp ON %I',t);
  IF t<>'financial_transactions' THEN EXECUTE format('CREATE TRIGGER update_timestamp BEFORE UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION touch_updated_at()',t); END IF;
  EXECUTE format('DROP TRIGGER IF EXISTS activity_history ON %I',t);
  EXECUTE format('CREATE TRIGGER activity_history AFTER INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION record_system_activity()',t);
 END LOOP;
END $$;

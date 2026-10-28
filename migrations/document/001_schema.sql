-- Canonical schema generated from the two architecture documents.
-- Overrides: created_at/updated_at; coupons and coupon_usages are explicitly requested.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  full_name varchar(160),
  email varchar(255) UNIQUE,
  phone varchar(30) UNIQUE,
  password_hash text,
  role varchar(30),
  status varchar(30),
  phone_verified_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE otp_challenges (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  user_id bigint,
  phone varchar(30),
  purpose varchar(30),
  otp_hash text,
  expires_at timestamptz,
  attempts int,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE refresh_tokens (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  user_id bigint NOT NULL,
  token_hash varchar(128) NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  replaced_by_token_id bigint,
  last_used_at timestamptz,
  device_label varchar(120),
  user_agent text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE user_addresses (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  user_id bigint,
  label varchar(50),
  contact_name varchar(160),
  contact_phone varchar(30),
  address_line1 text,
  address_line2 text,
  city varchar(120),
  latitude numeric(10,7),
  longitude numeric(10,7),
  is_default boolean,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE stations (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  name varchar(160),
  description text,
  latitude numeric(10,7),
  longitude numeric(10,7),
  radius_km numeric(8,2),
  status varchar(20),
  created_by bigint,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE restaurants (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  owner_user_id bigint,
  station_id bigint,
  name varchar(200),
  description text,
  phone varchar(30),
  address text,
  latitude numeric(10,7),
  longitude numeric(10,7),
  image_url text,
  fssai_number varchar(100),
  status varchar(30),
  is_open boolean,
  approved_by bigint,
  approved_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE restaurant_documents (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  restaurant_id bigint,
  document_type varchar(50),
  file_url text,
  verification_status varchar(30),
  verified_by bigint,
  verified_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE restaurant_bank_accounts (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  restaurant_id bigint,
  account_holder varchar(160),
  account_number_encrypted text,
  bank_name varchar(160),
  ifsc varchar(30),
  verification_status varchar(30),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE menu_categories (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  image_url text,
  name varchar(120),
  description text,
  sort_order int,
  is_active boolean,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE menu_items (
  restaurant_id bigint NOT NULL,
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  category_id bigint,
  name varchar(180),
  description text,
  base_price numeric(12,2),
  image_url text,
  preparation_minutes int,
  is_available boolean,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE item_variants (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  menu_item_id bigint,
  name varchar(120),
  price_delta numeric(12,2),
  is_available boolean,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE carts (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  user_id bigint,
  restaurant_id bigint,
  status varchar(20),
  expires_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE cart_items (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  cart_id bigint,
  menu_item_id bigint,
  variant_id bigint,
  quantity int,
  special_instructions text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE orders (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  order_number varchar(40) UNIQUE,
  user_id bigint,
  restaurant_id bigint,
  address_id bigint,
  status varchar(30),
  subtotal numeric(12,2),
  delivery_fee numeric(12,2),
  tax_amount numeric(12,2),
  discount_amount numeric(12,2),
  total_amount numeric(12,2),
  currency char(3),
  placed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE order_items (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  order_id bigint,
  menu_item_id bigint,
  variant_id bigint,
  quantity int,
  unit_price numeric(12,2),
  special_instructions text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE order_status_history (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  order_id bigint,
  from_status varchar(30),
  to_status varchar(30),
  changed_by bigint,
  changed_at timestamptz,
  note text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE riders (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  user_id bigint UNIQUE,
  station_id bigint,
  vehicle_type varchar(50),
  verification_status varchar(30),
  availability_status varchar(30),
  approved_by bigint,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE rider_documents (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  rider_id bigint,
  document_type varchar(50),
  file_url text,
  verification_status varchar(30),
  verified_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE delivery_agent_bank_details (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  rider_id bigint,
  account_holder varchar(160),
  account_number_encrypted text,
  bank_name varchar(160),
  ifsc varchar(30),
  verification_status varchar(30),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE delivery_assignments (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  order_id bigint,
  rider_id bigint,
  status varchar(30),
  assigned_at timestamptz,
  accepted_at timestamptz,
  picked_up_at timestamptz,
  delivered_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE rider_locations (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  rider_id bigint,
  latitude numeric(10,7),
  longitude numeric(10,7),
  recorded_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE TABLE payment_intents (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  order_id bigint,
  restaurant_id bigint,
  delivery_agent_id bigint,
  purpose varchar(32),
  method varchar(24),
  stage varchar(24),
  amount_paise bigint,
  currency char(3),
  status varchar(24),
  provider_order_id varchar(120),
  idempotency_key varchar(120),
  expires_at timestamptz
);

CREATE TABLE payments (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  payment_intent_id bigint,
  order_id bigint,
  provider varchar(24),
  provider_payment_id varchar(120),
  method varchar(32),
  amount_paise bigint,
  currency char(3),
  status varchar(24),
  received_by varchar(24),
  captured_at timestamptz
);

CREATE TABLE registration_fees (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  restaurant_id bigint,
  delivery_agent_id bigint,
  amount_paise bigint,
  currency char(3),
  payment_intent_id bigint,
  status varchar(24),
  paid_at timestamptz
);

CREATE TABLE restaurant_earnings (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  restaurant_id bigint,
  order_id bigint,
  gross_amount_paise bigint,
  fee_amount_paise bigint,
  adjustment_amount_paise bigint,
  net_amount_paise bigint,
  status varchar(24),
  eligible_at timestamptz
);

CREATE TABLE delivery_agent_earnings (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  delivery_agent_id bigint,
  order_id bigint,
  base_amount_paise bigint,
  incentive_amount_paise bigint,
  adjustment_amount_paise bigint,
  net_amount_paise bigint,
  status varchar(24),
  eligible_at timestamptz
);

CREATE TABLE restaurant_payouts (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  restaurant_id bigint,
  delivery_agent_id bigint,
  order_id bigint,
  method varchar(32),
  amount_paise bigint,
  status varchar(24),
  provider_transfer_id varchar(120),
  confirmed_by_restaurant_at timestamptz,
  paid_at timestamptz
);

CREATE TABLE delivery_agent_payouts (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  delivery_agent_id bigint,
  bank_details_id bigint,
  method varchar(24),
  amount_paise bigint,
  status varchar(24),
  provider_transfer_id varchar(120),
  paid_at timestamptz
);

CREATE TABLE restaurant_payout_allocations (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  restaurant_payout_id bigint,
  restaurant_earning_id bigint,
  amount_paise bigint
);

CREATE TABLE delivery_agent_payout_allocations (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  delivery_agent_payout_id bigint,
  delivery_agent_earning_id bigint,
  amount_paise bigint
);

CREATE TABLE cash_collections (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  order_id bigint,
  delivery_agent_id bigint,
  restaurant_id bigint,
  amount_due_paise bigint,
  amount_collected_paise bigint,
  collected_at timestamptz,
  restaurant_payout_id bigint,
  status varchar(24)
);

CREATE TABLE financial_transactions (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  order_id bigint,
  payment_id bigint,
  restaurant_earning_id bigint,
  agent_earning_id bigint,
  restaurant_payout_id bigint,
  agent_payout_id bigint,
  registration_fee_id bigint,
  transaction_type varchar(48),
  account_code varchar(48),
  direction varchar(8),
  amount_paise bigint,
  currency char(3),
  event_key varchar(160),
  occurred_at timestamptz
);

CREATE TABLE payment_webhook_events (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  provider varchar(24),
  provider_event_id varchar(160),
  event_type varchar(100),
  payload_json jsonb,
  signature_valid boolean,
  processing_status varchar(24),
  processed_at timestamptz
);

CREATE TABLE refunds (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  payment_id bigint,
  order_id bigint,
  amount_paise bigint,
  reason varchar(255),
  provider_refund_id varchar(120),
  status varchar(24),
  completed_at timestamptz
);

CREATE TABLE reconciliation_runs (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  source varchar(32),
  period_start timestamptz,
  period_end timestamptz,
  status varchar(24),
  difference_paise bigint,
  summary_json jsonb
);

CREATE TABLE system_activity_logs (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  actor_type varchar(32),
  actor_id bigint,
  action varchar(32),
  entity_type varchar(64),
  entity_id bigint,
  entity_uuid uuid,
  before_json jsonb,
  after_json jsonb,
  request_id varchar(120),
  ip_address inet,
  user_agent text,
  occurred_at timestamptz
);

CREATE TABLE coupons (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  code varchar(50) NOT NULL UNIQUE,
  description text,
  discount_type varchar(16) NOT NULL,
  discount_value bigint NOT NULL,
  min_order_amount_paise bigint NOT NULL DEFAULT 0,
  max_discount_amount_paise bigint,
  valid_from timestamptz NOT NULL DEFAULT now(),
  valid_until timestamptz,
  usage_limit integer,
  is_active boolean NOT NULL DEFAULT true
);

CREATE TABLE coupon_usages (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  coupon_id bigint NOT NULL,
  user_id bigint NOT NULL,
  order_id bigint NOT NULL UNIQUE,
  discount_amount_paise bigint NOT NULL,
  redeemed_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE otp_challenges ADD FOREIGN KEY (user_id) REFERENCES users(id);
CREATE INDEX otp_challenges_user_id_idx ON otp_challenges(user_id);
ALTER TABLE refresh_tokens ADD FOREIGN KEY (user_id) REFERENCES users(id);
CREATE INDEX refresh_tokens_user_id_idx ON refresh_tokens(user_id);
ALTER TABLE refresh_tokens ADD FOREIGN KEY (replaced_by_token_id) REFERENCES refresh_tokens(id);
CREATE INDEX refresh_tokens_replaced_by_token_id_idx ON refresh_tokens(replaced_by_token_id);
ALTER TABLE user_addresses ADD FOREIGN KEY (user_id) REFERENCES users(id);
CREATE INDEX user_addresses_user_id_idx ON user_addresses(user_id);
ALTER TABLE stations ADD FOREIGN KEY (created_by) REFERENCES users(id);
CREATE INDEX stations_created_by_idx ON stations(created_by);
ALTER TABLE restaurants ADD FOREIGN KEY (owner_user_id) REFERENCES users(id);
CREATE INDEX restaurants_owner_user_id_idx ON restaurants(owner_user_id);
ALTER TABLE restaurants ADD FOREIGN KEY (station_id) REFERENCES stations(id);
CREATE INDEX restaurants_station_id_idx ON restaurants(station_id);
ALTER TABLE restaurants ADD FOREIGN KEY (approved_by) REFERENCES users(id);
CREATE INDEX restaurants_approved_by_idx ON restaurants(approved_by);
ALTER TABLE restaurant_documents ADD FOREIGN KEY (restaurant_id) REFERENCES restaurants(id);
CREATE INDEX restaurant_documents_restaurant_id_idx ON restaurant_documents(restaurant_id);
ALTER TABLE restaurant_documents ADD FOREIGN KEY (verified_by) REFERENCES users(id);
CREATE INDEX restaurant_documents_verified_by_idx ON restaurant_documents(verified_by);
ALTER TABLE restaurant_bank_accounts ADD FOREIGN KEY (restaurant_id) REFERENCES restaurants(id);
CREATE INDEX restaurant_bank_accounts_restaurant_id_idx ON restaurant_bank_accounts(restaurant_id);
ALTER TABLE menu_items ADD FOREIGN KEY (restaurant_id) REFERENCES restaurants(id);
CREATE INDEX menu_items_restaurant_id_idx ON menu_items(restaurant_id);
ALTER TABLE menu_items ADD FOREIGN KEY (category_id) REFERENCES menu_categories(id);
CREATE INDEX menu_items_category_id_idx ON menu_items(category_id);
ALTER TABLE item_variants ADD FOREIGN KEY (menu_item_id) REFERENCES menu_items(id);
CREATE INDEX item_variants_menu_item_id_idx ON item_variants(menu_item_id);
ALTER TABLE carts ADD FOREIGN KEY (user_id) REFERENCES users(id);
CREATE INDEX carts_user_id_idx ON carts(user_id);
ALTER TABLE carts ADD FOREIGN KEY (restaurant_id) REFERENCES restaurants(id);
CREATE INDEX carts_restaurant_id_idx ON carts(restaurant_id);
ALTER TABLE cart_items ADD FOREIGN KEY (cart_id) REFERENCES carts(id);
CREATE INDEX cart_items_cart_id_idx ON cart_items(cart_id);
ALTER TABLE cart_items ADD FOREIGN KEY (menu_item_id) REFERENCES menu_items(id);
CREATE INDEX cart_items_menu_item_id_idx ON cart_items(menu_item_id);
ALTER TABLE cart_items ADD FOREIGN KEY (variant_id) REFERENCES item_variants(id);
CREATE INDEX cart_items_variant_id_idx ON cart_items(variant_id);
ALTER TABLE orders ADD FOREIGN KEY (user_id) REFERENCES users(id);
CREATE INDEX orders_user_id_idx ON orders(user_id);
ALTER TABLE orders ADD FOREIGN KEY (restaurant_id) REFERENCES restaurants(id);
CREATE INDEX orders_restaurant_id_idx ON orders(restaurant_id);
ALTER TABLE orders ADD FOREIGN KEY (address_id) REFERENCES user_addresses(id);
CREATE INDEX orders_address_id_idx ON orders(address_id);
ALTER TABLE order_items ADD FOREIGN KEY (order_id) REFERENCES orders(id);
CREATE INDEX order_items_order_id_idx ON order_items(order_id);
ALTER TABLE order_items ADD FOREIGN KEY (menu_item_id) REFERENCES menu_items(id);
CREATE INDEX order_items_menu_item_id_idx ON order_items(menu_item_id);
ALTER TABLE order_items ADD FOREIGN KEY (variant_id) REFERENCES item_variants(id);
CREATE INDEX order_items_variant_id_idx ON order_items(variant_id);
ALTER TABLE order_status_history ADD FOREIGN KEY (order_id) REFERENCES orders(id);
CREATE INDEX order_status_history_order_id_idx ON order_status_history(order_id);
ALTER TABLE order_status_history ADD FOREIGN KEY (changed_by) REFERENCES users(id);
CREATE INDEX order_status_history_changed_by_idx ON order_status_history(changed_by);
ALTER TABLE riders ADD FOREIGN KEY (user_id) REFERENCES users(id);
CREATE INDEX riders_user_id_idx ON riders(user_id);
ALTER TABLE riders ADD FOREIGN KEY (station_id) REFERENCES stations(id);
CREATE INDEX riders_station_id_idx ON riders(station_id);
ALTER TABLE riders ADD FOREIGN KEY (approved_by) REFERENCES users(id);
CREATE INDEX riders_approved_by_idx ON riders(approved_by);
ALTER TABLE rider_documents ADD FOREIGN KEY (rider_id) REFERENCES riders(id);
CREATE INDEX rider_documents_rider_id_idx ON rider_documents(rider_id);
ALTER TABLE delivery_agent_bank_details ADD FOREIGN KEY (rider_id) REFERENCES riders(id);
CREATE INDEX delivery_agent_bank_details_rider_id_idx ON delivery_agent_bank_details(rider_id);
ALTER TABLE delivery_assignments ADD FOREIGN KEY (order_id) REFERENCES orders(id);
CREATE INDEX delivery_assignments_order_id_idx ON delivery_assignments(order_id);
ALTER TABLE delivery_assignments ADD FOREIGN KEY (rider_id) REFERENCES riders(id);
CREATE INDEX delivery_assignments_rider_id_idx ON delivery_assignments(rider_id);
ALTER TABLE rider_locations ADD FOREIGN KEY (rider_id) REFERENCES riders(id);
CREATE INDEX rider_locations_rider_id_idx ON rider_locations(rider_id);
ALTER TABLE payment_intents ADD FOREIGN KEY (order_id) REFERENCES orders(id);
CREATE INDEX payment_intents_order_id_idx ON payment_intents(order_id);
ALTER TABLE payment_intents ADD FOREIGN KEY (restaurant_id) REFERENCES restaurants(id);
CREATE INDEX payment_intents_restaurant_id_idx ON payment_intents(restaurant_id);
ALTER TABLE payment_intents ADD FOREIGN KEY (delivery_agent_id) REFERENCES riders(id);
CREATE INDEX payment_intents_delivery_agent_id_idx ON payment_intents(delivery_agent_id);
ALTER TABLE payments ADD FOREIGN KEY (payment_intent_id) REFERENCES payment_intents(id);
CREATE INDEX payments_payment_intent_id_idx ON payments(payment_intent_id);
ALTER TABLE payments ADD FOREIGN KEY (order_id) REFERENCES orders(id);
CREATE INDEX payments_order_id_idx ON payments(order_id);
ALTER TABLE registration_fees ADD FOREIGN KEY (restaurant_id) REFERENCES restaurants(id);
CREATE INDEX registration_fees_restaurant_id_idx ON registration_fees(restaurant_id);
ALTER TABLE registration_fees ADD FOREIGN KEY (delivery_agent_id) REFERENCES riders(id);
CREATE INDEX registration_fees_delivery_agent_id_idx ON registration_fees(delivery_agent_id);
ALTER TABLE registration_fees ADD FOREIGN KEY (payment_intent_id) REFERENCES payment_intents(id);
CREATE INDEX registration_fees_payment_intent_id_idx ON registration_fees(payment_intent_id);
ALTER TABLE restaurant_earnings ADD FOREIGN KEY (restaurant_id) REFERENCES restaurants(id);
CREATE INDEX restaurant_earnings_restaurant_id_idx ON restaurant_earnings(restaurant_id);
ALTER TABLE restaurant_earnings ADD FOREIGN KEY (order_id) REFERENCES orders(id);
CREATE INDEX restaurant_earnings_order_id_idx ON restaurant_earnings(order_id);
ALTER TABLE delivery_agent_earnings ADD FOREIGN KEY (delivery_agent_id) REFERENCES riders(id);
CREATE INDEX delivery_agent_earnings_delivery_agent_id_idx ON delivery_agent_earnings(delivery_agent_id);
ALTER TABLE delivery_agent_earnings ADD FOREIGN KEY (order_id) REFERENCES orders(id);
CREATE INDEX delivery_agent_earnings_order_id_idx ON delivery_agent_earnings(order_id);
ALTER TABLE restaurant_payouts ADD FOREIGN KEY (restaurant_id) REFERENCES restaurants(id);
CREATE INDEX restaurant_payouts_restaurant_id_idx ON restaurant_payouts(restaurant_id);
ALTER TABLE restaurant_payouts ADD FOREIGN KEY (delivery_agent_id) REFERENCES riders(id);
CREATE INDEX restaurant_payouts_delivery_agent_id_idx ON restaurant_payouts(delivery_agent_id);
ALTER TABLE restaurant_payouts ADD FOREIGN KEY (order_id) REFERENCES orders(id);
CREATE INDEX restaurant_payouts_order_id_idx ON restaurant_payouts(order_id);
ALTER TABLE delivery_agent_payouts ADD FOREIGN KEY (delivery_agent_id) REFERENCES riders(id);
CREATE INDEX delivery_agent_payouts_delivery_agent_id_idx ON delivery_agent_payouts(delivery_agent_id);
ALTER TABLE delivery_agent_payouts ADD FOREIGN KEY (bank_details_id) REFERENCES delivery_agent_bank_details(id);
CREATE INDEX delivery_agent_payouts_bank_details_id_idx ON delivery_agent_payouts(bank_details_id);
ALTER TABLE restaurant_payout_allocations ADD FOREIGN KEY (restaurant_payout_id) REFERENCES restaurant_payouts(id);
CREATE INDEX restaurant_payout_allocations_restaurant_payout_id_idx ON restaurant_payout_allocations(restaurant_payout_id);
ALTER TABLE restaurant_payout_allocations ADD FOREIGN KEY (restaurant_earning_id) REFERENCES restaurant_earnings(id);
CREATE INDEX restaurant_payout_allocations_restaurant_earning_id_idx ON restaurant_payout_allocations(restaurant_earning_id);
ALTER TABLE delivery_agent_payout_allocations ADD FOREIGN KEY (delivery_agent_payout_id) REFERENCES delivery_agent_payouts(id);
CREATE INDEX delivery_agent_payout_allocations_delivery_agent_payout_id_idx ON delivery_agent_payout_allocations(delivery_agent_payout_id);
ALTER TABLE delivery_agent_payout_allocations ADD FOREIGN KEY (delivery_agent_earning_id) REFERENCES delivery_agent_earnings(id);
CREATE INDEX delivery_agent_payout_allocations_delivery_agent_earning_id_idx ON delivery_agent_payout_allocations(delivery_agent_earning_id);
ALTER TABLE cash_collections ADD FOREIGN KEY (order_id) REFERENCES orders(id);
CREATE INDEX cash_collections_order_id_idx ON cash_collections(order_id);
ALTER TABLE cash_collections ADD FOREIGN KEY (delivery_agent_id) REFERENCES riders(id);
CREATE INDEX cash_collections_delivery_agent_id_idx ON cash_collections(delivery_agent_id);
ALTER TABLE cash_collections ADD FOREIGN KEY (restaurant_id) REFERENCES restaurants(id);
CREATE INDEX cash_collections_restaurant_id_idx ON cash_collections(restaurant_id);
ALTER TABLE cash_collections ADD FOREIGN KEY (restaurant_payout_id) REFERENCES restaurant_payouts(id);
CREATE INDEX cash_collections_restaurant_payout_id_idx ON cash_collections(restaurant_payout_id);
ALTER TABLE financial_transactions ADD FOREIGN KEY (order_id) REFERENCES orders(id);
CREATE INDEX financial_transactions_order_id_idx ON financial_transactions(order_id);
ALTER TABLE financial_transactions ADD FOREIGN KEY (payment_id) REFERENCES payments(id);
CREATE INDEX financial_transactions_payment_id_idx ON financial_transactions(payment_id);
ALTER TABLE financial_transactions ADD FOREIGN KEY (restaurant_earning_id) REFERENCES restaurant_earnings(id);
CREATE INDEX financial_transactions_restaurant_earning_id_idx ON financial_transactions(restaurant_earning_id);
ALTER TABLE financial_transactions ADD FOREIGN KEY (agent_earning_id) REFERENCES delivery_agent_earnings(id);
CREATE INDEX financial_transactions_agent_earning_id_idx ON financial_transactions(agent_earning_id);
ALTER TABLE financial_transactions ADD FOREIGN KEY (restaurant_payout_id) REFERENCES restaurant_payouts(id);
CREATE INDEX financial_transactions_restaurant_payout_id_idx ON financial_transactions(restaurant_payout_id);
ALTER TABLE financial_transactions ADD FOREIGN KEY (agent_payout_id) REFERENCES delivery_agent_payouts(id);
CREATE INDEX financial_transactions_agent_payout_id_idx ON financial_transactions(agent_payout_id);
ALTER TABLE financial_transactions ADD FOREIGN KEY (registration_fee_id) REFERENCES registration_fees(id);
CREATE INDEX financial_transactions_registration_fee_id_idx ON financial_transactions(registration_fee_id);
ALTER TABLE refunds ADD FOREIGN KEY (payment_id) REFERENCES payments(id);
CREATE INDEX refunds_payment_id_idx ON refunds(payment_id);
ALTER TABLE refunds ADD FOREIGN KEY (order_id) REFERENCES orders(id);
CREATE INDEX refunds_order_id_idx ON refunds(order_id);
ALTER TABLE coupon_usages ADD FOREIGN KEY (coupon_id) REFERENCES coupons(id);
CREATE INDEX coupon_usages_coupon_id_idx ON coupon_usages(coupon_id);
ALTER TABLE coupon_usages ADD FOREIGN KEY (user_id) REFERENCES users(id);
CREATE INDEX coupon_usages_user_id_idx ON coupon_usages(user_id);
ALTER TABLE coupon_usages ADD FOREIGN KEY (order_id) REFERENCES orders(id);
CREATE INDEX coupon_usages_order_id_idx ON coupon_usages(order_id);

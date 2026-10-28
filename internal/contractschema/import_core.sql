-- Source references stay explicitly qualified. Target references resolve only
-- inside the replacement schema selected by PrepareLegacy.
CREATE TEMP TABLE legacy_customer_uuid_map(old_uuid uuid PRIMARY KEY,target_uuid uuid NOT NULL) ON COMMIT DROP;
INSERT INTO legacy_customer_uuid_map
SELECT c.id,COALESCE(c.user_id,(SELECT u.id FROM public.users u WHERE u.id=c.id OR (c.email IS NOT NULL AND lower(u.email)=lower(c.email)) ORDER BY (u.id=c.id) DESC LIMIT 1),c.id)
FROM public.customers c;

INSERT INTO users(uuid,full_name,email,phone,password_hash,role,status,created_at,updated_at,deleted_at)
SELECT u.id,u.full_name,u.email,u.phone,u.password_hash,u.role,CASE WHEN u.disabled_at IS NULL THEN 'ACTIVE' ELSE 'SUSPENDED' END,u.created_at,u.updated_at,u.deleted_at
FROM public.users u;
INSERT INTO users(uuid,full_name,email,phone,role,status,created_at,updated_at)
SELECT mapping.target_uuid,c.name,c.email,c.phone,'CUSTOMER','ACTIVE',c.created_at,c.updated_at
FROM public.customers c JOIN legacy_customer_uuid_map mapping ON mapping.old_uuid=c.id
WHERE NOT EXISTS(SELECT 1 FROM users u WHERE u.uuid=mapping.target_uuid);

INSERT INTO stations(uuid,name,description,latitude,longitude,radius_km,status,created_at,updated_at)
SELECT s.id,s.name,NULL,s.latitude,s.longitude,s.radius_m/1000,CASE WHEN s.active THEN 'ACTIVE' ELSE 'INACTIVE' END,s.created_at,s.updated_at
FROM public.stations s;

INSERT INTO restaurants(uuid,owner_user_id,station_id,name,description,phone,address,latitude,longitude,image_url,fssai_number,status,is_open,approved_at,created_at,updated_at,deleted_at)
SELECT old.id,u.id,s.id,old.name,old.description,old.phone,
 (SELECT b.address FROM restaurants.restaurant_branches b WHERE b.restaurant_id=old.id ORDER BY b.created_at,b.id LIMIT 1),
 old.latitude,old.longitude,NULL,
 (SELECT a.data->>'fssai_number' FROM public.partner_applications a WHERE a.resource_id=old.id AND a.kind='restaurant'),
 old.approval_status,old.is_active,old.reviewed_at,old.created_at,old.updated_at,old.deleted_at
FROM public.restaurants old LEFT JOIN users u ON u.uuid=old.owner_id LEFT JOIN stations s ON s.uuid=old.station_id;

INSERT INTO menu_categories(uuid,name,description,image_url,sort_order,is_active,created_at,updated_at,deleted_at)
SELECT old.id,old.name,old.description,old.image_url,old.sort_order,old.is_active,old.created_at,old.updated_at,old.deleted_at FROM public.menu_categories old;
INSERT INTO menu_items(uuid,restaurant_id,category_id,name,description,base_price,image_url,is_available,created_at,updated_at,deleted_at)
SELECT old.id,r.id,c.id,old.name,old.description,old.base_price/100.0,old.metadata->>'image_url',old.is_available,old.created_at,old.updated_at,old.deleted_at
FROM public.menu_items old JOIN restaurants r ON r.uuid=old.restaurant_id JOIN menu_categories c ON c.uuid=old.category_id;
INSERT INTO item_variants(uuid,menu_item_id,name,price_delta,is_available,created_at,updated_at)
SELECT old.id,i.id,old.name,(old.price-source.base_price)/100.0,old.is_active,old.created_at,old.updated_at
FROM public.item_variants old JOIN public.menu_items source ON source.id=old.item_id JOIN menu_items i ON i.uuid=old.item_id;

INSERT INTO user_addresses(uuid,user_id,label,contact_name,contact_phone,address_line1,address_line2,city,latitude,longitude,is_default,created_at,updated_at,deleted_at)
SELECT old.id,u.id,old.label,u.full_name,u.phone,old.address_line1,old.address_line2,old.city,old.latitude,old.longitude,old.is_default,old.created_at,old.updated_at,old.deleted_at
FROM public.user_addresses old JOIN legacy_customer_uuid_map mapping ON mapping.old_uuid=old.customer_id JOIN users u ON u.uuid=mapping.target_uuid;
-- Each historical snapshot becomes a retained address entity; saved-address
-- edits therefore cannot rewrite an old delivery destination.
INSERT INTO user_addresses(uuid,user_id,label,contact_name,contact_phone,address_line1,address_line2,city,latitude,longitude,is_default,created_at,updated_at)
SELECT md5(old.id::text||'/historical-address')::uuid,u.id,'Historical order address',
 COALESCE(old.delivery_address->>'contact_name',u.full_name),COALESCE(old.delivery_address->>'contact_phone',u.phone),
 old.delivery_address->>'address_line1',old.delivery_address->>'address_line2',old.delivery_address->>'city',
 NULLIF(old.delivery_address->>'latitude','')::numeric,NULLIF(old.delivery_address->>'longitude','')::numeric,false,old.created_at,old.created_at
FROM public.orders old JOIN legacy_customer_uuid_map mapping ON mapping.old_uuid=old.customer_id JOIN users u ON u.uuid=mapping.target_uuid
WHERE old.delivery_address IS NOT NULL AND old.delivery_address<>'{}'::jsonb;

-- A platform fee has no commercial column in the approved order schema. Do not
-- silently relabel it as food, tax or delivery; stop until its mapping is agreed.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM public.orders WHERE platform_fee<>0) OR EXISTS(SELECT 1 FROM finance.order_financials WHERE platform_charge<>0) THEN
  RAISE EXCEPTION 'legacy platform charges require an explicit commercial-total mapping';
 END IF;
END $$;
INSERT INTO orders(uuid,order_number,user_id,restaurant_id,address_id,status,subtotal,delivery_fee,tax_amount,discount_amount,total_amount,currency,placed_at,created_at,updated_at,deleted_at)
SELECT old.id,'LEGACY-'||replace(old.id::text,'-',''),u.id,r.id,a.id,
 CASE old.status WHEN 'PAYMENT_PENDING' THEN 'PENDING_PAYMENT' WHEN 'CREATED' THEN 'CONFIRMED' ELSE old.status END,
 CASE WHEN f.order_id IS NOT NULL THEN f.gross/100.0 ELSE old.total_amount/100.0 END,
 CASE WHEN f.order_id IS NOT NULL THEN f.delivery_charge/100.0 ELSE 0 END,
 CASE WHEN f.order_id IS NOT NULL THEN f.tax/100.0 ELSE 0 END,
 COALESCE(f.discount,0)/100.0,
 CASE WHEN f.order_id IS NOT NULL THEN f.total/100.0 ELSE old.total_amount/100.0 END,
 old.currency,old.created_at,old.created_at,old.updated_at,old.deleted_at
FROM public.orders old JOIN legacy_customer_uuid_map mapping ON mapping.old_uuid=old.customer_id
JOIN users u ON u.uuid=mapping.target_uuid JOIN restaurants r ON r.uuid=old.restaurant_id
LEFT JOIN user_addresses a ON a.uuid=md5(old.id::text||'/historical-address')::uuid
LEFT JOIN finance.order_financials f ON f.order_id=old.id;
-- Retain old receipt labels through referenced historical menu entities. The
-- replacement order_items has no name snapshot column; pointing every old line
-- at today's renamed/repriced menu item would silently rewrite receipt history.
INSERT INTO menu_items(uuid,restaurant_id,category_id,name,description,base_price,is_available,created_at,updated_at,deleted_at)
SELECT md5(old.id::text||'/historical-menu-item')::uuid,o.restaurant_id,current.category_id,old.name_snapshot,NULL,old.unit_price/100.0,false,o.created_at,o.created_at,o.created_at
FROM public.order_items old JOIN orders o ON o.uuid=old.order_id LEFT JOIN menu_items current ON current.uuid=old.item_id
WHERE current.id IS NULL OR current.name IS DISTINCT FROM old.name_snapshot OR current.base_price*100 IS DISTINCT FROM old.unit_price;
INSERT INTO order_items(uuid,order_id,menu_item_id,quantity,unit_price,special_instructions,created_at,updated_at)
SELECT old.id,o.id,COALESCE(historical.id,i.id),old.quantity,old.unit_price/100.0,old.metadata->>'special_instructions',o.created_at,o.updated_at
FROM public.order_items old JOIN orders o ON o.uuid=old.order_id LEFT JOIN menu_items i ON i.uuid=old.item_id
LEFT JOIN menu_items historical ON historical.uuid=md5(old.id::text||'/historical-menu-item')::uuid;
INSERT INTO order_status_history(uuid,order_id,from_status,to_status,changed_by,changed_at,note,created_at,updated_at)
SELECT old.id,o.id,lag(old.status::text) OVER(PARTITION BY old.order_id ORDER BY old.created_at,old.id),old.status::text,u.id,old.created_at,old.notes,old.created_at,old.created_at
FROM public.order_status_history old JOIN orders o ON o.uuid=old.order_id LEFT JOIN users u ON u.uuid=old.changed_by;

INSERT INTO riders(uuid,user_id,station_id,vehicle_type,verification_status,availability_status,created_at,updated_at,deleted_at)
SELECT old.id,u.id,s.id,(SELECT v.vehicle_type FROM delivery.vehicles v WHERE v.partner_id=old.id ORDER BY v.is_active DESC,v.id LIMIT 1),old.approval_status,old.status,old.created_at,old.updated_at,old.deleted_at
FROM public.riders old JOIN users u ON u.uuid=old.user_id LEFT JOIN stations s ON s.uuid=old.station_id;
INSERT INTO delivery_assignments(uuid,order_id,rider_id,status,assigned_at,picked_up_at,delivered_at,created_at,updated_at)
SELECT old.id,o.id,r.id,old.status,old.created_at,NULLIF(to_jsonb(old)->>'picked_up_at','')::timestamptz,NULLIF(to_jsonb(old)->>'delivered_at','')::timestamptz,old.created_at,old.updated_at
FROM public.delivery_assignments old JOIN orders o ON o.uuid=old.order_id JOIN riders r ON r.uuid=old.partner_id;
INSERT INTO rider_locations(uuid,rider_id,latitude,longitude,recorded_at,created_at,updated_at)
SELECT old.id,r.id,old.latitude,old.longitude,old.recorded_at,old.recorded_at,old.recorded_at FROM public.rider_locations old JOIN riders r ON r.uuid=old.partner_id;

INSERT INTO restaurant_documents(uuid,restaurant_id,document_type,file_url,verification_status,created_at,updated_at)
SELECT old.id,r.id,old.document_type,old.storage_key,old.status,old.created_at,old.updated_at FROM restaurants.merchant_documents old JOIN restaurants r ON r.uuid=old.restaurant_id;
INSERT INTO restaurant_documents(uuid,restaurant_id,document_type,file_url,verification_status,created_at,updated_at)
SELECT doc.id,r.id,doc.document_type,doc.backend||':'||doc.id::text,'PENDING',doc.created_at,doc.created_at
FROM public.private_documents doc JOIN public.partner_applications a ON doc.id=ANY(a.document_ids) AND a.kind='restaurant' AND a.user_id=doc.user_id JOIN restaurants r ON r.uuid=a.resource_id
ON CONFLICT(uuid) DO NOTHING;
INSERT INTO rider_documents(uuid,rider_id,document_type,file_url,verification_status,created_at,updated_at)
SELECT doc.id,r.id,doc.document_type,doc.backend||':'||doc.id::text,'PENDING',doc.created_at,doc.created_at
FROM public.private_documents doc JOIN public.partner_applications a ON doc.id=ANY(a.document_ids) AND a.kind='delivery' AND a.user_id=doc.user_id JOIN riders r ON r.uuid=a.resource_id;

INSERT INTO coupons(uuid,code,description,discount_type,discount_value,min_order_amount_paise,max_discount_amount_paise,valid_from,valid_until,usage_limit,is_active,created_at,updated_at,deleted_at)
SELECT old.id,old.code,old.description,old.discount_type,old.discount_value,old.min_order_amount,old.max_discount_amount,old.valid_from,old.valid_until,old.usage_limit,old.is_active,old.created_at,old.updated_at,old.deleted_at FROM orders.coupons old;
INSERT INTO coupon_usages(uuid,coupon_id,user_id,order_id,discount_amount_paise,redeemed_at,created_at,updated_at)
SELECT md5(old.order_id::text||'/coupon')::uuid,c.id,o.user_id,o.id,old.amount,old.created_at,old.created_at,old.created_at FROM finance.coupon_redemptions old JOIN coupons c ON c.uuid=old.coupon_id JOIN orders o ON o.uuid=old.order_id;

INSERT INTO payment_intents(uuid,order_id,purpose,method,stage,amount_paise,currency,status,provider_order_id,idempotency_key,expires_at,created_at,updated_at)
SELECT old.id,o.id,'ORDER',CASE WHEN old.provider='COD' THEN 'PAY_ON_DELIVERY' ELSE 'RAZORPAY' END,'CHECKOUT',old.amount,old.currency,
 CASE old.status WHEN 'PAID' THEN 'SUCCEEDED' WHEN 'EXPIRED' THEN 'CANCELLED' WHEN 'CREATING' THEN 'CREATED' ELSE old.status END,
 old.provider_id,'legacy/order/'||old.id::text,old.expires_at,old.created_at,old.created_at FROM finance.payment_attempts old JOIN orders o ON o.uuid=old.order_id;
INSERT INTO payment_intents(uuid,restaurant_id,delivery_agent_id,purpose,method,stage,amount_paise,currency,status,provider_order_id,idempotency_key,expires_at,created_at,updated_at)
SELECT old.id,r.id,agent.id,CASE WHEN old.restaurant_id IS NOT NULL THEN 'RESTAURANT_REGISTRATION' ELSE 'AGENT_REGISTRATION' END,'RAZORPAY','REGISTRATION',old.amount,old.currency,
 CASE old.status WHEN 'PAID' THEN 'SUCCEEDED' WHEN 'EXPIRED' THEN 'CANCELLED' WHEN 'CREATING' THEN 'CREATED' ELSE old.status END,
 old.provider_id,'legacy/registration/'||old.id::text,old.expires_at,old.created_at,old.created_at
FROM finance.onboarding_attempts old LEFT JOIN restaurants r ON r.uuid=old.restaurant_id LEFT JOIN riders agent ON agent.uuid=old.delivery_agent_id;
INSERT INTO registration_fees(uuid,restaurant_id,delivery_agent_id,amount_paise,currency,payment_intent_id,status,paid_at,created_at,updated_at)
SELECT md5(r.uuid::text||'/registration-fee')::uuid,r.id,NULL,COALESCE((SELECT amount FROM finance.onboarding_attempts WHERE restaurant_id=r.uuid ORDER BY (status='PAID') DESC,created_at DESC LIMIT 1),20000),'INR',
 (SELECT i.id FROM payment_intents i WHERE i.restaurant_id=r.id ORDER BY (i.status='SUCCEEDED') DESC,i.created_at DESC LIMIT 1),
 CASE WHEN source.fee_payment_status='PAID' THEN 'PAID' ELSE 'DUE' END,
 (SELECT created_at FROM finance.onboarding_attempts WHERE restaurant_id=r.uuid AND status='PAID' ORDER BY created_at LIMIT 1),r.created_at,r.updated_at
FROM restaurants r JOIN public.restaurants source ON source.id=r.uuid
UNION ALL
SELECT md5(r.uuid::text||'/registration-fee')::uuid,NULL,r.id,COALESCE((SELECT amount FROM finance.onboarding_attempts WHERE delivery_agent_id=r.uuid ORDER BY (status='PAID') DESC,created_at DESC LIMIT 1),15000),'INR',
 (SELECT i.id FROM payment_intents i WHERE i.delivery_agent_id=r.id ORDER BY (i.status='SUCCEEDED') DESC,i.created_at DESC LIMIT 1),
 CASE WHEN EXISTS(SELECT 1 FROM finance.onboarding_attempts WHERE delivery_agent_id=r.uuid AND status='PAID') THEN 'PAID' ELSE 'DUE' END,
 (SELECT created_at FROM finance.onboarding_attempts WHERE delivery_agent_id=r.uuid AND status='PAID' ORDER BY created_at LIMIT 1),r.created_at,r.updated_at
FROM riders r;
INSERT INTO payments(uuid,payment_intent_id,order_id,provider,provider_payment_id,method,amount_paise,currency,status,received_by,captured_at,created_at,updated_at)
SELECT old.id,i.id,i.order_id,CASE WHEN old.provider='COD' THEN 'CASH' ELSE old.provider END,old.payment_reference,CASE WHEN old.provider='COD' THEN 'CASH' ELSE 'OTHER' END,old.amount,old.currency,'CAPTURED',
 CASE WHEN old.provider='COD' AND EXISTS(SELECT 1 FROM finance.cod_custody c WHERE c.order_id=old.order_id) THEN 'DELIVERY_AGENT' ELSE 'YEMBER' END,old.created_at,old.created_at,old.created_at
FROM finance.payment_attempts old JOIN payment_intents i ON i.uuid=old.id WHERE old.status='PAID';
INSERT INTO payments(uuid,payment_intent_id,provider,provider_payment_id,method,amount_paise,currency,status,received_by,captured_at,created_at,updated_at)
SELECT old.id,i.id,'RAZORPAY',old.payment_reference,'OTHER',old.amount,old.currency,'CAPTURED','YEMBER',old.created_at,old.created_at,old.created_at
FROM finance.onboarding_attempts old JOIN payment_intents i ON i.uuid=old.id WHERE old.status='PAID';

INSERT INTO restaurant_earnings(uuid,restaurant_id,order_id,gross_amount_paise,fee_amount_paise,adjustment_amount_paise,net_amount_paise,status,eligible_at,created_at,updated_at)
SELECT md5(source.order_id::text||'/restaurant-earning')::uuid,o.restaurant_id,o.id,source.gross,source.commission,0,source.gross-source.commission,'AVAILABLE',event.created_at,event.created_at,event.created_at
FROM finance.order_financials source JOIN orders o ON o.uuid=source.order_id
JOIN finance.journals event ON event.order_id=source.order_id AND event.kind='EARN';
INSERT INTO delivery_agent_earnings(uuid,delivery_agent_id,order_id,base_amount_paise,incentive_amount_paise,adjustment_amount_paise,net_amount_paise,status,eligible_at,created_at,updated_at)
SELECT md5(source.order_id::text||'/agent-earning')::uuid,r.id,o.id,source.rider_base,0,0,source.rider_base,'AVAILABLE',event.created_at,event.created_at,event.created_at
FROM finance.order_financials source JOIN orders o ON o.uuid=source.order_id
JOIN finance.journals event ON event.order_id=source.order_id AND event.kind='EARN'
JOIN public.delivery_assignments assignment ON assignment.order_id=source.order_id AND assignment.status='DELIVERED'
JOIN riders r ON r.uuid=assignment.partner_id;

INSERT INTO restaurant_payouts(uuid,restaurant_id,delivery_agent_id,order_id,method,amount_paise,status,confirmed_by_restaurant_at,paid_at,created_at,updated_at,deleted_at)
SELECT old.uuid,o.restaurant_id,r.id,o.id,'CASH_HANDOVER',old.amount_paise,old.status,old.confirmed_at,old.confirmed_at,old.created_at,old.updated_at,old.deleted_at
FROM finance.cash_handovers old JOIN orders o ON o.uuid=old.order_id JOIN finance.cod_custody custody ON custody.order_id=old.order_id JOIN riders r ON r.uuid=custody.partner_id;
INSERT INTO cash_collections(uuid,order_id,delivery_agent_id,restaurant_id,amount_due_paise,amount_collected_paise,collected_at,restaurant_payout_id,status,created_at,updated_at)
SELECT md5(old.order_id::text||'/cash-collection')::uuid,o.id,r.id,o.restaurant_id,old.amount,
 CASE WHEN collected.created_at IS NOT NULL THEN old.amount ELSE 0 END,collected.created_at,p.id,
 CASE WHEN p.status='PAID' THEN 'HANDED_OVER' WHEN p.status='DISPUTED' THEN 'DISPUTED' WHEN collected.created_at IS NOT NULL THEN 'COLLECTED' ELSE 'DUE' END,
 old.created_at,COALESCE(p.updated_at,collected.created_at,old.created_at)
FROM finance.cod_custody old JOIN orders o ON o.uuid=old.order_id JOIN riders r ON r.uuid=old.partner_id
LEFT JOIN finance.journals collected ON collected.order_id=old.order_id AND collected.kind='CASH_COLLECTION'
LEFT JOIN restaurant_payouts p ON p.order_id=o.id AND p.method='CASH_HANDOVER';
INSERT INTO restaurant_payout_allocations(uuid,restaurant_payout_id,restaurant_earning_id,amount_paise,created_at,updated_at)
SELECT md5(p.uuid::text||'/allocation')::uuid,p.id,e.id,LEAST(p.amount_paise,e.net_amount_paise),p.paid_at,p.paid_at
FROM restaurant_payouts p JOIN restaurant_earnings e ON e.order_id=p.order_id AND e.restaurant_id=p.restaurant_id
WHERE p.method='CASH_HANDOVER' AND p.status='PAID' AND LEAST(p.amount_paise,e.net_amount_paise)>0;
INSERT INTO system_activity_logs(uuid,actor_type,actor_id,action,entity_type,entity_uuid,after_json,occurred_at,created_at,updated_at)
SELECT md5(old.uuid::text||'/cash-history')::uuid,'AGENT',u.id,'LEGACY_IMPORT','RESTAURANT_PAYOUT',old.uuid,
 jsonb_build_object('submitted_by_uuid',old.submitted_by,'confirmed_by_uuid',old.confirmed_by,'original_status',old.status,'amount_paise',old.amount_paise),old.created_at,old.created_at,old.created_at
FROM finance.cash_handovers old LEFT JOIN users u ON u.uuid=old.submitted_by;

INSERT INTO financial_transactions(uuid,order_id,payment_id,registration_fee_id,restaurant_earning_id,agent_earning_id,restaurant_payout_id,transaction_type,account_code,direction,amount_paise,currency,event_key,occurred_at,created_at,updated_at)
SELECT e.id,o.id,p.id,f.id,re.id,ae.id,cash_payout.id,
 CASE j.kind WHEN 'CAPTURE' THEN 'COLLECTION' WHEN 'CASH_COLLECTION' THEN 'COLLECTION' WHEN 'ONBOARDING' THEN 'COLLECTION' WHEN 'EARN' THEN 'EARNING' WHEN 'CASH_HANDOVER' THEN 'PAYOUT' WHEN 'PAYOUT' THEN 'PAYOUT' WHEN 'REFUND' THEN 'REFUND' WHEN 'REVERSAL' THEN 'REVERSAL' ELSE 'ADJUSTMENT' END,
 CASE e.account WHEN 'CASH' THEN 'YEMBER_BANK' WHEN 'UNEARNED' THEN 'ORDER_RECEIPTS' WHEN 'PARTNER_PAYABLE' THEN CASE WHEN source_user.role='DELIVERY_PARTNER' THEN 'AGENT_PAYABLE' ELSE 'RESTAURANT_PAYABLE' END ELSE e.account END,
 CASE WHEN e.amount>0 THEN 'DEBIT' ELSE 'CREDIT' END,abs(e.amount),j.currency,'legacy:'||j.id::text||':'||e.id::text,j.created_at,e.created_at,e.created_at
FROM finance.entries e JOIN finance.journals j ON j.id=e.journal_id
LEFT JOIN orders o ON o.uuid=j.order_id LEFT JOIN public.users source_user ON source_user.id=e.user_id
LEFT JOIN payments p ON p.provider_payment_id=j.reference
LEFT JOIN registration_fees f ON f.payment_intent_id=p.payment_intent_id
LEFT JOIN users payee ON payee.uuid=e.user_id
LEFT JOIN restaurants payee_restaurant ON payee_restaurant.id=o.restaurant_id AND payee_restaurant.owner_user_id=payee.id
LEFT JOIN restaurant_earnings re ON re.order_id=o.id AND re.restaurant_id=payee_restaurant.id
LEFT JOIN riders payee_rider ON payee_rider.user_id=payee.id
LEFT JOIN delivery_agent_earnings ae ON ae.order_id=o.id AND ae.delivery_agent_id=payee_rider.id
LEFT JOIN restaurant_payouts cash_payout ON cash_payout.order_id=o.id AND cash_payout.method='CASH_HANDOVER' AND j.kind='CASH_HANDOVER';

INSERT INTO system_activity_logs(uuid,actor_type,actor_id,action,entity_type,entity_uuid,after_json,occurred_at,created_at,updated_at)
SELECT old.id,'ADMIN',u.id,'LEGACY_IMPORT',CASE WHEN length(old.target_type)<=64 THEN upper(old.target_type) ELSE 'LEGACY_ENTITY' END,
 CASE WHEN old.target_id~*'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN old.target_id::uuid ELSE NULL END,
 jsonb_build_object('original_action',old.action,'original_target_type',old.target_type,'details',old.details),
 old.created_at,old.created_at,old.created_at
FROM public.admin_audit_logs old LEFT JOIN users u ON u.uuid=old.admin_id;
INSERT INTO system_activity_logs(uuid,actor_type,actor_id,action,entity_type,entity_uuid,after_json,occurred_at,created_at,updated_at)
SELECT old.id,'SYSTEM',u.id,'LEGACY_IMPORT','FINANCIAL_EVENT',old.id,
 jsonb_build_object('kind',old.kind,'reference',old.reference,'reason',old.reason,'original_event_key',old.event_key,'order_uuid',old.order_id,'currency',old.currency),
 old.created_at,old.created_at,old.created_at
FROM finance.journals old LEFT JOIN users u ON u.uuid=old.actor;

DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM public.users old WHERE NOT EXISTS(SELECT 1 FROM users current WHERE current.uuid=old.id)) THEN RAISE EXCEPTION 'user conservation check failed'; END IF;
 IF (SELECT count(*) FROM public.orders)<>(SELECT count(*) FROM orders) THEN RAISE EXCEPTION 'order conservation check failed'; END IF;
 IF (SELECT count(*) FROM public.order_items)<>(SELECT count(*) FROM order_items) THEN RAISE EXCEPTION 'order-line conservation check failed'; END IF;
 IF (SELECT count(*) FROM finance.entries)<>(SELECT count(*) FROM financial_transactions WHERE event_key LIKE 'legacy:%') THEN RAISE EXCEPTION 'ledger conservation check failed'; END IF;
 IF EXISTS(SELECT 1 FROM public.orders old JOIN orders current ON current.uuid=old.id WHERE current.total_amount*100<>old.total_amount) THEN RAISE EXCEPTION 'historical order total changed'; END IF;
 IF EXISTS(SELECT 1 FROM public.order_items old JOIN order_items current ON current.uuid=old.id WHERE current.unit_price*100<>old.unit_price OR current.quantity<>old.quantity) THEN RAISE EXCEPTION 'historical charged price or quantity changed'; END IF;
 IF EXISTS(SELECT 1 FROM public.order_items old JOIN order_items current ON current.uuid=old.id LEFT JOIN menu_items item ON item.id=current.menu_item_id WHERE item.id IS NULL OR item.name IS DISTINCT FROM old.name_snapshot) THEN RAISE EXCEPTION 'historical receipt label changed'; END IF;
 IF EXISTS(SELECT 1 FROM finance.entries old LEFT JOIN financial_transactions current ON current.uuid=old.id WHERE current.id IS NULL OR CASE current.direction WHEN 'DEBIT' THEN current.amount_paise ELSE -current.amount_paise END<>old.amount) THEN RAISE EXCEPTION 'historical ledger posting changed'; END IF;
 IF EXISTS(SELECT 1 FROM finance.cod_custody old LEFT JOIN cash_collections c ON c.uuid=md5(old.order_id::text||'/cash-collection')::uuid WHERE c.id IS NULL OR c.amount_due_paise<>old.amount) THEN RAISE EXCEPTION 'cash custody amount changed'; END IF;
 IF EXISTS(SELECT 1 FROM finance.cash_handovers old LEFT JOIN restaurant_payouts p ON p.uuid=old.uuid WHERE p.id IS NULL OR p.amount_paise<>old.amount_paise OR p.status<>old.status OR p.paid_at IS DISTINCT FROM old.confirmed_at) THEN RAISE EXCEPTION 'cash handover history changed'; END IF;
 IF EXISTS(
  WITH before AS (SELECT e.user_id,sum(e.amount) AS balance FROM finance.entries e WHERE e.account='PARTNER_PAYABLE' GROUP BY e.user_id),
  after AS (SELECT COALESCE(restaurant_user.uuid,rider_user.uuid) AS user_id,sum(CASE entry.direction WHEN 'DEBIT' THEN entry.amount_paise ELSE -entry.amount_paise END) AS balance
    FROM financial_transactions entry
    LEFT JOIN restaurant_earnings re ON re.id=entry.restaurant_earning_id LEFT JOIN restaurants r ON r.id=re.restaurant_id LEFT JOIN users restaurant_user ON restaurant_user.id=r.owner_user_id
    LEFT JOIN delivery_agent_earnings ae ON ae.id=entry.agent_earning_id LEFT JOIN riders agent ON agent.id=ae.delivery_agent_id LEFT JOIN users rider_user ON rider_user.id=agent.user_id
    WHERE entry.account_code IN ('RESTAURANT_PAYABLE','AGENT_PAYABLE') GROUP BY COALESCE(restaurant_user.uuid,rider_user.uuid))
  SELECT 1 FROM before FULL JOIN after USING(user_id) WHERE COALESCE(before.balance,0)<>COALESCE(after.balance,0) OR after.user_id IS NULL
 ) THEN RAISE EXCEPTION 'recipient payable balance changed or lost its owner reference'; END IF;
END $$;

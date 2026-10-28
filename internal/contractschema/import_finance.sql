-- A digest is evidence of an event, not the original webhook body or proof of
-- signature verification. Preserve that distinction and never replay an import.
-- Preserve the original AES-GCM ciphertext, key selector and associated-data
-- UUIDs together. Changing the numeric owner ID must not invalidate decryption.
-- Cleartext holder/routing fields are deliberately not inferred from ciphertext.
INSERT INTO restaurant_bank_accounts(uuid,restaurant_id,account_number_encrypted,verification_status,created_at,updated_at)
SELECT md5(old.id::text||'/'||r.uuid::text)::uuid,r.id,
 jsonb_build_object('format','legacy-v1','key_id',old.key_id,'owner_uuid',old.user_id,'bank_uuid',old.id,'ciphertext',encode(old.ciphertext,'base64'),'last_four',old.last_four,'provider_fund_account',old.provider_fund_account)::text,
 old.status,old.created_at,old.created_at
FROM finance.bank_accounts old JOIN users u ON u.uuid=old.user_id JOIN restaurants r ON r.owner_user_id=u.id;
INSERT INTO delivery_agent_bank_details(uuid,rider_id,account_number_encrypted,verification_status,created_at,updated_at)
SELECT old.id,r.id,
 jsonb_build_object('format','legacy-v1','key_id',old.key_id,'owner_uuid',old.user_id,'bank_uuid',old.id,'ciphertext',encode(old.ciphertext,'base64'),'last_four',old.last_four,'provider_fund_account',old.provider_fund_account)::text,
 old.status,old.created_at,old.created_at
FROM finance.bank_accounts old JOIN users u ON u.uuid=old.user_id JOIN riders r ON r.user_id=u.id;
INSERT INTO system_activity_logs(uuid,actor_type,actor_id,action,entity_type,entity_uuid,after_json,occurred_at,created_at,updated_at)
SELECT md5(old.id::text||'/verification-history')::uuid,'ADMIN',u.id,'LEGACY_IMPORT','BANK_VERIFICATION',old.id,
 jsonb_build_object('verification_reference',old.verification_reference,'verified_by_uuid',old.verified_by,'original_status',old.status),old.created_at,old.created_at,old.created_at
FROM finance.bank_accounts old LEFT JOIN users u ON u.uuid=old.verified_by;

INSERT INTO payment_webhook_events(uuid,provider,provider_event_id,event_type,payload_json,signature_valid,processing_status,processed_at,created_at,updated_at)
SELECT md5(old.provider||'/'||old.event_id)::uuid,old.provider,old.event_id,'LEGACY_EVENT',
 jsonb_build_object('legacy_digest',old.digest,'original_payload_available',false),NULL,'IMPORTED',NULL,old.received_at,old.received_at
FROM finance.provider_events old;

INSERT INTO refunds(uuid,payment_id,order_id,amount_paise,reason,provider_refund_id,status,completed_at,created_at,updated_at)
SELECT old.id,p.id,o.id,old.amount,left(old.reason,255),old.provider_id,
 CASE old.status WHEN 'SUCCEEDED' THEN 'COMPLETED' ELSE old.status END,
 (SELECT max(j.created_at) FROM finance.journals j WHERE j.kind='REFUND' AND j.order_id=old.order_id AND j.reference=old.provider_id),
 old.created_at,old.created_at
FROM finance.refund_requests old JOIN orders o ON o.uuid=old.order_id
LEFT JOIN payments p ON p.order_id=o.id AND p.status='CAPTURED';

-- Approval/request keys and unbounded original reasons belong in activity
-- history; the exact refund schema has no columns for these legacy facts.
INSERT INTO system_activity_logs(uuid,actor_type,actor_id,action,entity_type,entity_uuid,after_json,occurred_at,created_at,updated_at)
SELECT md5(old.id::text||'/request-history')::uuid,'ADMIN',u.id,'LEGACY_IMPORT','REFUND',old.id,
 jsonb_build_object('request_key',old.request_key,'reason',old.reason,'original_status',old.status,'approved_by_uuid',old.approved_by),
 old.created_at,old.created_at,old.created_at
FROM finance.refund_requests old LEFT JOIN users u ON u.uuid=old.requested_by;

-- Jobs are intent to request a refund, not completed transfers. Keep them on
-- review so an uncertain provider response cannot cause a second refund.
INSERT INTO refunds(uuid,payment_id,order_id,amount_paise,reason,status,created_at,updated_at)
SELECT md5(old.order_id::text||'/refund-job')::uuid,p.id,o.id,p.amount_paise,'Imported cancellation refund requires review','REVIEW',old.created_at,old.created_at
FROM finance.refund_jobs old JOIN orders o ON o.uuid=old.order_id
JOIN payments p ON p.order_id=o.id AND p.status='CAPTURED'
WHERE NOT EXISTS(SELECT 1 FROM refunds r WHERE r.order_id=o.id);
INSERT INTO system_activity_logs(uuid,actor_type,action,entity_type,entity_uuid,after_json,occurred_at,created_at,updated_at)
SELECT md5(old.order_id::text||'/refund-job-history')::uuid,'SYSTEM','LEGACY_IMPORT','REFUND_JOB',old.order_id,
 jsonb_build_object('original_status',old.status,'last_error',old.last_error,'requires_review',true),old.created_at,old.created_at,old.created_at
FROM finance.refund_jobs old;

INSERT INTO reconciliation_runs(uuid,source,period_start,period_end,status,difference_paise,summary_json,created_at,updated_at)
SELECT old.id,'LEGACY_RECONCILIATION',old.created_at,old.created_at,'IMPORTED',NULL,
 jsonb_build_object('reference',old.reference,'actor_uuid',old.actor,'details',old.details,'verified_by_import',false),old.created_at,old.created_at
FROM finance.reconciliations old;
INSERT INTO reconciliation_runs(uuid,source,period_start,period_end,status,difference_paise,summary_json,created_at,updated_at)
SELECT old.id,'LEGACY_DISCREPANCY',old.created_at,COALESCE(old.resolved_at,old.created_at),
 CASE WHEN old.resolved_at IS NULL THEN 'EXCEPTIONS' ELSE 'RESOLVED' END,NULL,
 jsonb_build_object('original_event_key',old.event_key,'kind',old.kind,'order_uuid',old.order_id,'details',old.details,'resolved_by_uuid',old.resolved_by,'resolution',old.resolution,'resolved_at',old.resolved_at),
 old.created_at,COALESCE(old.resolved_at,old.created_at)
FROM finance.discrepancies old;

DO $$ BEGIN
 IF EXISTS(
  SELECT 1 FROM finance.bank_accounts old WHERE NOT EXISTS(
   SELECT 1 FROM (SELECT account_number_encrypted FROM restaurant_bank_accounts UNION ALL SELECT account_number_encrypted FROM delivery_agent_bank_details) b
   WHERE (b.account_number_encrypted::jsonb->>'bank_uuid')::uuid=old.id
     AND (b.account_number_encrypted::jsonb->>'owner_uuid')::uuid=old.user_id
     AND b.account_number_encrypted::jsonb->>'key_id'=old.key_id
     AND decode(b.account_number_encrypted::jsonb->>'ciphertext','base64')=old.ciphertext
  )
 ) THEN RAISE EXCEPTION 'bank encryption context or ownership was not preserved'; END IF;
 IF EXISTS(SELECT 1 FROM finance.refund_requests old LEFT JOIN refunds r ON r.uuid=old.id WHERE r.id IS NULL OR r.amount_paise<>old.amount OR r.provider_refund_id IS DISTINCT FROM old.provider_id) THEN RAISE EXCEPTION 'refund value or provider reference changed'; END IF;
 IF EXISTS(SELECT 1 FROM finance.provider_events old LEFT JOIN payment_webhook_events current ON current.provider=old.provider AND current.provider_event_id=old.event_id WHERE current.id IS NULL OR current.payload_json->>'legacy_digest'<>old.digest) THEN RAISE EXCEPTION 'provider event evidence missing'; END IF;
 IF EXISTS(SELECT 1 FROM finance.reconciliations old LEFT JOIN reconciliation_runs r ON r.uuid=old.id WHERE r.id IS NULL OR r.summary_json->'details' IS DISTINCT FROM old.details) THEN RAISE EXCEPTION 'reconciliation history changed'; END IF;
 IF EXISTS(SELECT 1 FROM finance.discrepancies old LEFT JOIN reconciliation_runs r ON r.uuid=old.id WHERE r.id IS NULL OR r.summary_json->'details' IS DISTINCT FROM old.details) THEN RAISE EXCEPTION 'discrepancy history changed'; END IF;
END $$;

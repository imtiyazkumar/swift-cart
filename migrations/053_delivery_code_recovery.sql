ALTER TABLE delivery.completion_challenges ADD COLUMN sent_at timestamptz NOT NULL DEFAULT now(), ADD COLUMN resend_count integer NOT NULL DEFAULT 0 CHECK(resend_count BETWEEN 0 AND 3);

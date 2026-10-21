ALTER TABLE public.private_documents ADD COLUMN IF NOT EXISTS backend TEXT NOT NULL DEFAULT 'local';

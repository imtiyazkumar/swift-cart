-- Migration 011: Add auth.otp_codes table for OTP creation and verification

CREATE TABLE IF NOT EXISTS auth.otp_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    identifier TEXT NOT NULL,
    otp_code TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    is_used BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_otp_identifier ON auth.otp_codes(identifier, is_used, expires_at);

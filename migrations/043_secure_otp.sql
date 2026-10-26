-- Existing plaintext development challenges must not remain usable.
DELETE FROM auth.otp_codes;

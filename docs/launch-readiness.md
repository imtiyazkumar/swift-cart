# Validation and operational readiness

The local document schema, strict startup checks, administrator workspaces and automated workflows have been validated. The database is exactly the 36 documented tables plus the two approved coupon tables.

Automated checks cover numeric relationships and UUID boundaries, ownership/roles, refresh rotation/replay, global category access, service coverage and authoritative checkout, variant pricing, historical order retention, coupon limits and administrator lifecycle, balanced immutable financial postings, gross cash handover and fee receivables, discrepancies held for review, provider capture/refund idempotency, independent bank payout confirmation and legacy data preservation/rollback.

The web component/API tests and production build pass. All three apps and the shared partner package typecheck. Mobile transport/session tests and Expo lint run without errors.

Manual/external validation remains separate: real gateway capture/refund/webhook behavior, bank transfer confirmation, SMS device delivery, Android/iOS UI flows and browser visual review. The attempted browser review timed out awaiting Chrome computer-use approval. No real money was moved by automated verification.

Do not treat a populated legacy database as migrated merely because its old migrations ran. Unsupported financial conversion sources stop preparation and require a completed, verified conversion.

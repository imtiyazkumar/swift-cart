# Historical conversion fixtures

These packages support migration preservation and regression tests for the former
schema. No current command imports them. The active API is `internal/platform`;
current development seeds are `internal/devseed` and `internal/demoseed`.

- `auth`: historical identities, session fixtures and database rate limiting.
- `finance`: historical ledger, banking, payout and refund behavior used to verify
  lossless conversion.
- `devseed`: legacy-schema fixtures for isolated historical test databases.
- `config`: explicit options for those fixtures, without an environment loader.

The historical SQL bundle remains in `migrations/legacy` with its original
checksums. Current installs use the four stages in `migrations/document`. Retain
this archive while nonempty legacy conversion is supported; do not register its
HTTP handlers in the current API.

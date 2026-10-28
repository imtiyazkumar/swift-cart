# Rich local development fixtures

The opt-in demo seed adds a large synthetic dataset to the existing document-schema development database. It is separate from `seed_local`, whose minimal fixtures and behaviour are unchanged.

## Run and verify

From `yember-core`:

```sh
go run ./cmd/seed_demo --allow-local-yember
go run ./cmd/seed_demo --allow-local-yember --verify
go run ./cmd/seed_demo --allow-local-yember --inspect
python scripts/verify-demo-images.py
```

The command reads existing private `.env` settings without printing credentials. It accepts only loopback `yember_local`/`yember_local_test` targets in development/test, or the loopback `yember` development database with the explicit flag. It validates the 38-table schema before proceeding. No reset, truncate, deletion, provider request, or environment change is performed.

Defaults add 600 customers, 60 restaurant owners/restaurants, 120 delivery users/agents, 20 service areas, 15 menu categories, 1,200 menu items, 2,400 variants, 5,000 orders and 15,000 order lines. Related data covers all remaining tables: addresses, carts, documents, bank placeholders, assignments, locations, fees, payment intents/payments, earnings, payouts, allocations, cash collections, refunds, webhook examples, reconciliation, coupon redemptions, expired authentication records, and activity history.

Sizes are configurable with `--customers`, `--restaurants`, `--riders`, and `--orders`. `--dry-run` executes the same SQL and constraints but rolls back rows and does not create document files. Normal PostgreSQL identity-sequence gaps can still occur during dry runs or conflict checks.

## Recorded result

The completed default run added **203,885 rows** and left **204,216 total rows** across **all 38 tables**. A second complete run added **zero rows**. Detailed before/after counts are in [demo-data-counts.json](demo-data-counts.json).

The fixtures contain 1,275 new catalog image fields using 13 distinct direct Pexels URLs. Missing images on the three original minimal catalog fixtures were also filled. The URL checker confirmed HTTP 200, an image MIME type, and actual JPEG signatures for all 13 URLs. Public CDN availability can change; rerun the checker when needed.

## Images and documents

[demo_images.json](../internal/demoseed/demo_images.json) records every image URL, original photo page, and license link. These images are available under the [Pexels license](https://www.pexels.com/license/). URLs are fixed image resources rather than random-photo endpoints or expiring signed URLs. No image-service API key is needed.

The seed creates 360 small PDFs in `DOCUMENT_LOCAL_DIR` or the application's default `.private-documents` directory, named with the same UUIDs used by the authenticated document endpoint. Every PDF is marked **DEMO ONLY - NOT A REAL CERTIFICATE**. Existing files are not overwritten. The directory is excluded from Git. Test fixtures use a temporary document directory.

## Data meaning and preservation

All newly generated customer/partner identities use synthetic names, `.invalid` email domains and non-routable sample telephone patterns. Their initial password comes from the existing `LOCAL_SEED_PASSWORD`; no password is committed or printed. Existing identities, passwords, sessions, catalog content, and operational rows are preserved. Stable fixture UUIDs and conflict-safe inserts prevent duplicate records, while normal application edits to fixtures are retained.

Financial records and provider references are explicitly marked `DEMO`. They are illustrative local history, not evidence of real captured payments, refunds, bank verification, or transfers. Bank records contain marked, non-operational demo envelopes with sample last-four values, not real bank accounts or decryptable production credentials. No gateway, SMS, email, or bank service is called by the seed. The 600 seeded refresh tokens are expired and revoked; the 600 OTP records are expired and consumed.

Foreign keys, checks, append-only protections, audit triggers and deferred financial-balance triggers remain enabled. A seed-owned expression index temporarily speeds deferred event validation and is removed before commit; no permanent tables, indexes, migrations, or schema changes are introduced. The database writes use one transaction and an advisory lock.

Order totals equal their line sums, captured amounts match order totals, refunds stay within payment amounts, payout allocations match payout totals, and every seeded ledger event balances. Automatic order-status and activity history remain intact. Additional historical audit examples are explicitly labelled synthetic.

## Verification performed

- Isolated PostgreSQL integration test: full table coverage, dry-run rollback, repeated-seed idempotency, original administrator preservation, order/ledger consistency, expired authentication fixtures, document files, and image manifest consistency.
- Real development run and independent `--verify` check: 38 populated tables, 204,216 rows, 1,275 new image fields, 13 distinct image URLs, 360 PDFs.
- Repeated default run on the development database: zero rows added.
- `go test ./internal/demoseed ./cmd/seed_demo -count=1` with `YEMBER_INTEGRATION=1` passed.
- Image verification script: all 13 URLs returned actual JPEG files successfully.

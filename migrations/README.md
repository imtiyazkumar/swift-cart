# Domain migrations

Run from `yember-core`:

```sh
go run ./cmd/migrate
```

| File | Responsibility |
| --- | --- |
| `auth.sql` | Users, roles, sessions, password reset, OTP, MFA, rate limits |
| `customer.sql` | Customer profiles, addresses, favorites, lifecycle requests |
| `rider.sql` | Rider onboarding fields, vehicles, earnings, availability guards |
| `restaurant.sql` | Merchants, branches, hours, documents, reviews, commission configuration |
| `admin.sql` | Audit, platform settings, private documents, partner applications/support, stations |
| `catalog.sql` | Categories, items, variants, modifiers, addons, historical image updates, optional demo seed |
| `order.sql` | Orders, checkout, carts, coupons, history, amount compatibility, station snapshots |
| `delivery.sql` | Assignments, offers, completion codes/proofs, delivery station relations |
| `payments.sql` | Gateway records, onboarding payments, central finance, immutable ledgers, refunds, settlement/payout/reconciliation |
| `notification.sql` | In-app, email, SMS and push queues, devices, notification preferences and triggers |
| `realtime.sql` | Live locations and persisted order events/capture triggers used by realtime delivery |

## Execution and existing databases

These are domain source files, **not independently executable SQL scripts**.
`manifest.json` orders their blocks across domain boundaries, reconstructing the
original 54 migrations byte for byte. Cross-domain foreign keys, data repairs,
triggers and transaction boundaries therefore retain their existing behavior.
The embedded bundle is validated before any migration is applied.

The old numbered files have been removed. Their names remain stable migration
**identities** in the manifest and `public.schema_migrations`. Applied checksums,
timestamps and database data are not rewritten, and tracked migrations are not
replayed. Both historical LF and CRLF checksums are recognized. An incorrect
checksum still stops the migration. Fully migrated databases require no reset or
manual baseline. Partially migrated tracked databases resume their missing steps.

For an existing **untracked** database, inspect its actual schema and back up first.
Only then use the existing `--baseline-through <historical-identity>` option when
that identity really reflects what is installed. This option asserts that the
earlier SQL already ran; it does not verify the assertion. A fresh database needs
no baseline. `--file <historical-identity>` remains available for deliberate
repairs; a domain filename is rejected because it would bypass dependencies.

Demo users/catalog remain disabled by default. `--include-demo-seed` retains the
old optional seed behavior; never use it in production. For current local fixtures
prefer the guarded `cmd/seed_local` workflow documented in `docs/local-development.md`.

## Future changes

Do not edit historical blocks, their checksums or IDs. Add a new, uniquely numbered
standalone SQL migration (for example `054_descriptive_change.sql`); the runner
embeds and discovers it automatically, applies it in filename order, and tracks
its checksum using the same transaction/advisory lock. Do not add SQL outside the
marked blocks in domain files. `.gitattributes` preserves their exact bytes.

## Verification

```sh
YEMBER_INTEGRATION=1 go test ./... -count=1
```

Migration tests cover bundle integrity, partially migrated upgrades, unchanged
tracking history, repeated execution, line-ending compatibility, untracked schema
protection and transaction rollback. For an independent pre/post comparison, keep
an external snapshot of the original SQL and set `YEMBER_ORIGINAL_MIGRATIONS` to
that directory and `YEMBER_PG_DUMP` to the installed `pg_dump` executable. The
additional tests compare every original SQL byte, complete fresh schema dumps,
and every table's seed row count, with and without demo data. They create and
remove disposable test databases; they do not modify the configured database.

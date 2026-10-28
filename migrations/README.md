# Document database schema

The application installs exactly **38 tables in public**: the 36 tables from the two supplied architecture documents, plus the explicitly requested coupons and coupon_usages.

The active installation is ordered and contains only final definitions:

1. [001_schema.sql](document/001_schema.sql): the 38 tables, public UUIDs, numeric identities and foreign keys.
2. [002_constraints.sql](document/002_constraints.sql): invariants and operational indexes.
3. [003_behaviors.sql](document/003_behaviors.sql): current functions and triggers, each defined once.
4. [004_geography.sql](document/004_geography.sql): service-area calculations.

`document.SQL` embeds this order explicitly. Existing installations validate their
schema and refresh the idempotent behavior definitions; they do not replay table
creation or reset records. Historical intermediate function versions are absent
from the active baseline. All internal keys are GENERATED ALWAYS bigint identities.
Public identifiers are unique UUIDs. Foreign keys use bigint.

Run from yember-core:

```sh
go run ./cmd/migrate
```

The command installs an empty database or validates/upgrades an existing document installation. It rejects extra application tables and incompatible columns. The API performs the same strict validation at startup. It never creates a schema_migrations application table.

## Legacy installations

The SQL in legacy/ is an immutable historical conversion/test source. It is not executed by the normal migration command. Its original migration identities and checksums remain unchanged so existing financial history can be checked correctly.

Never run the legacy SQL manually against a document installation. Never baseline or reset an existing database to make it pass validation.

The backed-up local yember database had no operational records. It was replaced transactionally with the 38-table schema; its 67 configuration/migration records were retained in system_activity_logs. An empty legacy replacement requires a verified custom-format pg_dump backup:

```sh
go run ./cmd/migrate --replace-empty-legacy --backup-file /path/to/verified-backup.dump
```

This command refuses even one existing operational record. Nonempty installations require lossless conversion. internal/contractschema.PrepareLegacy builds a separate schema and checks users, orders, charged prices, receipt labels, addresses, ledger values, payee balances, bank encryption context, refunds and cash history. Some central payout and legacy-provider conversions remain guarded as unsupported; such databases must not be switched or reset.

Integration tests create isolated databases. Existing application/test databases are not deleted by the test helper.

Tables are organized into document-based PostgreSQL schemas; see [database groups](../docs/database-groups.md). Existing public tables are moved transactionally without rebuilding data.

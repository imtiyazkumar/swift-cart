# Platform setup and operating notes

## Workspace

```text
yember/
    yember-core/                  Go/Echo API, workers, migrations, tests
    yember-app/                   npm mobile workspace
        apps/customer/           existing customer UI and native configuration
        apps/restaurant/         restaurant partner entry and workflows
        apps/delivery/           delivery partner entry and workflows
        packages/partner/        shared auth, onboarding, API context and UI
    yember-web/                   independent React/TypeScript admin app
```

Existing uncommitted work was retained. Customer source/config/assets were moved
together, rather than rebuilding the customer application. Existing customer
routing remains; partner workflows use Expo Router tabs and protected routes. The admin
web app uses React Router for pages and TanStack Query for server data.
The persisted restaurant role remains `MERCHANT_OWNER` for schema compatibility.

## Local startup

Ready-to-use development/test profiles and seeded logins are documented in
[local development](local-development.md). Both dedicated local databases are
already migrated and seeded on this machine; the steps below also describe
provisioning another environment.

Use Node 24, Go 1.25 or newer, and PostgreSQL 16 or newer. PowerShell installations
that block npm scripts should use `npm.cmd` and `npx.cmd`.

1. In `yember-core`, copy `.env.example` to `.env` and set a local DSN and a random
   `JWT_SECRET` of at least 32 characters. Never reuse example secrets.
2. For a new empty database, run `go run ./cmd/migrate`. Demo users/catalog are
   excluded unless `--include-demo-seed` is explicitly passed. Do not enable demo
   seeds in production.
3. For an existing untracked database, first back it up and inspect its schema.
   The runner refuses to replay history automatically. Only if migrations through
   020 are already represented in that schema, use:
   `go run ./cmd/migrate --baseline-through 020_refresh_seed_food_image_urls.sql`.
   This records checksums through that file and applies later migrations. Do not
   baseline a partially migrated schema. Applied files are checksum-verified on
   later runs. Each migration and its ledger insert commit together under a lock.
4. Provision the first administrator using process-only `ADMIN_EMAIL` and
   `ADMIN_PASSWORD` (12–72 bytes), then `go run ./cmd/create_admin`. Clear those
   variables afterward. This command never promotes or overwrites an account.
5. Set `CUSTOMER_PAYMENT_METHOD=CASH_ON_DELIVERY` only if that method is actually
   supported by your operation. Otherwise checkout remains disabled. Orders do
   not become paid merely because they were created or delivered. Cash settlement
   and refunds require additional operational workflows before production use.
6. Start the API: `go run ./cmd/api` (port 8080).
7. In `yember-app`, run `npm.cmd install`, then one of:
   `npm.cmd run customer`, `npm.cmd run restaurant`, `npm.cmd run delivery`.
   Partner Metro ports are 8082 and 8083. Set `EXPO_PUBLIC_API_URL` in each app's
   local `.env` to a reachable API including `/api/v1`; on an Android emulator use
   `http://10.0.2.2:8080/api/v1`, and on a device use your development machine's LAN
   address. These variables must never contain secrets.
8. For admin development over HTTP, set `ADMIN_COOKIE_SECURE=false` in Core and
   include `http://127.0.0.1:5173` in `CORS_ALLOWED_ORIGINS`; restart Core. In
   `yember-web`, run `npm.cmd install` and `npm.cmd run dev` (port 5173). The admin
   app requires migrations through `031_review_moderation.sql`. See
   [admin console notes](admin-console.md) and `yember-web/README.md` for routes,
   actions and session behavior. Use secure cookies with HTTPS outside local HTTP
   development.

## Email

Set server-only `RESEND_API_KEY`, a verified-domain `EMAIL_FROM`,
`EMAIL_VERIFICATION_URL`, and `PASSWORD_RESET_URL`. Configure Resend DNS/domain
verification in your own account. Then explicitly set `EMAIL_DELIVERY_ENABLED=true`
when live delivery is authorized. Default is false; queued messages remain visible
to administrators. No live email was sent during implementation.

The worker uses Resend idempotency keys, a database lease, bounded retries and
backoff. Failed messages remain visible through `/admin/email-failures`. Verification
and reset tokens are single-use and stored as hashes in token tables; outbox bodies
contain links until sent, then are cleared. Protect database backups accordingly.
SMTP settings are legacy; password-reset requests now use the Resend outbox.

Partners sign up in their own app, request verification, open the emailed link and
copy its `token` query parameter into the app's verification field. A hosted link
landing page/deep-link handoff is not included. Email verification is required
before partner application submission. Phone verification remains unavailable
until an actual SMS provider is configured; no fake OTP is accepted or delivered.

## Private document storage

For local development, set `DOCUMENT_STORAGE_BACKEND=local` and
`DOCUMENT_STORAGE_DIRECTORY=.private-documents`. For production, select `s3`, set
`DOCUMENT_S3_BUCKET` and `AWS_REGION`, and supply AWS workload identity or standard
server-only AWS credentials. Optional `DOCUMENT_S3_ENDPOINT` supports compatible
object stores. Configure a private bucket with public access blocked, encryption,
least-privilege object access and retention appropriate for your jurisdiction.

Uploads accept sniffed PDF/JPEG/PNG up to 10 MiB, use UUID keys, and store only
metadata in PostgreSQL. Downloads require an owner/admin bearer token. Backend
selection is retained per document, so switching the default does not make old
objects public. No real documents were uploaded. Malware scanning and jurisdiction-
specific document types/expiry policies remain production requirements.

Food images use a separate public catalog endpoint after owner-authorized upload;
they never expose private document IDs. Set `PUBLIC_API_BASE_URL` to the externally
reachable API URL including `/api/v1`. Validated JPEG/PNG files are limited to
5 MiB and 10,000 pixels per dimension. The endpoint serves only active, approved,
paid restaurants. Menu prices accept at most two decimal places and convert to
integer minor units with rounding.

## Onboarding fees and Stripe

An administrator must configure the amount in minor units, 3-letter currency and
payment/tax terms in Platform settings. No production fee is invented. Existing
checkout sessions retain their original fee snapshot.

Set server-only `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, `PAYMENT_SUCCESS_URL`
and `PAYMENT_CANCEL_URL`. Use Stripe test mode first and ensure the account supports
your operating country/currency. Register `/api/v1/webhooks/stripe` for
`checkout.session.completed` and `checkout.session.async_payment_succeeded`.
Use HTTPS in production. The approved owner starts hosted checkout from the
restaurant app. Redirecting back does not confirm payment: only a timestamp-checked
HMAC-verified webhook with matching session, merchant, amount and currency can
mark the fee paid and activate an approved restaurant.

Webhook events and checkout creation are idempotent. Expired/abandoned checkout
sessions currently require operator reconciliation; automatic session replacement,
refunds, invoice exports and tax calculation are not implemented. Do not charge
real customers until your financial policies and these operational cases are ready.
No real payment or provider checkout request was made during implementation.

## Orders and assignment

The backend owns prices, item snapshots and transitions. Restaurants explicitly
accept, prepare and mark ready. Offers can begin after acceptance, but pickup
requires `READY_FOR_PICKUP`. One database scheduler lock coordinates replicas;
eligible online/approved partners with vehicles rotate by `last_offered_at`.
Offers expire after five minutes. Each partner is tried once per order; exhaustion
leaves the order unassigned for intervention. New eligible partners can still be
considered. Active assignment indexes prevent multiple orders per partner and
multiple partners per order. No GPS tracking is claimed.

Administrators can cap offers per order under Platform settings. Zero (the default)
tries each eligible partner once; a positive value stops after that many offers.
The five-minute offer timeout remains fixed to the required workflow.

Status polling is used by clients (10–30 seconds); no push or WebSocket delivery
is claimed. A restaurant rejection cannot silently discard a paid order. Delivered
orders accept one restaurant review and optional partner rating from their owner.

## Validation and builds

```powershell
# yember-core
go test ./...
$env:YEMBER_INTEGRATION='1'
go test ./... -count=1
go vet ./...
go build ./cmd/api

# yember-app
npm.cmd run typecheck
npm.cmd run lint
npm.cmd run export --workspace @yember/customer
npm.cmd run export --workspace @yember/restaurant
npm.cmd run export --workspace @yember/delivery

# yember-web
npm.cmd run build
```

Integration tests read only the local database connection configuration, create
uniquely named `yember_test_*` databases, apply all non-demo migrations and remove
only those disposable databases afterward. Existing application data is untouched.
Provider tests use local HTTP servers, not live accounts.

All mobile apps have distinct bundle/package identifiers and EAS profiles. Native
directories use Expo Continuous Native Generation; they are not hand-written.
For native releases, configure your Expo project and signing in each app and use
`npx.cmd eas-cli@latest build --profile production --platform android` or `ios`.
These cloud builds can incur costs and require separate authorization. JavaScript/
Hermes exports are not signed APK/AAB/IPA builds. Xcode is unavailable on Windows;
Android SDK/device and signing verification are not complete in this environment.

## Migration rollback and production limits

Migrations 021–030 add operational data and constraints. Conflicting legacy
assignments/reviews deliberately make unique-index creation fail; resolve those
records manually after backup instead of deleting user data automatically. Roll
back application deployment first; retain additive tables/columns and backups.
Do not drop payment, audit, document or delivery tables to roll back code. PostgreSQL
enum values cannot be removed safely in a normal down migration. Restore a verified
backup to a separate database if data rollback is required.

Production is not certified: live email/Stripe/S3 configuration and verification,
native signing/builds, browser/device end-to-end QA, audited production migration,
jurisdiction policies, malware scanning, dependency advisory remediation, distributed
rate limiting, financial reconciliation/refunds, advanced operational interventions,
advanced assignment retry policies and menu modifiers/variant management still
need work. Current auth rate limits are per API instance. No deployment was performed.

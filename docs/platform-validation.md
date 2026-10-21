# Validation results — 2026-10-21

## Passed locally

- Dedicated `yember_local` and `yember_local_test` databases were created,
  migrated and seeded. Both Core profiles have real ignored env files; every client app
  also has development/test env files and tracked templates.
- Seeder integration tests verify bcrypt logins for ADMIN, CUSTOMER,
  MERCHANT_OWNER and DELIVERY_PARTNER, complete linked profiles, repeat-run
  idempotency and preservation of account changes. Local-only target guards pass.
- HTTP smoke checks against the test API verified all four seeded password
  logins, role identities, profile endpoints, all admin resource lists, dashboard
  data, admin cookie restoration and logout. The owned test server was stopped.
- `npm.cmd test` in `yember-web`: eight tests passed, including the homepage
  dashboard, fulfillment links, audit activity and refresh. The strict TypeScript
  production build also passed after the dashboard changes.
- The old API process on port 8080 was replaced with the current compiled Core
  API. Admin session, seeded administrator login and dashboard requests passed
  through the running Vite proxy on port 5173. A missing session route now shows
  explicit restart/proxy guidance instead of the generic Not Found message.
- Vite configuration resolution verified development 5173 -> API 8080 and test
  5174 -> API 8084.

- `YEMBER_INTEGRATION=1 go test ./... -count=1`: all backend test packages passed.
- `go vet ./...`: passed.
- `go build -o .test-cache/yember-core-validation.exe ./cmd/api`: passed.
- `npm.cmd run typecheck` in the mobile workspace: all three apps and the shared
  partner package passed strict TypeScript checks.
- `npm.cmd run lint`: passed with six existing customer warnings (unused bindings
  and an axios import warning); no lint errors. Shared partner source also passed
  a direct ESLint check.
- Prettier checks on new partner/admin source and rewritten customer detail/review
  screens: passed using four-space indentation.
- All three `expo export --platform all` commands: Android/Hermes, iOS/Hermes and
  web bundles exported successfully for customer, restaurant and delivery.
- `npm.cmd run build` in `yember-web`: strict TypeScript and Vite production build
  passed. Bundler emits TanStack Query `use client` directive warnings for this
  browser-only app.
- Local admin HTTP check returned 200.
- Existing-repository `git diff --check`: passed (Git reports normal LF/CRLF
  conversion notices).

## Real PostgreSQL workflows exercised

Tests created uniquely named disposable `yember_test_*` databases, applied all
non-demo SQL migrations through 031 plus the legacy auth migration, and removed
only those disposable test databases. The two intentionally persistent local
fixture databases are retained for development and manual testing.

- Partner signup, password login and server-side role restrictions.
- Single-use email verification using the real token and outbox tables.
- Authenticated multipart private-document upload and owner download.
- Application submission, duplicate-submission rejection, review authorization,
  required rejection reason, atomic audit/email insertion and approval without
  premature restaurant activation.
- Refresh-token rotation, old-token replay rejection, logout revocation and
  disabled-account access rejection.
- Checkout with authoritative prices, idempotent retry and immutable item/name
  snapshots after catalog changes.
- Owner-authorized menu image upload and validation using a generated test image.
- Restaurant accept/prepare/ready transitions and invalid transition rejection.
- Round-robin delivery offers, five-minute expiry, next-partner selection and
  eight concurrent acceptance attempts producing exactly one assignment.
- Pickup only after ready, delivery only after pickup, repeat delivery rejection.
- Completed-order restaurant and partner ratings and duplicate-review rejection.

## Provider boundaries exercised with local HTTP servers

- Resend request authentication, event idempotency key and provider-failure handling.
- Stripe hosted-checkout creation, configured amount/currency, idempotent session
  reuse, webhook HMAC/timestamp checks, tampered/stale signatures, wrong payment
  amount rejection and duplicate-event-safe activation/email confirmation.
- S3 signed PUT/GET/DELETE using the official AWS SDK against an isolated local
  HTTP server; no public-read ACL. Local storage traversal/overwrite prevention.

No live Resend, Stripe or S3 request was made. Provider test doubles validate the
integration boundary, not live account, DNS, webhook routing or bucket policy.

## Not verified / remaining work

- Browser visual or interactive QA: browser inventory was empty; the attempted
  in-app preview reported the browser unavailable. HTTP 200 is not browser QA.
- Mobile emulator/device UI tests and signed APK/AAB/IPA builds were not run.
- Docker builds/deployment and GitHub-hosted CI were configured but not run.
- The existing application/production databases were not migrated or reset.
  Dedicated local development/test databases were created and migrated separately.
- Live email domain delivery, payment checkout/webhooks and S3 bucket permissions
  require the owner's configuration and authorization.
- Mobile `npm audit` reports 32 advisories (22 high, 10 moderate), including
  transitive Expo/Metro/React Native/NativeWind dependencies. Some automated fixes
  suggest incompatible major downgrades; these were not applied blindly. The new
  admin dependency installation reported zero advisories.
- Advanced refunds/reconciliation, abandoned Stripe session recovery, operational
  financial policies, distributed rate limits,
  jurisdiction-specific document requirements/expiry and scanning, automated
  operating-hours enforcement, and advanced menu modifiers remain incomplete.

This is a functional local implementation with tested core workflows. It does not
satisfy the brief's full production definition of done yet. See `platform-setup.md`
for configuration, launch/build commands, compatibility decisions and rollback
guidance.

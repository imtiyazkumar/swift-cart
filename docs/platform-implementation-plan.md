# Multi-vendor platform implementation

## Approved layout

The workspace root contains only three application directories: `yember-core`
(existing Go/Echo/PostgreSQL backend), `yember-app` (npm workspaces with customer,
restaurant and delivery Expo apps), and `yember-web` (independent admin web app).
Shared mobile packages live inside `yember-app/packages`.

## Audit, 2026-10-21

- Both existing repositories have substantial uncommitted work. Preserve it.
- Backend: Go 1.25 module, Echo 4, pgx 5, PostgreSQL, handwritten SQL migrations.
- Customer: Expo 57, React 19.2, React Native 0.86, TypeScript 6, npm lockfile,
  TanStack Query 5. Preserve compatible installed versions.
- Existing domains include customer checkout, catalog, merchants, restaurant,
  rider, delivery, admin, payments and notifications; reuse these.
- Admin, restaurant and rider statistics contain fabricated error fallbacks.
- Restaurant/rider handlers trust client resource IDs in several writes.
- Signup always creates a customer; partner onboarding is incomplete.
- Approval immediately activates merchants without payment; rejection reasons
  and atomic decision audit records are missing.
- Delivery acceptance is not exclusive; transitions do not verify prior states.
- JWT refresh is stateless; role claims can outlive role changes.
- Migration runner replays every file, including an old destructive migration.
- Local PostgreSQL and Go/Node are available; Docker is not on PATH. Windows
  cannot run Xcode. Provider setup must remain explicit, never simulated.

## Milestones and validation

1. Preserve existing work, introduce the approved folder layout and independent
   entries, retain the existing customer experience; strict type checks.
2. Harden identity, ownership and approval paths; additive migrations, backend
   tests, isolated test database. Prevent approval from bypassing payment.
3. Admin app: authenticated server-enforced access, real dashboard, applications,
   decisions with reasons, users, order search and audit history; production build.
4. Partner apps: real API authentication, onboarding, restaurant menu/order
   workflows and delivery availability/offers/milestones; independent bundles.
5. Reliable delivery scheduling, transactional transitions, email outbox,
   provider integration boundaries, ratings and failure/concurrency tests.
6. Setup/build/operations documentation and actual validation results. Record
   provider credentials, legal policies and native signing/build limitations.

No deployment, external paid activity, live emails, real payments or changes to
an existing production database are authorized. Use isolated local test data.
New workflows use conditional rendering rather than adding navigation libraries;
the existing customer router is retained for compatibility.

## Implemented and verified

The approved three-directory layout, three mobile entries, admin web app, partner
onboarding, private local/S3 documents, transactional review and audit, Resend
outbox, Stripe onboarding boundary, menu/profile/hours editing, order processing,
round-robin delivery, ratings and safer migration tracking are implemented.
Current verification and remaining scope are recorded in `platform-validation.md`.
The full production definition of done remains open; do not infer completion from
successful compilation or bundles.

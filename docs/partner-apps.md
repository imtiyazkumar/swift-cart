# Restaurant and delivery partner workspaces

The two partner apps now use Expo Router with protected routes, five bottom tabs,
stack detail screens, and shared native UI. TanStack Query handles server state,
cancellation, focus/reconnect refresh, bounded polling and mutation invalidation.

## Product research

Reviewed official publisher listings on 2026-10-21:

| App                                                                                                               | Relevant advertised capabilities                                                       | Yember implementation                                                                                       |
| ----------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| [Swiggy restaurant partner](https://play.google.com/store/apps/details?id=in.swiggy.partnerapp&hl=en)             | Orders, menu/photos, customer feedback and business insights                           | Orders, menu stock/prices/photos, reviews/responses and business trends                                     |
| [Zomato restaurant partner](https://play.google.com/store/apps/details?id=com.application.services.partner&hl=en) | Order fulfillment, catalog availability, sales metrics, outlet settings and help cases | Fulfillment stages, catalog management, gross sales, outlet profile/hours and support conversations         |
| [Swiggy delivery partner](https://play.google.com/store/apps/details?id=in.swiggy.deliveryapp&hl=en)              | Flexible availability, delivery work, recorded earnings and order support              | Online/offline availability, time-limited offers, active deliveries, earnings ledger and contextual support |
| [Zomato delivery partner](https://play.google.com/store/apps/details?id=com.zomato.delivery&hl=en)                | Flexible work, earnings visibility, account verification and support                   | Availability controls, earnings periods, reviewed partner identity and help/account screens                 |

These workflows informed Yember's information architecture. Competitor payout,
insurance, incentive, advertising and referral claims do not describe Yember.
Yember does not display invented payout balances, bonuses, insurance or heatmaps.

## Restaurant app

- **Home:** outlet availability, activation state, live queue counts, seven-day
  delivered sales, published ratings and links to daily operations.
- **Orders:** searchable, paginated active/history queues with status filters.
  Detail shows immutable item snapshots, order timeline and assigned courier.
  Accept, reject with a reason, prepare and ready actions update the real order.
  Ready confirmation prevents a single accidental tap from advancing the order.
- **Menu:** category filters, search, stock switches, category/dish creation,
  price/description/vegetarian editing and authenticated food-photo uploads.
- **Business:** 7/30 UTC-day delivered gross sales, order value/counts, cancelled
  orders, daily trends and best-selling items from delivered snapshots.
- **Account:** contact details, outlet profile, primary branch address, cuisines,
  weekly operating schedule, verification status, password changes and sign-out.
- **Related routes:** published reviews with saved restaurant responses, help
  center, support cases/conversations, onboarding and recovery/verification links.

The current partner profile selects one outlet. Multi-outlet switching, staff
invitations, timed catalog/modifiers, restaurant-funded advertising/discounts and
automated schedule enforcement remain separate product work. The stored weekly
schedule does not override the live Home availability switch.

## Delivery app

- **Home:** availability, real expiring offers, active assignment count, recorded
  earnings, published rating and explicit foreground location-sharing controls.
- **Deliveries:** active/history lists and a dedicated task screen with pickup
  and drop-off information, map navigation, active contact actions, item snapshots
  and timeline. Confirmed pickup, start-delivery and handoff actions follow the
  server state machine. Customer contact/address is withheld on completed jobs.
- **Earnings:** 7/30 UTC-day recorded delivery fees/tips, daily trend and paginated
  ledger. It never labels the ledger as a bank payout or withdrawable balance.
- **Help:** FAQs, order-linked cases, durable conversation threads and follow-ups.
- **Account:** identity/contact editing, reviewed vehicle/eligibility information,
  requests to update verified data, password changes and sign-out.

Location permission is requested only after the user enables sharing. Sharing is
limited to the foreground and paused when there is neither online availability
nor an active assignment. Permission denial and upload errors are visible.
Background location, push/voice alerts, proof of delivery, masked calling and
cash reconciliation still require dedicated implementation and device validation.

## Account and operational safeguards

Native refresh credentials use Expo SecureStore. Access credentials remain in
memory, and web previews keep credentials in memory. The client rotates refresh
tokens once across concurrent expired requests, validates the app-specific role,
clears private caches on sign-out, and rejects refresh results arriving after
sign-out. Startup network failure preserves the saved credential for retry.
Requests time out and mutations are not automatically retried.

Partner data is scoped to the authenticated user. Clients cannot choose another
owner/courier by submitting IDs. Support creation uses a per-user request key,
limits open cases and verifies ownership of linked orders. Administrators can
reply from the web support inbox and resolve/reopen cases with an audit reason.
Partner follow-ups reopen resolved conversations.

Admin support messages and ticket status are stored in migration 032 tables.
Account/password writes commit atomically; password changes revoke refresh
sessions. The server still requires current roles, active accounts, partner
approval and applicable onboarding/vehicle eligibility before operational writes.

## Run and validate

Migration `032_partner_workspaces.sql` is required. It has been applied to
`yember_local` and `yember_local_test` only. After upgrading the API, restart Metro
because both partner apps now enter through `expo-router/entry`:

```powershell
cd C:\workspace\imtiyaz\projects\yember\yember-app
npm.cmd run start --workspace @yember/restaurant -- --clear
# Separate terminal in the same directory:
npm.cmd run start --workspace @yember/delivery -- --clear
```

The existing per-app public API env settings and seeded logins still apply.
For a phone, configure the computer's reachable LAN address. Production API
endpoints need HTTPS. Native verification/reset links use the installed app's
scheme and require a development/release build that registers it; Expo Go testing
needs a matching Expo development link instead.

Server-side email link targets are selected by the stored account role:
`RESTAURANT_PASSWORD_RESET_URL`, `DELIVERY_PASSWORD_RESET_URL`,
`RESTAURANT_EMAIL_VERIFICATION_URL`, `DELIVERY_EMAIL_VERIFICATION_URL`.
Customer link settings retain their existing behavior. Email delivery still
requires configured provider credentials and explicit delivery enablement.

```powershell
npm.cmd test
npm.cmd run typecheck
npm.cmd run lint
npm.cmd run export --workspace @yember/restaurant
npm.cmd run export --workspace @yember/delivery
```

The six session tests cover role rejection, native credential selection, refresh
concurrency, sign-out races (including delayed SecureStore writes) and transient
startup errors. PostgreSQL integration tests exercise analytics, history/pagination, scoped order details, contact
restriction, review replies, support ownership/idempotency/operator replies,
follow-up reopening, account edits and password/session invalidation. Availability
tests verify eligibility gates and that going offline expires pending offers
while preserving accepted assignments.

Exports validate Android/iOS Hermes and web bundling; they do not establish device
interaction, location accuracy, app-store signing or production payment safety.
Signed builds, device/permission tests, production observability, external provider
configuration and operational financial policies remain release prerequisites.

Operational station setup and strict coverage contracts: [stations](stations.md).

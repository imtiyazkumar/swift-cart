# Yember customer flow audit and implementation plan

**Scope:** Read-only audit of the `yember-core` repository on 2026-10-06. The repository contains the Go/Echo backend and SQL migrations, but no `yember-app` source tree, mobile package manifest, UI screens, or frontend tests. The mobile frontend therefore could not be inspected or exercised. Findings below describe the backend contract the app can currently call and the server-side risks visible in code.

## Implementation update (2026-10-06)

The first backend implementation pass has since addressed part of the audit:

- Public signup now creates only `CUSTOMER` accounts and provisions a matching customer row. Access and refresh JWTs have distinct token types; refresh tokens are rejected by access middleware.
- OTP sending fails closed with `503` until a real delivery provider exists; fixed-code acceptance was removed. Secondary-contact verification requires the caller's own bearer-authenticated identity.
- Customer, admin, restaurant, rider, merchant, order, catalog, delivery, and payment route groups now require bearer authentication; persona groups have role gates. Customer profile/address/favorite/tracking/review operations derive identity from the token or check ownership. Admin-only order listing/status/delete routes are restricted.
- Restaurant search now binds the search term. Unsupported storefront ratings/ETA/offer/image data are being returned blank rather than fabricated.
- Customer cart and checkout endpoints were added. Cart price and merchant are resolved on the server; checkout validates saved-address ownership and item availability, locks checkout rows, creates item/address snapshots and initial status history transactionally, and accepts an idempotency key. This checkout currently supports INR only and has no tax, delivery fee, coupon application, inventory reservation, or gateway confirmation.
- Migration `015_customer_checkout.sql` adds the delivery-address snapshot, cart-line uniqueness, and role-name normalization. `docs/api.md` now describes implemented routes and remaining limits.

**Still outstanding:** the `015` migration has not been applied to a database; no integration flow or tests were run; OTP delivery, refresh rotation/revocation, gateway creation/webhooks/refunds, customer order history, actual delivery pricing/coverage, merchant membership checks, broader status transition enforcement, and the separate mobile frontend remain incomplete. Because earlier signup accepted caller-chosen roles, operators must audit existing privileged accounts and rotate `JWT_SECRET` at rollout to invalidate already-issued tokens.

## Executive assessment

The backend has pieces for restaurant discovery, menu reads, customer profiles/addresses, orders, and payment-intent rows, but does not implement a production-ready customer purchase flow. The existing API can create an order using client-supplied customer/merchant IDs and total, then create an unrelated payment intent. It does not create or persist cart lines, validate current item prices/availability, associate a delivery address, authorize ownership, or verify payment. Treat checkout and payment as **not operational** until the P0/P1 work below is complete.

## Flow audit

| Customer step | What exists | Audit result |
| --- | --- | --- |
| Sign up / sign in | Email/password signup/login, OTP endpoints, JWT access/refresh, `/auth/me` | Unsafe for customer launch: signup accepts the role; OTP is the literal `1234` and verification accepts that value regardless of a stored code; refresh tokens are not distinguished from access tokens and are not rotated or revoked. Only `/auth/me` is wired with JWT middleware. |
| Home / restaurant discovery | `GET /api/v1/customer/restaurants` | Route exists. `city` is accepted but ignored. Search text is interpolated into SQL, creating SQL-injection risk. Rating, review count, delivery time, distance, fee, offer, image, and accent are constants, not merchant data; list fetches each merchant's menu individually (N+1). |
| Restaurant detail | `GET /api/v1/customer/restaurants/:id` | Returns restaurant and menu, but the same fabricated metrics/image are used. Menu item image, tag, rating, and review count are also constants. No restaurant hours, delivery coverage, real availability context, or category grouping in this response. |
| Dish detail | `GET /api/v1/catalog/items/:id`, merchant item list/search; menu items embedded in customer restaurant response | An item read exists, but no dedicated customer dish endpoint or modifier/variant selection contract. The customer menu model omits category, currency, variants, add-ons, and some catalog fields. No reliable item image is stored in this schema/API. |
| Cart | Cart/cart-item tables exist in migration 006 | No cart API or cart service. No add/update/remove/clear, single-merchant constraint at API level, price refresh, stock validation, or cart persistence. |
| Delivery address | Address table and add/list repository methods exist | Create route exists; list method is not routed. Customer identity is caller supplied/fallback, address fields are defaulted to Srinagar values, and saving address and updating legacy customer address are separate unchecked writes rather than one transaction. No update/delete/default-address API or geocoding/delivery-zone validation. |
| Checkout / order | `POST /api/v1/order/create` with merchant ID, customer ID, total; order read/list/status/delete | No checkout operation. Total is client supplied; zero/negative totals are not checked in service; no order lines, address, tax/fee/coupon snapshot, idempotent submission, inventory reservation, or transactional state/history write. List/read/status/delete APIs are unscoped and unauthenticated. Status validation checks membership only, not legal transitions. |
| Payment | `POST /api/v1/payments/intents` inserts a row with defaults | Persists an intent record only. No order ownership/existence/amount match check, gateway call, client confirmation, signed webhook route, payment status read, retry/reconciliation, refund, or order state transition. Idempotency key is optional and unique constraint behavior is not translated into a safe replay response. |
| Confirmation / tracking / history | Customer tracking route; order tables and status history exist | Tracking route is unauthenticated and accepts any order ID. No customer order history endpoint. Rider coordinates/contact can be exposed by the tracking query without checking that the requester owns the order. Realtime hub is in-process and has no client-facing WebSocket contract. |

## Findings by priority

### P0 — baseline issues (the implementation update above resolves some)

1. **Unauthenticated access and missing ownership/role checks.** Only `GET /api/v1/auth/me` has JWT middleware (`internal/auth/handler.go`). Customer profiles, addresses, favorites, notifications, reviews, tracking, orders, payments, merchant, rider, and admin handlers are registered without auth/RBAC middleware (`cmd/api/main.go`, respective `RegisterRoutes` handlers). Several accept a user/customer/order ID from the request. This permits unauthorized data reads and mutations, including admin operations and order status changes.
2. **Public privilege escalation.** `POST /auth/signup` accepts the caller's role and `NewUser` stores it directly (`internal/auth/handler.go`, `internal/auth/model.go`). Since tokens inherit that role, a public caller may register with a privileged role; admin routes also lack role checks.
3. **OTP authentication is bypassable.** `/auth/send-otp` stores literal `1234`, and verify endpoints accept `1234` even when DB verification failed (`internal/auth/handler.go`). OTP send logs the full identifier; there is no delivery provider, attempt throttling, or resend policy. This is effectively public account creation/login for a known phone/email.
4. **Order/payment flow trusts caller data and is not connected.** Order create accepts caller-selected customer ID, merchant ID and total; payment intent accepts caller-selected order ID and amount. There is no trusted price calculation, owner check, payment provider interaction, webhook verification, or transaction linking successful payment to order state. Do not expose this as a real checkout.
5. **Restaurant search SQL injection.** `ListRestaurants` appends unescaped `search` using `fmt.Sprintf` into SQL (`internal/customer/repository.go`). Bind the search parameter; validate wildcard behavior separately.

### P1 — required for a complete usable flow

6. **Refresh token is not a separate credential.** Access and refresh use the same claims and parser without a token type/audience; refresh returns only a new access token and does not rotate/revoke/persist a refresh session. The DB refresh-token/session tables are not used by these handlers.
7. **Cart and checkout APIs are missing.** Cart schema exists but has no handlers/service/repository. Existing order API cannot persist purchased items or reconstruct a receipt.
8. **Customer identity is not mapped safely.** OTP creates/loads `public.users`; customer endpoints use `public.customers` IDs. No guaranteed customer profile is provisioned/mapped to the authenticated user. In `CreateCustomer`, a new ID is generated but repository insert does not use it, and customer update/address paths update both tables without a transaction and ignore some DB errors (`internal/customer/repository.go`).
9. **Addresses cannot complete checkout.** Address listing is not routed, create silently substitutes location-specific defaults, and update/default/delete/delivery eligibility are absent. Request identity can be overridden by body/query IDs.
10. **Order state is not a state machine.** Any known status may be assigned to any order, including `DELIVERED` or `REFUNDED`; writes do not append `order_status_history` or check affected-row count. Public delete can cascade order/payment history.
11. **Amounts have inconsistent units/types.** Catalog item price and payment amount are integer minor units (`BIGINT`); customer menu divides catalog price by 100 into a float; order total is `float64` backed by `NUMERIC(12,2)`. Currency/unit semantics are not uniform. Do not calculate checkout totals in the client or mix these values.
12. **Discovery and dish data is partly fabricated.** Static ratings/reviews, timing, distance, delivery fee, offers, image, accent and item attributes are sent as if live. `city` is ignored and restaurant lists include a menu fetch per merchant. Remove fabricated values or clearly define real source/nullable fields and improve query shape.
13. **Coupons are not checkout-bound.** Coupon validation takes caller-provided amount and returns a discount, but checkout does not revalidate or snapshot it; no usage limits/customer eligibility/order association are enforced by the order flow.
14. **Error semantics can hide failures.** Some customer repository paths replace read failures with empty menu/favorites responses; address writes ignore insert errors; several HTTP handlers return 500 for missing records or success without confirming rows affected. This can make UI display false success or misleading empty states.

### P2 — product readiness and operations

15. **API documentation is stale/inconsistent.** `docs/api.md` lists routes/behavior that are absent or differ from actual routes (notably auth OTP, customer profile/addresses, catalog, orders, and admin). `docs/architecture.md` describes carts/state history/payment processing that is not implemented. Keep an OpenAPI contract generated from registered behavior and update app-facing docs with each phase.
16. **No mobile frontend in this workspace.** This repo has no `yember-app` source. UI navigation, state management, error/loading/empty states, local cart behavior, token storage, and accessibility remain unaudited. The current root `/` HTML is an API status page, not the customer app.
17. **No pagination/limits for major list endpoints.** Restaurant, catalog, customers, and orders can return unbounded collections. Add pagination and stable ordering before scale.
18. **CORS allows every origin** (`cmd/api/main.go`). Configure explicit app/admin origins per environment. The global in-memory rate limiter is process-local; OTP/auth need purpose-specific, shared limits.

## Proposed implementation plan

Sequence is intentional: secure identity and authoritative data before wiring the client to purchase.

### Phase 0 — contract and launch boundary

- Confirm customer login method (phone OTP, email/password, or both), money representation (recommend integer minor units plus ISO currency end to end), delivery geography, and payment provider/account.
- Treat current API as development-only for customer purchase; remove the static OTP fallback and stop exposing privileged signup before connecting the app.
- Publish one versioned OpenAPI contract. Mark absent capabilities as absent rather than reflecting aspirational docs.

### Phase 1 — authentication, identity, and authorization (P0)

- Make public signup customer-only; provision staff roles through controlled admin/onboarding workflow. Normalize and validate email/phone.
- Implement real OTP delivery with expiring single-use codes, attempt/resend limits, enumeration-resistant responses, and no secret/identifier logging. Remove all fixed-code fallbacks.
- Give access and refresh tokens distinct claims/types; rotate refresh credentials, persist/revoke sessions, enforce expiry, and support logout/revocation. Check account status on auth.
- Add authentication and role middleware to every protected route. Resolve customer/merchant/rider records from authenticated user membership, never caller-supplied IDs. Add object-level ownership checks to profile/address/order/payment/tracking reads.
- Restrict CORS to configured app origins and give OTP/login dedicated distributed rate limiting.

### Phase 2 — truthful storefront APIs

- Define customer restaurant listing and detail response from persisted merchant/branch/hours/location/availability data. Implement city/coordinate filters, search with parameterized SQL, pagination, and real delivery eligibility/fees/ETA sources.
- Implement categories and dish detail including images, currency/minor-unit price, availability, variants/modifiers/add-ons, and dietary metadata. Remove fabricated ratings/offers/metrics or provide nullable fields backed by real aggregates.
- Add indexes/query strategy; replace per-restaurant menu N+1 with bounded/explicit menu endpoints where appropriate.

### Phase 3 — customer profile, addresses, and cart

- Ensure authentication creates or links exactly one customer profile. Add `/customer/me` and self-scoped profile update.
- Provide address list/create/update/delete/default endpoints with full address/coordinates and delivery-area validation. Make address writes atomic and remove hard-coded geographic defaults.
- Implement authenticated cart create/read/add/update/remove/clear. Enforce one merchant per active cart (or explicit replace-cart flow); validate item/variant/modifier combinations and availability on every mutation; never trust client unit prices.

### Phase 4 — authoritative checkout and order lifecycle

- Add checkout preview that prices cart server-side: line snapshots, discounts, delivery fee, taxes/fees, total, currency, and address eligibility. Define rounding and integer-unit rules.
- Add idempotent checkout that revalidates inventory/prices/coupon/address in a DB transaction; reserves stock if enabled; creates order, item snapshots, address snapshot, status history, and payment attempt atomically. Do not accept a client total.
- Scope order history/detail/tracking to the authenticated customer. Add legal transition rules and actor authorization; append status history and check rows affected. Replace customer-accessible hard delete with explicit cancellation policy.
- Publish order lifecycle events through durable/outbox delivery so notifications and realtime views survive process restarts.

### Phase 5 — payment integration and confirmation

- Integrate the chosen gateway to create provider-side payment orders/intents only after server checkout. Return only client-safe provider fields.
- Verify signed webhooks, deduplicate provider events, validate provider/order/amount/currency, and atomically update payment/order states. Add payment status query, retry/reconciliation, cancellation/refund flows, and idempotency semantics.
- Keep order confirmation dependent on verified provider outcome (or a deliberately defined cash-on-delivery path); never infer success from intent creation/client callback.

### Phase 6 — mobile integration and end-to-end verification

- In `yember-app` (not present in this repository), implement auth/session expiry, location-aware home/discovery, restaurant/menu/dish screens, one-merchant cart, address selection, checkout quote, payment handoff, order confirmation/tracking/history, and profile.
- Add explicit loading/empty/offline/error/retry states; never render demo values as persisted data. Keep cart UX resilient to stale prices/availability and require customer review when checkout quote changes.
- Verify the contract across app and API with integration tests covering authorization, price tampering, duplicate checkout/webhooks, stock changes, coupon validity, payment failure/success, and order ownership. Exercise the complete real-provider sandbox journey before production.

## Acceptance gates

- Anonymous requests cannot access customer data, orders, tracking, payments, or operational/admin endpoints; one customer cannot act on another customer's records.
- Public signup cannot create privileged roles; OTP cannot be guessed/reused and is delivered through the configured provider.
- Restaurant/item data shown as factual is sourced from persisted data; all search inputs are parameterized.
- Cart and order totals are computed server-side in one defined currency unit; order records include immutable line and address snapshots.
- Repeated checkout and repeated payment webhooks do not duplicate orders/charges/state changes.
- Only a verified payment outcome can mark an online-payment order paid/confirmed; customer can query their own order and payment state.
- Mobile frontend is reviewed in its own repository with the same API contract and completed end-to-end flow. This gate cannot be assessed from `yember-core` alone.

## Audit method and limits

Inspected route registration, handlers, services, repositories, models, migrations, and existing API/architecture/frontend-agent docs. No source files were changed by the audit except this report. Tests and live database/API flows were not run. The findings are code-level observations; deployment configuration, gateway account setup, production schema state, and the separate mobile app require their own inspection.

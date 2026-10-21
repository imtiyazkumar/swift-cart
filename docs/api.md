# Yember Core API

Base path: `/api/v1`. This document describes the implemented routes, not planned routes. JSON list responses are arrays unless otherwise stated. Protected routes require `Authorization: Bearer <access_token>`.

## Health

- `GET /` serves the API status page.
- `GET /healthz` reports process health.
- `GET /readyz` checks database readiness.

## Authentication

- `POST /auth/signup` accepts `email`, `password`; public accounts are always `CUSTOMER`. Returns the new ID, email, and role. This endpoint does not log the user in.
- `POST /auth/login` accepts `email`, `password`; returns `access_token` and `refresh_token`.
- `POST /auth/forgot-password` accepts `email` and sends a 30-minute reset link when SMTP is configured. The response does not reveal whether the account exists.
- `POST /auth/reset-password` accepts `token`, `password` (at least 8 characters); reset tokens are single-use and stored hashed.
- `POST /auth/refresh` accepts `refresh_token`; returns a new access token. Refresh tokens cannot be used as bearer access tokens.
- `GET /auth/me` returns the current user and requires an access token.
- `POST /auth/send-otp` currently returns `503` until an SMS/email provider is configured. There is no fixed or demo OTP.
- `POST /auth/verify-otp` accepts only a valid, unexpired, unused OTP row; no public endpoint currently delivers one.
- `POST /auth/verify-secondary-otp` requires an access token and a valid OTP.

## Customer (`/customer`)

All routes require a `CUSTOMER` access token. Customer IDs are derived from the token for self-service operations.

- `GET /restaurants?search=` lists active restaurants and matches restaurant names and available dish names. `city` is not yet implemented. Unavailable discovery fields such as rating, ETA, fees, and offers are returned empty rather than fabricated. Menu items expose `image` from `metadata.image_url` when set.
- `GET /restaurants/:id` returns active restaurant data and available menu items.
- `POST /coupons/validate` accepts `code` and `amount`; this is a preview only and checkout does not yet apply coupons.
- `GET /favorites`; `POST /favorites/toggle` with `merchant_id`.
- `GET /orders/:id/track` only returns tracking for the authenticated customer's order.
- `POST /reviews` accepts `order_id`, `rating`, `comment`; order ownership is checked.
- `POST /create` and `PUT /profile` update the authenticated customer's `name` and `gender`.
- `GET /profile/:id` only permits the authenticated customer's own ID.
- `GET /addresses`; `POST /addresses` accepts `label`, `address_line1`, `city`, `state`, `postal_code`.
- `GET /notifications` returns the authenticated user's notification records.
- `GET /cart` returns the active cart and `subtotal` in INR paise (minor units).
- `POST /cart/items` accepts `item_id`, `quantity` (1–99); prices and merchant are resolved server-side. Active carts contain one merchant; clear the cart to switch restaurants.
- `PATCH /cart/items/:id` accepts `quantity` (0 removes the line; 1–99 sets it); `DELETE /cart` clears the active cart.
- `POST /checkout` accepts `address_id`, `idempotency_key`. It re-reads current item prices, verifies address ownership and availability, creates order/item/address snapshots and initial status history transactionally, and returns order ID/status/subtotal. The subtotal is INR paise. There are currently no delivery fees, tax, coupon deduction, stock reservation, or payment confirmation in checkout.

## Payment and orders

- `POST /payments/intents` requires a customer token and accepts `order_id`, `amount` in INR paise, optional `provider`, `method`, `currency`, `idempotency_key`. It verifies order ownership and amount against the order total, then records an intent row. It does **not** contact a payment gateway or verify payment.
- `GET /order/:id` requires a customer token and returns only the caller's order; admins may read any order.
- `GET /order/`, `PUT /order/:id/status`, `DELETE /order/:id`, and legacy `POST /order/create` are admin-only. Customer order creation must use `/customer/checkout`.
- Payment status lookup, gateway callbacks/webhooks, refunds, customer order-history list, and provider reconciliation are not implemented.

## Other ecosystem routes

- `/rider/*`, `/restaurant/*`, `/admin/*`, `/merchant/*`, `/delivery/*`, and catalog write routes require access tokens; persona and admin groups additionally require the matching role. Merchant membership/ownership checks within a role are still incomplete. Do not treat this API as production-ready for merchant/rider/admin use until tenant-level checks are complete.
- Catalog reads require a token; catalog category/item writes require `MERCHANT_OWNER` or `ADMIN`. Item responses include `image_url` when one is set; creating or updating an item can set it, and it is stored in `metadata.image_url`.

## Known limitations

- The customer mobile app is a separate repository and is not included here.
- OTP delivery is disabled until a provider is configured. Password login is available.
- Refresh token rotation/revocation, payment gateway integration, delivery pricing/coverage, coupon application in checkout, inventory reservation, pagination, and merchant-specific authorization remain to be implemented.

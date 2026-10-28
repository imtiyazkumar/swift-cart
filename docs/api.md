# Document API

All routes use /api/v1. Authenticated routes require an active account and a bearer access token. The administrator browser uses an HttpOnly refresh cookie and X-Yember-Admin: 1. Access tokens stay in memory. Related entity identifiers in request/response fields are public UUIDs.

## Authentication

POST /auth/login, /auth/signup, /auth/partner-signup, /auth/refresh, /auth/logout and /auth/verify-otp; POST /auth/send-otp sends a phone challenge through the configured TextBee sender. OTP hashes, attempts and consumption are stored in otp_challenges. GET /auth/me and POST /auth/logout-all require authentication.

Administrator browser routes are POST /auth/admin/login, /auth/admin/session and /auth/admin/logout. No public signup can grant ADMIN.

## Customer ordering

GET/PUT /customer/profile; GET/POST /customer/addresses; PUT/DELETE /customer/addresses/:id.
GET /customer/restaurants and /customer/restaurants/:id require address_id.
GET /customer/cart; POST /customer/cart/items with menu_item_id, optional variant_id, address_id and quantity; PATCH /customer/cart/items/:id; DELETE /customer/cart.
PUT /customer/cart/delivery-address validates ownership and abandons a cart if the new address cannot be served by its restaurant.
POST /customer/coupons/validate provides a preview; POST /customer/checkout revalidates and locks coupon usage and authoritative menu prices. Checkout requires a stable idempotency_key.
GET /customer/orders, /customer/orders/:id and /customer/orders/:id/tracking.
GET /customer/finance/pricing and /customer/finance/orders/:id.
POST /customer/finance/orders/:id/checkout starts the owned order's online payment.

## Restaurant and delivery

POST /onboarding/application creates a restaurant or rider and assesses its registration fee; retries preserve the existing application.
POST /onboarding/documents and GET /onboarding/documents/:id store and authorize private verification documents.
GET /stations lists active service stations.
GET /restaurant/profile; PUT /restaurant/profile; PATCH /restaurant/status.
GET /restaurant/menu/categories; GET/POST /restaurant/menu/items; PUT/DELETE /restaurant/menu/items/:id; PATCH /restaurant/menu/items/:id/availability; POST /restaurant/menu/items/:id/image.
GET/POST /restaurant/menu/items/:id/variants; PUT /restaurant/menu/items/:id/variants/:variant.
GET /rider/profile; PATCH /rider/status; POST /rider/location.
GET /partner/orders is paginated. GET /partner/orders/:id enforces ownership.
PATCH /restaurant/orders/:id/status permits CONFIRMED → ACCEPTED → PREPARING → READY_FOR_PICKUP, or rejection with a reason.
PATCH /rider/deliveries/:id/status permits ASSIGNED → ACCEPTED → PICKED_UP → DELIVERED. A rejected assignment is retained as history.
GET/PUT /partner/account and POST /partner/password manage the signed-in partner.

## Money

POST /restaurant/onboarding-payment or /delivery/onboarding-payment returns a scoped Razorpay checkout URL. Payment never approves the application.
GET /payments/intents/:id; POST /payments/intents/:id/order and /verify.
POST /payments/webhooks/razorpay verifies the signature and fetches authoritative provider state.
GET /partner/finance/registration, /summary, /ledger, /settlements, /bank and /cash-handovers.
PUT /partner/finance/bank encrypts the number and requires new verification.
POST /partner/finance/cash-handovers/:id records the agent's gross handover; POST the same path plus /confirm records the restaurant acknowledgement.

## Administration

GET /admin/contract supplies the 38 workspace definitions. GET /admin/dashboard/stats supplies current totals.
GET /admin/manage/:resource and /:id read records. Resource names use table names with underscores replaced by hyphens. POST, PUT and DELETE are available only for editable entities; deletes retain referenced rows.
POST /admin/manage/orders/:id/assign explicitly assigns an eligible rider.
POST /admin/manage/:resource/:id/verify-bank records independent bank verification.
POST /admin/payouts/restaurant or /agent reserves earning_ids against bank_id. POST /admin/payouts/:kind/:id/confirm records a separately verified bank result.
POST /admin/payments/:id/refunds requires amount_paise, reason and a stable idempotency_key. POST /admin/refunds/:id/reconcile verifies its provider_refund_id.

Financial records and authentication histories are created by their workflows, not arbitrary generic record edits.

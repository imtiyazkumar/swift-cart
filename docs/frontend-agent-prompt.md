# SwiftKart Frontend Agent Brief

This document summarizes the current repository and provides a copy-ready prompt for an AI coding agent to build the SwiftKart web frontend.

## Project Study

### What exists

- SwiftKart is a Go 1.25 modular monolith using Echo, PostgreSQL/pgx, JWT authentication, Zap logging, SQL migrations, and an in-process event bus.
- The API listens on port `8080`; API routes are under `/api/v1`. Local setup is documented in `docs/deployment.md` and `Makefile` (`make docker-up`, `make migrate`, `make run`).
- There is currently no frontend application, JavaScript package manifest, or frontend build pipeline in the repository. The root `/` endpoint serves an API service-status HTML page, not a product UI.
- The principal API/domain packages are `auth`, `merchant`, `customer`, `catalog`, `order`, `delivery`, `payments`, `notifications`, and `realtime`.
- Merchant types are `RESTAURANT`, `GROCERY`, `PHARMACY`, `PET_STORE`, and `FLOWER_SHOP`. SwiftKart is intentionally modeled as Merchant → Catalog → Item, not only as a restaurant/menu/food app.

### Current API surface

All URLs below are relative to `http://localhost:8080` by default. Successful creates generally return `201`; updates/deletes often return `204` with no body. Lists are plain JSON arrays, not paginated envelopes, unless the backend is changed.

| Area | Current routes and behavior |
| --- | --- |
| Health | `GET /`, `GET /healthz`, `GET /readyz`. The latter checks database readiness. |
| Auth | `POST /api/v1/auth/signup` accepts `email`, `password`, optional `role` (defaults to `customer`) and returns `id`, `email`, `role`. `POST /api/v1/auth/login` accepts email/password and returns `access_token`, `refresh_token`. `POST /api/v1/auth/refresh` accepts `refresh_token`, returns a new `access_token`. `GET /api/v1/auth/me` returns `id`, `email`, `role`, `created_at`; it requires a bearer token. |
| Merchants | `POST /api/v1/merchant/create`; `GET /api/v1/merchant/`; `GET /api/v1/merchant/:id`; `PUT /api/v1/merchant/:id`; `DELETE /api/v1/merchant/:id`. Profile fields: `id`, `name`, `email`, `phone`, `merchant_type`, `onboarding_status`, `approval_status`, `created_at`, `updated_at`. Defaults: type `RESTAURANT`, onboarding `PROFILE_CREATED`, approval `PENDING`. Name/email required on create. |
| Customers | `POST /api/v1/customer/create`; `GET /api/v1/customer/`; `GET /api/v1/customer/:id`; `PUT /api/v1/customer/:id`; `DELETE /api/v1/customer/:id`. Fields: `id`, `name`, `email`, `phone`, `address`, `created_at`, `updated_at`. Name/email required on create. |
| Catalog | `POST /api/v1/catalog/categories` with `merchant_id`, `name`, optional `sort_order`; `POST /api/v1/catalog/items`; `GET /api/v1/catalog/items?q=&limit=`; `GET /api/v1/catalog/items/:id`; `GET /api/v1/catalog/merchants/:merchant_id/items`; `PUT /api/v1/catalog/items/:id`. Item fields: `id`, `merchant_id`, `category_id`, `name`, `description`, `item_type`, optional `is_vegetarian`, `base_price`, `currency`, `is_available`, `inventory_tracked`, `substitution_policy`, timestamps. Create requires merchant/category/name; defaults include `STANDARD`, `INR`, `NONE`; base price cannot be negative. Category listing, category update/delete, and item delete routes do not exist. |
| Orders | `POST /api/v1/order/create` accepts `merchant_id`, `customer_id`, `total_price`; `GET /api/v1/order/`; `GET /api/v1/order/:id`; `PUT /api/v1/order/:id/status` with `{ "status": "..." }`; `DELETE /api/v1/order/:id`. Create sets status `CREATED`. Valid statuses: `CREATED`, `PAYMENT_PENDING`, `CONFIRMED`, `PREPARING`, `READY_FOR_PICKUP`, `PICKED_UP`, `OUT_FOR_DELIVERY`, `DELIVERED`, `CANCELLED`, `REFUNDED`. The API model currently returns only order ID, merchant/customer IDs, status, total price, and timestamps. |
| Delivery | `POST /api/v1/delivery/partners/:id/online`; `POST /api/v1/delivery/partners/:id/offline`; `POST /api/v1/delivery/locations` with partner ID and coordinates; `POST /api/v1/delivery/assignments` with order and partner IDs. No partner list/create/profile route exists. |
| Payments | `POST /api/v1/payments/intents` accepts order ID and positive amount, with defaults `RAZORPAY`, `UPI`, `INR`; returns an intent. There are no payment list/detail/refund/settlement routes. |

### Important backend gaps and risks

- The Postman collection and `docs/api.md` confirm that only `GET /api/v1/auth/me` currently requires a bearer token. The merchant, customer, catalog, order, delivery, and payment routes are not protected by JWT/RBAC middleware.
- Signup accepts the role from the request body. There is no `SUPERADMIN` role or secure admin provisioning flow. The schema mentions `ADMIN`, `SUPPORT_AGENT`, `MERCHANT_OWNER`, `DELIVERY_PARTNER`, and `CUSTOMER`, while auth defaults to lowercase `customer`. Do not trust a client-selected role or describe frontend-only route guards as security.
- The authenticated user is not linked by an exposed API to a merchant or customer record. There is no current-user customer profile/order/address endpoint, no merchant membership/tenant endpoint, and no way for the frontend to determine an authorized merchant ID securely.
- Merchant and customer list routes return broad datasets; order listing is also global. Do not use those global responses to power customer/merchant self-service views in a production-safe way. Client-side filtering is not access control.
- The database schema is broader than the HTTP API. It includes customer addresses/saved locations/preferences/wallet, merchant branches/documents/operating hours, catalog variants/modifiers/add-ons, inventory warehouses/stock/reservations/substitution preferences, order items/status history, delivery partner/vehicle/location/assignment records, payments/refunds/transactions/settlements, and notification logs/push tokens. Most have no read/write HTTP routes.
- Categories can be created but not listed via a category API. Merchant discovery/listing, cart persistence, checkout with line items/address, order history scoped to the signed-in customer, payment status/refunds, delivery partner management/tracking reads, notifications, and realtime/WebSocket endpoints are not available.
- The order API accepts only merchant ID, customer ID, and a total; it does not accept cart lines, delivery address, tax, fee, coupon, or payment selection. Do not claim those details are persisted by this endpoint.
- Catalog `base_price` and payment `amount` are integer fields/examples, while order `total_price` is a floating-point API field backed by a decimal database column. Confirm and centralize currency/unit formatting; do not silently assume all monetary fields use the same scale.
- DELETE routes are destructive from the UI perspective; confirm before calling them. Do not assume soft-delete/undo semantics merely because some schema tables have `deleted_at` columns.
- Notifications are triggered internally for order-created events, but there is no notification API. Realtime is an in-process hub, not a frontend WebSocket contract.

## Copy-Ready Prompt

> You are a senior frontend engineer implementing the SwiftKart web application inside this repository. First inspect the existing workspace and follow its instructions. Build a complete, responsive web frontend for the existing SwiftKart Go/Echo + PostgreSQL API. The repository currently has no frontend scaffold, so add the frontend in a clearly isolated directory such as `web/` without breaking the Go API, migrations, Docker setup, or existing docs. Do not stop at mockups: implement working navigation, forms, tables, detail views, API integration, validation, loading/error/empty states, and responsive behavior.
>
> ## Product goal
>
> Deliver one coherent SwiftKart product with three distinct workspaces selected by the signed-in account/context:
>
> 1. **Superadmin console**: platform-wide overview; merchant directory and merchant creation/editing; inspect a merchant’s profile, catalog, customers and orders where the current API permits; customer directory/details; global order operations and status management; delivery assignment operations; payment-intent visibility only where actually available; integration/health visibility. Make it easy to move from a merchant to its catalog and related operational records.
> 2. **Merchant workspace**: merchant overview; manage the merchant’s own profile and catalog items/categories; inspect/manage that merchant’s orders and advance allowed order statuses; provide clear entry points for branches, hours, documents, inventory, analytics, settlements, and delivery coordination, but mark unavailable capabilities as not connected until APIs exist. Never imply that client-side merchant filtering is authorization.
> 3. **Customer storefront/account**: customer-facing discovery and shopping experience; browse/search merchants and products; product details; cart; checkout/payment initiation; order status/tracking view; profile and address management; order history; customer preferences. Connect what can be connected to real routes. For absent endpoints, either provide a clearly labeled demo mode with isolated seeded data or show a useful unavailable/coming-from-API state; never present mock data as successfully persisted real customer data.
>
> Also provide a polished sign-in/sign-up flow, session-expired handling, a role/workspace-aware navigation shell, profile/account controls, and a useful not-found/unauthorized state. The API currently has no secure role provisioning, so do not offer arbitrary privilege escalation through a public role selector. If a developer-only demo role selector is added, gate and label it as demo-only, keep it out of production behavior, and document the limitation.
>
> ## Technical implementation
>
> - Use React + TypeScript + Vite unless a compatible frontend stack has appeared in the repo since this brief was written. Keep the app in its own directory and preserve the existing Go build/runtime paths.
> - Use a maintainable route structure, typed API client, domain types matching the JSON field names, reusable form/table/status components, and explicit loading, empty, success, validation, network, 401, 403, and server-error states.
> - Configure the API base URL through a frontend environment variable with local default `http://localhost:8080`; keep secrets out of browser code. Support the `/api/v1` prefix consistently. The backend currently allows CORS broadly for local development.
> - Use `fetch` or a small existing project-standard client. Centralize bearer-token attachment, JSON serialization, response parsing, and API error normalization. Handle `204 No Content` without attempting JSON parsing. Support auth refresh using the refresh endpoint, and on refresh failure clear the session and return to sign-in. Do not log tokens or credentials.
> - Treat frontend role checks as navigation/UX only. Clearly document that backend authorization must be implemented before production; never claim that hiding a button protects an endpoint. Avoid putting long-lived credentials in local storage unless there is a deliberate, documented security decision; the API does not currently issue HttpOnly cookies.
> - Keep requests and UI capability-aware. Use only routes that exist in the repository. Do not invent API paths and silently assume they work. Where a required workflow is blocked, keep the UI usable with a clear disabled/unavailable explanation and record the exact endpoint/data contract needed in a frontend integration-gap document.
> - Add scripts for development, production build, and lint/typecheck as appropriate; document install/run instructions, API URL configuration, demo-mode behavior, and known backend blockers.
>
> ## Live route contract
>
> Integrate these existing endpoints exactly:
>
> - Auth: `POST /api/v1/auth/signup` (`email`, `password`, optional `role`); `POST /api/v1/auth/login` (`email`, `password`); `POST /api/v1/auth/refresh` (`refresh_token`); `GET /api/v1/auth/me` (bearer token).
> - Merchants: `POST /api/v1/merchant/create`; `GET /api/v1/merchant/`; `GET /api/v1/merchant/{id}`; `PUT /api/v1/merchant/{id}`; `DELETE /api/v1/merchant/{id}`. Merchant JSON uses `name`, `email`, `phone`, `merchant_type`, `onboarding_status`, `approval_status`; types are `RESTAURANT`, `GROCERY`, `PHARMACY`, `PET_STORE`, `FLOWER_SHOP`.
> - Customers: `POST /api/v1/customer/create`; `GET /api/v1/customer/`; `GET /api/v1/customer/{id}`; `PUT /api/v1/customer/{id}`; `DELETE /api/v1/customer/{id}`. Customer JSON uses `name`, `email`, `phone`, `address`.
> - Catalog: `POST /api/v1/catalog/categories`; `POST /api/v1/catalog/items`; `GET /api/v1/catalog/items?q={query}&limit={limit}`; `GET /api/v1/catalog/items/{id}`; `GET /api/v1/catalog/merchants/{merchant_id}/items`; `PUT /api/v1/catalog/items/{id}`. Item JSON fields are `merchant_id`, `category_id`, `name`, `description`, `item_type`, optional `is_vegetarian`, `base_price`, `currency`, `is_available`, `inventory_tracked`, `substitution_policy`.
> - Orders: `POST /api/v1/order/create` (`merchant_id`, `customer_id`, `total_price`); `GET /api/v1/order/`; `GET /api/v1/order/{id}`; `PUT /api/v1/order/{id}/status` (`status`); `DELETE /api/v1/order/{id}`. Valid status strings are `CREATED`, `PAYMENT_PENDING`, `CONFIRMED`, `PREPARING`, `READY_FOR_PICKUP`, `PICKED_UP`, `OUT_FOR_DELIVERY`, `DELIVERED`, `CANCELLED`, `REFUNDED`.
> - Delivery: `POST /api/v1/delivery/partners/{id}/online`; `POST /api/v1/delivery/partners/{id}/offline`; `POST /api/v1/delivery/locations` (`partner_id`, `latitude`, `longitude`, optional `recorded_at`); `POST /api/v1/delivery/assignments` (`order_id`, `partner_id`). The UI has no supported way to discover/create partners.
> - Payments: `POST /api/v1/payments/intents` (`order_id`, positive `amount`, optional `provider`, `method`, `currency`, `idempotency_key`). Defaults are `RAZORPAY`, `UPI`, `INR`. This creates an intent only; there is no payment processing UI contract, status query, refund, or settlement API.
> - Health: `GET /healthz`, `GET /readyz`.
>
> Existing documentation and payload examples are in `docs/api.md`, `docs/architecture.md`, `docs/deployment.md`, and `postman/SwiftKart API.postman_collection.json`. If the repository has evolved, verify the current handlers and route registration before relying on this list.
>
> ## Portal requirements
>
> ### Shared shell and access
>
> - Create a consistent SwiftKart identity and clear distinction among Superadmin, Merchant, and Customer experiences. Show current workspace/account and provide an explicit way to switch only when the account is genuinely authorized to do so.
> - Make desktop information-dense but readable; provide a responsive mobile layout with navigation that remains usable without horizontal overflow.
> - Include search where data volume warrants it, filters, sorting, pagination/client-side paging only when the full server result is actually loaded, and clear record IDs/statuses for operational lookup.
> - All destructive or consequential actions need confirmation, clear success/error feedback, and protection against duplicate submissions.
> - Forms need labels, required-field validation, accessible keyboard behavior, focus management, and clear inline errors. Tables and controls need semantic markup and accessible names.
>
> ### Superadmin
>
> - Dashboard: show operational counts only when derivable from successfully loaded complete datasets. Label their source and avoid fabricated revenue, growth, conversion, uptime history, customer satisfaction, or other analytics not returned by the API. Display API/database readiness from health checks separately from business metrics.
> - Merchant directory: searchable/sortable list, merchant-type and approval/onboarding filters, create form, detail view, edit flow, delete confirmation. Include all five supported merchant types and status fields. Make clear that the backend has no approval workflow beyond updating the merchant record.
> - Merchant detail workspace: profile summary; catalog items by merchant; related orders only if safely available in the current environment; customers and other relationships only if the API returns them. Tabs for branches, documents, hours, inventory, analytics, and settlements may be shown as unavailable integration modules but must not fake persisted data.
> - Customer directory/detail: use customer CRUD for administrative operations with clear privacy-conscious presentation. Never expose password/token information. Avoid editing arbitrary auth roles because there is no safe user-admin API.
> - Orders: list/detail and status update using the exact status values. Since there is no order-items/history endpoint, show only fields returned; do not fabricate product lines or status timeline. Confirm delete and distinguish cancellation/refund statuses from actual payment refund processing.
> - Delivery operations: allow assignment creation only when the operator has a known valid partner ID; support online/offline/location writes only when a partner ID is explicitly supplied. Do not imply that partner discovery, map tracking, or assignment history is available.
> - Payments: explain that the live API can create payment intents but cannot list/reconcile/refund them. Never show invented payment status or settlements.
>
> ### Merchant workspace
>
> - Merchant home with current merchant profile, a small actionable summary based only on available records, and shortcuts to orders/catalog.
> - Profile editor for the exposed merchant fields and supported merchant type. Use an explicitly configured merchant ID for local integration/demo only; there is no secure merchant membership lookup yet.
> - Catalog manager: load items with `GET /catalog/merchants/{merchant_id}/items`; create categories and items; edit items; search catalog; edit availability and supported item metadata. Because there is no category-list endpoint, do not assume created categories can be reloaded unless the API response/state provides them. Do not show unsupported variant/modifier/add-on editing as live.
> - Orders: the backend currently returns all orders and offers no merchant filter. Do not fetch global orders into a production merchant session and call client-side filtering secure. Use an explicitly labeled local/demo fixture if the correct tenant-scoped endpoint is unavailable; list the required `GET /merchant/{id}/orders` or equivalent API contract in the gap document.
> - Provide unavailable-but-designed areas for branches/hours/documents, stock/inventory, delivery coordination, reports/analytics, and settlements. State the precise missing read/write capability rather than presenting nonfunctional controls as complete.
>
> ### Customer storefront and account
>
> - Design a genuinely usable customer-first shopping flow: service location/context, merchant/category discovery, search, product listing/detail, quantity selection, cart review, checkout, payment initiation, order confirmation/status, profile, addresses, and order history.
> - Current API supports global item search, item detail, merchant item list, customer CRUD, minimal order create/read/status update, and payment-intent create. It does not support merchant discovery, category listing, cart persistence, line-item checkout, addresses, scoped self-profile, customer order history, or safe customer order status updates. Do not send unsupported fields and imply they were saved.
> - A local in-browser cart can be usable, but explain/demo-label that it is not persisted server-side. Before creating an order, require a real customer ID and merchant ID; the existing customer CRUD is not linked to the logged-in auth user. Do not create a pretend relationship.
> - Do not call global customer or order listing endpoints from the customer portal to obtain private account data. Do not expose the global order status update endpoint to customers.
> - If demo mode is used, visually identify it, isolate it from live API data, and prevent demo values from being confused with actual accounts/orders/payments. For real-mode unsupported flows, present a clear integration-needed state and keep the rest of the app functional.
>
> ## Design direction
>
> - Build an intentional, polished commerce operations product, not a generic landing page or a set of decorative cards. The first screen after sign-in should be the appropriate usable workspace.
> - Use SwiftKart's existing visual cues as a starting point: forest green, bright citrus/lime, warm near-white, and a restrained coral accent; strong readable contrast; Manrope and DM Mono are already used by the API status page. Define reusable design tokens and status colors with accessible contrast.
> - Use quiet, dense admin layouts with clear tables, compact metrics, restrained borders, useful empty states, and consistent status badges. Keep customer shopping more visual and product-forward while remaining fast and legible.
> - Use icons for common actions, tooltips for unfamiliar icon-only controls, responsive behavior, and reduced-motion support. Avoid oversized marketing heroes, excessive nested cards, fake charts, placeholder lorem ipsum, and interactions that look functional but do nothing.
>
> ## Delivery and verification
>
> - Implement the app, not just a plan. Keep changes focused on the frontend and its documentation unless an existing backend change is strictly necessary; report backend work that must happen before secure production use.
> - Verify the frontend can be installed and started, run the production build plus available lint/type checks, and exercise the key flows against the local API when it is available. If the API/database is unavailable, report that limitation and still run offline build/type checks.
> - Check at least desktop and narrow mobile layouts, including sign-in, admin merchant list/detail, merchant catalog, customer storefront/cart, and empty/error states. Fix console errors, broken routes, overflow, and failed network handling.
> - Finish with a concise summary of implemented portals, exact run commands, tests/build checks performed, API endpoints connected, demo behavior, and the remaining backend/API authorization gaps. Do not describe unsupported capabilities as complete.
>
> ## Production backend work to call out
>
> Before production launch, recommend backend work for: server-enforced roles including a provisioned superadmin; merchant/customer ownership and tenant scoping; protected routes and permission checks; auth-user-to-customer/merchant links; scoped order/customer/merchant endpoints; merchant discovery and category reads; full checkout with order items/address/fees/idempotency; address and cart APIs; safe customer order history/status semantics; category/item lifecycle endpoints; merchant branches/hours/documents; inventory/variants/modifiers; delivery partner provisioning/listing/assignment reads/tracking; payment status/refunds/settlements; notifications/realtime contracts; pagination/filtering; and audit logging for administrative changes.

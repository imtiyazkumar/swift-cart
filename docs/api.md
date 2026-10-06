# Yember Core API

Base path: `/api/v1`

## Health & Status

- `GET /` (HTML platform status overview page)
- `GET /healthz`
- `GET /readyz`

## Multi-Application API Endpoints

### 1. Auth & Session (`/api/v1/auth`)
- `POST /signup` — Register user with role (`CUSTOMER`, `RESTAURANT_OWNER`, `DELIVERY_PARTNER`, `ADMIN`)
- `POST /login` — Authenticate and receive JWT access token & refresh token
- `POST /refresh` — Refresh access token
- `GET /me` — Get current authenticated user profile

### 2. Customer App (`/api/v1/customer`)
- `GET /restaurants?city=srinagar&search=ahdoos` — List active restaurants with categorized menus
- `GET /restaurants/:id` — Get restaurant detail & full menu
- `POST /coupons/validate` — Validate coupon code (e.g. `VALLEY20`) and calculate discount
- `GET /favorites` & `POST /favorites/toggle` — Manage saved favorite restaurants
- `GET /orders/:id/track` — Active order tracking & rider live GPS location
- `POST /reviews` — Submit order rating (1-5) and review comment

### 3. Rider App (`/api/v1/rider`)
- `GET /profile` — Get rider profile and vehicle details
- `PATCH /status` — Toggle status (`ONLINE`, `OFFLINE`, `BUSY`)
- `POST /location` — Update live GPS location (`latitude`, `longitude`)
- `GET /deliveries/available` — Fetch available delivery jobs
- `POST /deliveries/:id/accept` — Accept delivery job assignment
- `PATCH /deliveries/:id/status` — Update order delivery status (`PICKED_UP`, `OUT_FOR_DELIVERY`, `DELIVERED`)
- `GET /earnings` — Fetch total deliveries, daily earnings, and tips

### 4. Restaurant App (`/api/v1/restaurant`)
- `GET /profile` — Get restaurant profile and branch status
- `PATCH /status` — Toggle store open/closed status
- `GET /menu/categories` & `POST /menu/categories` — Menu category management
- `POST /menu/items` & `PATCH /menu/items/:id/availability` — Menu item CRUD and in-stock toggle
- `GET /orders` — Live store order stream/dashboard
- `PATCH /orders/:id/status` — Update order status (`CONFIRMED`, `PREPARING`, `READY_FOR_PICKUP`)
- `GET /analytics/daily` — Daily order count & revenue summary

### 5. Admin Web App (`/api/v1/admin`)
- `GET /dashboard/stats` — Platform summary metrics (total users, total orders, total revenue)
- `GET /users` & `PATCH /users/:id/role` — User listing & role assignment
- `GET /merchants/pending` & `PATCH /merchants/:id/approval` — Merchant onboarding approval
- `GET /riders/pending` & `PATCH /riders/:id/approval` — Rider onboarding approval
- `GET /coupons`, `POST /coupons`, `DELETE /coupons/:id` — Coupon management

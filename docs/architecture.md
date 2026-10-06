# Yember Core Architecture

Yember Core is a modular monolith designed to serve as the single, shared backend platform for the entire Yember ecosystem:
1. **Customer App**
2. **Rider App**
3. **Restaurant App**
4. **Administration Web App**

## Core Domain Modules

- `auth`: Unified multi-role authentication, JWT token issuance, refresh tokens, and Role-Based Access Control (RBAC).
- `customer`: Customer profile, saved addresses, favorite restaurants, coupon verification, active order tracking, and reviews.
- `rider`: Delivery partner profile, vehicle registration, online/offline status toggle, live GPS location updates, delivery job queue, and earnings logs.
- `restaurant`: Store management, branch operating status, menu category & item catalog, store order dashboard, and daily sales metrics.
- `admin`: Platform-wide analytics overview, user role management, merchant and rider onboarding approvals, and coupon management.
- `catalog`: Shared menu categories, items, variants, modifiers, addons, and search.
- `order`: Cart management, order creation, order lifecycle state machine, and status history.
- `delivery`: Delivery partner assignment, vehicle management, and live locations.
- `payments`: Payment intent abstraction and transaction processing.
- `notifications`: Notification logging, push token registry, and event subscribers.
- `realtime`: In-process channel hub for WebSockets and live location broadcasting.

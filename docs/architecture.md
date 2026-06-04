# SwiftKart Architecture

SwiftKart starts as a modular monolith with explicit domain boundaries. Each module owns handlers, services, repositories, DTO/model types, and database tables. That keeps local development simple while preserving a clear path to extract modules into services later.

## Domain Shape

The platform uses `Merchant -> Catalog -> Item`, not `Restaurant -> Menu -> Food`.

Supported merchant types are:

- `RESTAURANT`
- `GROCERY`
- `PHARMACY`
- `PET_STORE`
- `FLOWER_SHOP`

Food-specific properties, such as vegetarian flags, live on generic catalog items as optional attributes. Inventory-related fields are also optional so grocery, dark-store, and pharmacy flows can plug in later without replacing the order model.

## Modules

- `auth`: JWT access tokens, refresh-token rotation primitives, roles, sessions, OTP placeholders.
- `customer`: customer profile and address-ready shape.
- `merchant`: merchant onboarding foundation with branches, documents, hours, and approval-ready schema.
- `catalog`: generic categories, items, variants, modifiers, addons, pricing, and availability.
- `order`: carts, checkout-ready tables, status lifecycle, order items, and status history.
- `delivery`: partner availability, location updates, and assignment records.
- `payments`: payment-intent abstraction with provider/idempotency fields for Razorpay-style callbacks.
- `notifications`: notification logs, push-token storage, and event-driven send hooks.
- `realtime`: in-process channel hub ready to sit behind WebSockets and a future pub/sub backend.
- `events`: in-process event bus that can be replaced by Kafka or NATS later.

For an end-to-end explanation of how the modules work together, see [project-flow.md](project-flow.md).

## Extraction Strategy

The service and repository interfaces are the extraction boundary. When a module grows, move its package, tables, and event contracts behind a network API while keeping the caller-facing interface stable.

## Observability

The API uses structured Zap logging, Echo request IDs, health/readiness endpoints, secure headers, CORS, graceful shutdown, and in-process rate limiting. Prometheus/tracing can be added through middleware without changing domain code.

## Concurrency

Fanout, notification delivery, and matching workflows should use bounded queues and context-aware goroutines. The in-process event bus is synchronous today to keep rollback behavior predictable; asynchronous dispatch can be introduced behind the same `events.Bus` interface.

# Yember document architecture

The active API is cmd/api and internal/platform. It uses only the approved 38-table schema in migrations/document. internal/contractschema validates exact table names, columns, types, identity keys and numeric foreign keys. The extracted source definitions remain in schema-contract.json.

The 36 documented tables cover users and authentication, saved addresses, stations, restaurants and verification/banking, global menu categories and item variants, carts/orders/history, riders and delivery assignments/locations, payment intents/captures/webhooks/refunds, registration fees, earnings/payouts/allocations, cash collections, financial transactions, reconciliation and system activity. Coupons and immutable coupon usages are the only user-approved additions.

Each entity has a numeric internal identity and a unique public UUID. API callers use UUIDs; database joins and financial references use bigint. Order prices are stored in the documented numeric(12,2) core columns. Financial records use integer paise and INR. created_at and updated_at replace the documents' create_at and update_at spelling.

Restaurant owners manage their own menus and applications. Categories are global and maintained by active Yember administrators. Customers own their addresses, carts and orders. Riders access assignments belonging to their own account. The server verifies roles, ownership, availability, service coverage, variants and prices.

Menu and variant changes retain referenced historical versions; delivery-address edits create new address entities. Placed line quantities and prices, order history, financial postings, activity history and coupon usage cannot be overwritten through editing.

The API no longer mounts legacy domain handlers or workers that depend on extra tables. Historical SQL and older regression/conversion sources are retained separately from the active document implementation. They are not the application schema and must not be used to add features back.

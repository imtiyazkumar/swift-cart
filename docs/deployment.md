# Document-schema deployment

Deploy Core, administrator web and all three apps from the same checkout. The API refuses a database with extra application tables or incompatible document columns. Run go run ./cmd/migrate as a separate deployment step; startup never resets or recreates application tables.

Back up an existing installation before conversion. The empty-legacy replacement requires a verified pg_dump archive and refuses any existing operational record. For a populated legacy database, complete a verified lossless conversion and conservation checks first. Never apply legacy/ SQL to an active document schema.

Required private server configuration includes POSTGRES_DSN, JWT_SECRET, allowed origins and secure administrator cookies, Razorpay keys/webhook secret, bank encryption keys and the configured phone OTP sender. PUBLIC_API_BASE_URL is the public Core origin; checkout links also tolerate the older /api/v1 suffix.

DELIVERY_FEE_PAISE and RESTAURANT_COMMISSION_BPS are explicit monetary policy settings. Restaurant and rider registration are fixed at INR 200 and INR 150, non-refundable.

Verification files use DOCUMENT_LOCAL_DIR. The container creates /data/documents for the non-root application user, and Compose persists it in documents-data. Back up that volume together with PostgreSQL. Public menu images use scoped file references; verification files require the owner or an active administrator.

The local seed is development-only, preserves accounts and never fabricates financial history. Production must use the operator-only create_admin command with securely supplied ADMIN_EMAIL and ADMIN_PASSWORD.

Before operational use, verify gateway webhooks and refunds against provider test credentials, bank confirmation procedures, SMS delivery, production CORS/cookies/TLS and native device workflows. Local mocked tests do not move money or certify those external services.

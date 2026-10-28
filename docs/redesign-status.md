# Document alignment status

The local yember application database has been replaced with exactly 38 tables in document-based schemas: the 36 supplied tables plus the explicitly confirmed coupons and coupon_usages. Exact table/column/type, GENERATED ALWAYS bigint identity, unique UUID and bigint foreign-key checks pass. cmd/api now runs internal/platform; cmd/migrate installs and validates the document schema rather than the legacy bundle.

The pre-cutover local database contained no users, orders, ledger entries, payouts or other operational records. Its 67 configuration/migration rows were retained in system_activity_logs, and a complete verified custom-format backup is retained under .test-cache/backups/yember-document-cutover-20261028T055656Z. Other databases were not reset or converted.

Historical SQL now lives under migrations/legacy. It remains a conversion/regression source, never the normal install target. Nonempty migrations remain guarded: unsupported central payout and legacy-provider histories cannot be silently omitted or reset. Conversion tests preserve existing users, orders, receipt labels/prices, historical addresses, ledger values, payee balances, bank decryption context, refunds, webhook evidence and cash history.

The active backend covers authentication/rotation/OTP, administration, service coverage, applications/documents/banks, menus/variants, carts/coupons/checkout, order fulfillment and rider location, Razorpay checkout/capture/refunds, earnings, bank payout allocation/confirmation, cash handovers and exception records.

The administrator web uses full-page document workspaces and editors. Unsupported web feature pages were removed. Customer and partner apps were updated for the schema and out-of-scope active routes were removed. The default local API/web were started and smoke tested with all 38 administrator workspaces, all three app roles/profiles, cookie restoration and logout.

Validation includes backend PostgreSQL integration tests, exact schema tests, preservation/rollback tests, cash and provider workflows, administrator-only coupon lifecycle, web component/API tests and production build, and all mobile typechecks/transport tests/Expo lint.

Browser visual verification was completed in a separate Playwright profile after native Chrome app access timed out. See the web modernization notes for the 38-list and responsive coverage.

Limitations: Real Razorpay, real SMS/bank activity and Android/iOS device UI end-to-end flows have not been certified by the local tests. Nonempty legacy conversion must be completed and verified for each such installation before switching it.

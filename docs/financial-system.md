# Document financial system

The active financial tables are payment_intents, payments, payment_webhook_events, refunds, registration_fees, restaurant_earnings, delivery_agent_earnings, restaurant_payouts, delivery_agent_payouts, restaurant_payout_allocations, delivery_agent_payout_allocations, cash_collections, financial_transactions and reconciliation_runs. system_activity_logs records cross-role changes. There are no separate journal, ledger-entry, batch, custody, permission or provider-attempt tables in the application schema.

All financial values use integer paise and INR. Core order/menu numeric prices are converted at the boundary. Delivery charge and commission configuration are explicit; there is no hidden platform-fee column.

Online collection uses Razorpay Orders, server-side signature checks and authoritative payment fetches. Webhook IDs and body digests are retained in the inbox; replay cannot create another capture or posting. An uncertain provider response stays reserved and is not automatically retried as a new charge. A short-lived scoped checkout page lets all apps use Razorpay checkout without exposing the server secret.

Restaurant registration costs INR 200 and delivery registration costs INR 150. Both are non-refundable. Fees are separately assessed and paid; payment does not approve an applicant.

Cash is posted to CASH_IN_TRANSIT, never YEMBER_BANK. Delivery records preserve the actual receipt independently of settlement. A mismatch creates an exception and holds payout review. Gross handovers require the restaurant's exact acknowledgement. Eligible restaurant earnings are allocated separately from the gross cash, and excess cash is tracked as RESTAURANT_FEE_RECEIVABLE.

Earnings are recognized only for delivered, fully collected orders. Bank payout preparation reserves available earnings against a verified account. The application records a bank-confirmed result; it does not silently initiate a real bank transfer. A different administrator verifies the bank reference before PAID postings.

Refund requests are bound to an original online order payment, a stable request key and the remaining refundable amount. Only provider-confirmed refunds complete. Refund-related earning/payout allocation is separately held for review instead of silently rewriting settled liabilities. Registration refunds are rejected.

financial_transactions and activity history are append-only. Deferred balance checks require each financial event to balance. Coupon usage and order milestone history are also immutable. Bank numbers use contextual AES-GCM encryption; hashes, bank data, provider tokens and personal contact/location details are redacted from audit snapshots.

Real gateway, SMS and bank activity has not been exercised by local mocked tests. Configure and validate the corresponding services before operational use.

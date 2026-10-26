# Central finance: rules, activation and operations

## Confirmed launch decisions and pending approvals

The user confirmed India (Jammu & Kashmir), INR, Razorpay including UPI, cash on delivery, commission of 15% of restaurant listed item value, and fixed Rs 40 delivery pricing. Customer delivery charge and rider base each default to 4000 paise and remain separately configurable. Promotions are initially Yember-funded. TextBee is the selected phone SMS provider. There is no GST number currently: do not invent a GSTIN or a statutory tax rate. Entity identity, provider approval for marketplace collection/holding/payouts, tax obligations and chargeback recourse still require validation before live activation. A 15-day schedule is not a claim that it satisfies local settlement regulation.

Razorpay hosted Payment Links now collect customer payments. Set ONBOARDING_PAYMENT_PROVIDER=RAZORPAY for onboarding; the legacy Stripe adapter remains for compatibility. RazorpayX bank payouts are a separate integration requiring account approval, funding and IP allowlisting. Collection keys alone do not authorize third-party payouts. Never enable live money movement merely because API keys exist.

## Default rules

- Amounts are signed integer minor currency units; rates are basis points. Default restaurant commission is 1500 bps (15%), rounded half-up once on eligible item value at order placement. Existing snapshots never change.
- Item prices are the source of restaurant gross and commission basis, before platform-funded promotional discounts. Taxes and delivery charges are excluded from commission. Do not invent statutory tax rates. The absence of GST registration does not establish exemption.
- Capture creates an asset and an unearned order liability. Only paid AND delivered orders release that liability into restaurant/rider payables and platform revenue. COD requires central remittance confirmation; delivery alone is not proof Yember received cash.
- Central refund policy: cancelled/rejected paid orders create reviewable full-refund jobs. Pre-recognition refunds release unearned liabilities; post-recognition refunds debit platform refund expense and raise discrepancies. Reviewed recognized refunds are initially absorbed by Yember, without silently clawing back partner earnings. Partial refunds cannot exceed remaining captured funds. Unknown provider results are held for review. Chargebacks block affected settlements pending verified accounting; complete chargeback resolution remains a launch blocker. COD cash-return refunds require evidenced operations and are not sent to an online gateway.
- Settlement eligibility uses earned timestamps strictly before a UTC midnight cutoff. Default interval is 15 days anchored at 2026-01-01T00:00:00Z. Batch generation reserves journal entries, never pays them. Finance approval must be separate from batch preparation; bank verification and reconciliation require independent reviewers.
- Bank numbers are encrypted with AES-256-GCM and contextual authenticated data. APIs return last four digits only. No secret or complete bank number is returned to clients. Key IDs support staged rotation; preserve old keys while re-encrypting.
- Historic orders are not retrospectively assigned commissions or marked paid. Reconcile and import their opening balances through reviewed adjustment journals.

## Deployment gates

Apply migrations before enabling finance. Back up and restore-test PostgreSQL and private objects. Configure finance encryption keys independently from JWT keys and grant explicit finance permissions to selected administrators through the database/bootstrap process. Confirm provider sandbox flows, reconciliation, bank verification and legal/tax review. Keep money movement disabled until those external requirements are met.

Track essential remaining work in [launch readiness](launch-readiness.md); implementing finance does not certify cloud policies, app-store approvals, physical-device behavior or regulatory compliance.

## Environment and operations

All secret variables are backend-only. See `.env.example` and the Compose environment mapping. Apply migrations 035–053 with the existing migration command before starting the binary. Do not backfill paid states or commissions on historic orders: import reconciled opening balances through reviewed adjustment journals. After live movement, roll forward rather than dropping ledger tables.

- `ENVIRONMENT=production` enables secure-cookie, MFA, verified database TLS, bank key, OTP secret, scanner and messaging startup checks. `POSTGRES_DSN` must use `sslmode=verify-full`; configure the CA/hostname correctly. `TRUST_PROXY_CIDRS` contains only deployed reverse-proxy network ranges.
- `FINANCE_BANK_KEYS` is JSON mapping key IDs to base64 32-byte AES keys; `FINANCE_BANK_ACTIVE_KEY` selects the active ID. Keep old keys until ciphertext migration completes. Automated re-encryption is not implemented.
- `RAZORPAY_KEY_ID`, `RAZORPAY_KEY_SECRET`, `RAZORPAY_WEBHOOK_SECRET` configure collection. Subscribe `/api/v1/webhooks/razorpay` to payment-link paid, processed refunds and dispute events. `ONBOARDING_PAYMENT_PROVIDER=RAZORPAY` selects the India onboarding adapter. Use sandbox credentials first.
- `FINANCE_PAYOUT_PROVIDER=RAZORPAYX`, separate `RAZORPAYX_KEY_ID`/`RAZORPAYX_KEY_SECRET`, `RAZORPAYX_ACCOUNT_NUMBER` and `RAZORPAYX_WEBHOOK_SECRET` configure bank payouts. Configure `/api/v1/webhooks/razorpayx` and approved source IPs. Without RazorpayX, evidence-backed `BANK_RECONCILIATION` supports independent bank confirmation. Neither mode marks a generated batch paid.
- `ADMIN_TOTP_SECRETS` maps admin UUIDs to provisioned base32 secrets; `ADMIN_MFA_REQUIRED=true` is mandatory in production. Grant finance permissions through `go run ./cmd/finance_access --email <admin> --grant READ,PREPARE` and assign other operators only their needed CONFIGURE/APPROVE/PAYOUT/RECONCILE/ADJUST grants. Keep preparer, approver, initiator and reconciler independent as required by the API.
- `FINANCE_SCHEDULER_ENABLED=true` runs the API's settlement/refund worker. Configure `FINANCE_AUTOMATION_ADMIN_ID` with PREPARE, ADJUST and RECONCILE as needed. Database locks support multiple API instances; no separate cron is required. `FINANCE_LIVE_APPROVED=true` additionally gates live provider requests. Enable the finance policy only after provider/legal/tax approval.
- `OTP_HASH_SECRET` must be an independent random secret of at least 32 characters. `TEXTBEE_DELIVERY_ENABLED=true`, `TEXTBEE_API_KEY`, optional `TEXTBEE_DEVICE_ID` and `TEXTBEE_BASE_URL` configure phone messages. The device must remain powered and connected. API acceptance means QUEUED, not delivered. Monitor UNKNOWN/SENDING/FAILED/EXPIRED outbox records; do not blindly replay uncertain OTP sends. Validate India commercial messaging/SIM/DLT requirements with the provider. Resend email requires `EMAIL_DELIVERY_ENABLED=true`, `RESEND_API_KEY` and verified `EMAIL_FROM`.
- `DOCUMENT_CLAMAV_ADDRESS` specifies a private reachable clamd TCP endpoint. Production upload scanning fails closed. Configure private document storage, least-privilege IAM and object retention separately.
- `DELIVERY_PROOF_REQUIRED=true` enables proof in development; production always requires it. Completion codes are sent to the customer after pickup. COD completion records cash custody; remittance requires central evidence.

Monitor `/api/v1/admin/finance/health`, unresolved discrepancies, old payment attempts, provider reports and outbox backlog. Unknown payment-link creation can be recovered using the admin payment recovery endpoint with the independently verified provider link ID; it validates reference/amount/currency before attaching it. Onboarding attempts are listed under `/admin/finance/onboarding-payments` and use the same recovery API. Unknown payout responses preserve their original idempotency attempt. Confirmed reversals restore liabilities with new journals. Backlog counters do not configure your cloud alerts or on-call process.

Run sandbox drills for capture, partial/full refund, replayed callbacks, COD remittance, bank verification, independent batch approval, payout timeout/failure/success/reversal and reconciliation. Supply Expo project IDs/push credentials and build signed native clients; JavaScript checks do not prove device background-location behavior. Demonstrate database/private-object restore before release.

## Regulatory review references

CBIC Circular 167/23/2021 addresses GST liability for restaurant services supplied through e-commerce operators, including unregistered restaurants. Have an India tax professional determine Yember's obligations, registration, invoicing and reporting before launch; do not assume that no GST number means no liability. [Official circular](https://cbic-gst.gov.in/pdf/Circular-167-17-12-2021-GST.pdf).

Provider documentation: [Razorpay Payment Links](https://razorpay.com/docs/payments/payment-links/), [RazorpayX payout APIs](https://github.com/razorpay/markdown-docs/blob/master/x/payouts/api.md), [TextBee SMS](https://textbee.dev/docs/sending-sms/sending-sms).

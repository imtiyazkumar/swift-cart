# Yember: essentials before end-to-end testing and live launch

Updated 26 October 2026. Scope: `yember-core`, all three mobile applications in `yember-app`, and the admin `yember-web`. This replaces the old audit, implementation-plan and validation reports. Deployment/API guides remain operational references.

**Decision:** the code can proceed to controlled sandbox end-to-end testing once the environment prerequisites below are supplied. It is not approved for live customer payments or unrestricted launch. Software gaps and external approvals remain; passing local tests does not close them.

## 1. Current evidence

Rechecked during this cleanup:

| Check | Result | Limit |
|---|---|---|
| Go tests with `YEMBER_INTEGRATION=1` | Full suite passed, including isolated PostgreSQL migrations through 053 | Does not exercise an actual Razorpay or bank account |
| `go vet ./...` | Passed | Static analysis, not penetration testing |
| Admin web tests and production build | 18 tests passed; TypeScript/Vite build passed | Browser/device journey and accessibility review still required; dependency directive warnings remain |
| Mobile workspace TypeScript and lint | Passed; zero lint errors, seven warnings | No signed-device certification |
| Mobile session/storage tests | Eight passed | Provider/device integration still required |
| Production release configuration test | Passed | Validates URL/project-ID format, not account ownership or deployed connectivity |

Earlier in the same implementation session, all three apps exported Android, iOS and web bundles, the API built, and govulncheck reported zero reachable vulnerabilities with two additional non-reachable package/module findings. The last mobile dependency audit reported eight findings: five high and three moderate. Those audit/export/scan results are historical evidence, not new remote checks during cleanup; rerun them against the release lockfiles and artifact before live sign-off. Generated exports, scan files and logs have been removed intentionally.

Docker was unavailable in this environment, so the image, Compose topology and real ingress have not been validated. No production migration, live charge or live payout is established by this report.

Local startup repair: `yember_local` was subsequently found to be missing migrations 036–053. They were applied, and the running admin-session endpoint returned the expected 401 without a cookie instead of 503. The API now checks required schema before serving traffic and reports the migration command on failure. The new schema regression test and focused middleware/API checks passed. This local repair is not evidence that staging or production has been migrated.

## 2. Required before starting sandbox end-to-end testing

| ID | Essential prerequisite | Evidence required to start |
|---|---|---|
| E1 | One isolated staging environment for all repositories | A recorded commit from each repo; HTTPS API/admin URLs; separate PostgreSQL and private object storage; migrations through 053 applied; no production customer data or secrets |
| E2 | Working collection sandbox | Razorpay test credentials, signed webhook endpoint reachable from provider, UPI test path supported by the account, `ONBOARDING_PAYMENT_PROVIDER=RAZORPAY`, and COD enabled deliberately |
| E3 | Explicit payout test scope | Approved RazorpayX sandbox/source account and provider credentials if available; otherwise use a controlled stub to verify the adapter and mark real provider payout testing blocked. Bank-reconciliation mode tests do not certify RazorpayX |
| E4 | Transactional communications | Online registered TextBee Android device/API key and a consenting test Indian phone; verified Resend sender; actual EAS project IDs and APNs/FCM/Expo credentials |
| E5 | Native clients and identities | Install native development/internal-distribution builds for customer, restaurant and rider; create approved/covered restaurant, verified rider, customer, independent finance operators, and a second unrelated partner/customer for isolation tests |
| E6 | Secure server configuration | Bank-encryption key ring, independent OTP/JWT secrets, admin TOTP provisioning, explicit finance grants, trusted proxy CIDRs, deployed CORS and secure cookies; reachable ClamAV and private document storage |
| E7 | Deterministic operational fixtures | Active Jammu & Kashmir station coverage; restaurant hours; menu/pricing; valid delivery coordinates; verified test payout accounts; known commission and delivery defaults; eligible older earnings seeded only in the isolated test database |

Keep `FINANCE_LIVE_APPROVED=false` and use test provider keys. Enable only the staging finance policy and workers needed for the test. A production-like security rehearsal should also pass `ENVIRONMENT=production` validation with verified database TLS. See [finance configuration](financial-system.md), [deployment](deployment.md), and the mobile repository's `docs/production-release.md` for variable names and setup.

TextBee acceptance is only queued SMS; independently verify arrival on the phone, expiry, offline-device behavior and support recovery. Its documentation requires an online registered device and supports following the returned SMS batch reference. [TextBee sending documentation](https://textbee.dev/docs/sending-sms/sending-sms).

## 3. Minimum end-to-end acceptance matrix

Record order/payment/payout IDs, expected paise amounts, actual journal totals, device/build, and pass/fail for each case. Use masked evidence without passwords, bank numbers, OTPs or secrets.

| Area | Essential scenarios and pass condition |
|---|---|
| Customer identity | TextBee/email OTP arrival, expiry, failed attempts, secondary-contact verification, password recovery, refresh rotation, logout and a second account on the same device. Old sessions/data must not reappear |
| Partner onboarding | Restaurant/rider documents scanned and authorized; approval/coverage rules enforced; bank numbers masked; account replacement requires fresh verification; expired/uncertain Razorpay onboarding links recover without duplicate collection |
| Online order | Server-priced UPI/online checkout; user abandons/reopens payment; delayed/duplicate/invalid-signature callback; wrong amount/currency rejected; payment callback after cancellation. Client return alone never marks paid |
| COD order | Delivery code plus full cash confirmation; rider custody recorded; central remittance separately reconciled. Delivery alone must not create centrally collected cash or settle an unremitted amount |
| Pricing and earnings | Default 15% commission on listed item gross, platform-funded discount, separate Rs 40 customer delivery fee and rider base, rounding and commission change. Old snapshots stay unchanged; partner payables remain separate from platform revenue |
| Fulfillment | Accept/prepare/pickup/deliver; enforce closed hours and station coverage; rider foreground/background location, stale location, offline transitions, denied permissions and app termination. Verify reassignment/exception limitations before broader trials |
| Delivery proof | Correct/incorrect/expired codes, durable attempt count, customer-only resend cooldown/limit, other-user rejection, replay rejection and code exhaustion support handling |
| Refunds | Cancel/reject before delivery, partial/full refund, duplicate callback, unknown refund request outcome and refund after earnings/payout. No duplicate refund and no silent partner clawback; unresolved allocation remains visible |
| Settlement | Seed eligible entries and exercise before/at/after cutoff; duplicate/concurrent batch generation; inspect payees; independent approval; account replacement, disabled users and new discrepancies block unsafe payout initiation |
| Payouts | Timeout/UNKNOWN recovery retains key/body; failed payment remains owed; only confirmed failure permits new attempt; provider success posts exactly once; late callbacks and reversal restore correct liabilities. Demonstrate bank evidence for manual reconciliation |
| Access control | Customer/partner cannot read another account's records or documents. Test each finance grant, missing grants, invalid MFA, self-approval and self-reconciliation rejection |
| Notifications/support | App open/background/terminated push receipt and correct order deep link; logout device unregister; customer case and admin reply; email/push/SMS failure and backlog handling |
| Recovery | Kill/restart API around outbound provider requests and callbacks; retry webhooks; scanner/provider outage; replay work with multiple API instances. Totals and liabilities must reconcile without duplicate movement |
| Browser and native UX | Real HTTPS origin, payment-browser return/resume, refresh/logout across tabs, mobile reconnect, large text/screen reader and masked financial history; no client-side secrets or production debug inspector |

Do not wait 15 real days to test a batch: use isolated eligible fixtures and the configured cutoff. Do not rewrite production ledger history or mark test payouts successful without the corresponding test confirmation. Native bundle export is not a substitute for this matrix.

## 4. Essential software work before live

These gaps can be investigated during sandbox testing but block a complete launch sign-off.

| Priority | Gap confirmed in source | Required completion |
|---|---|---|
| P0 | Chargebacks are discrepancy flags; the Razorpay dispute path can create a global unresolved hold without complete allocation/resolution | Map provider dispute/payment/order, verify authoritative outcomes, post immutable bank-loss/recovery accounting, define who absorbs loss, and resolve holds safely. Test open/won/lost/replayed/late dispute cases |
| P0 | COD cash-return refunds do not have a complete evidenced financial workflow | Define refund recipient/method, refund approval and cash/bank evidence, prevent duplicate return, and reconcile rider custody/remittance/partner liabilities. A gateway refund endpoint does not cover returned physical cash |
| P0 | Failed/returned/undeliverable orders and exhausted delivery-code recovery lack a complete operational path | Define authorized transitions, customer/support/rider handling, cash custody and rider compensation; implement and test the corresponding financial consequences |
| P0 | Privacy export/deletion is only a database request-table foundation, not a complete lifecycle | Implement authenticated request/review/fulfillment, consent and retention rules, deletion/anonymization across personal data/documents/location, preserve legally required financial history, and publish accurate privacy/grievance policies |
| P0 | Eight mobile dependency findings remain in the last audit | Refresh the audit, determine shipped/runtime versus build-tool exposure, apply compatible fixes, and obtain documented security acceptance for any residual advisory with an enforceable mitigation. Do not force incompatible Expo/ESM upgrades to obtain a cosmetic zero |
| P1 | Onboarding provider fees and accounting exports are not fully reconciled with bank/provider reports | Include onboarding collection fees/charges, explain cash versus gateway clearing/settlement timing, reconcile captured/refunded/settled amounts and exported journal totals; sign off the chart of accounts with accounting |
| P1 | Messaging/realtime and browser concurrency recovery are not yet proven operationally | Add delivery-status/backlog visibility and recovery for stuck/unknown TextBee sends; verify cross-tab rotating sessions; establish cross-instance realtime delivery or an explicit tested polling/push fallback |

The central refund launch assumption is that reviewed recognized refunds are absorbed by Yember; partner earnings are not automatically clawed back. Confirm partner agreements, promotion funding and tax handling before approving that policy for live use. Default settlement is 15 days on UTC midnight cutoffs, not the 1st/15th of every calendar month. Confirm this schedule with finance/provider/legal owners.

## 5. Essential external and deployment gates before live

| Gate | Required evidence | Owner |
|---|---|---|
| Entity, bank and payment model | Confirm legal entity, KYC, business bank account and written provider acceptance of the actual marketplace collection/holding/settlement arrangement | Business/legal + provider |
| Tax and invoices | Determine India/J&K GST/e-commerce operator and other applicable tax obligations; provide verified invoice identity, registration/treatment, calculations, reporting and retention. Zero configured tax is not tax approval | India tax professional + finance |
| Payout activation | Separately approved RazorpayX product/source account, funding, allowlisted egress IPs, production keys/webhooks and a capped independently reconciled payout drill | Provider + finance + operations |
| SMS operating approval | Confirm commercial SMS/SIM/DLT applicability with the provider/operator; demonstrate TextBee device uptime, delivery monitoring, rate limits and a fallback/support process | Business + messaging operations |
| Hosting security | Build/scan actual image, private database/networking, verified TLS, ingress CSP/cookies/CORS/proxy limits, restricted runtime DB/IAM roles, private objects and scanner fail-closed drill | Infrastructure/security |
| Secrets and access | Secret manager, reviewed admin/finance grants and separation of duties, MFA recovery, bank-key backup/rotation plan, no live secrets in clients/Git/logs | Security + finance |
| Restore and deployment | Successful PostgreSQL/PITR and private-object restore; measured recovery targets; migration/upgrade rehearsal from realistic prior schema; known compatible rollback/roll-forward plan | Infrastructure + backend |
| Monitoring and reconciliation | Actionable alerts for webhook failures, UNKNOWN payouts/refunds, SMS/email/push backlog, stale rider location, stuck orders and DB health; daily provider/bank/ledger reconciliation and named on-call | Operations + finance |
| Signed apps and stores | Actual EAS project ownership/signing, production API/project settings, Android/iOS device matrix, background-location and push declarations, privacy/store approvals | Mobile + business |
| Release control | Fixed commits/image digests, regression/security acceptance, staging sign-off, capped pilot and accountable go/no-go owner | Release owner |

Razorpay's payout API documentation separately requires contact/fund-account setup, IP allowlisting and payout idempotency; collection integration alone does not establish payout readiness. [Official payout documentation](https://razorpay.com/docs/api/x/payouts/).

The user has no GST number currently. That is not evidence of exemption: CBIC Circular 167/23/2021 specifically addresses e-commerce operator liability for restaurant services, including unregistered restaurants. Obtain advice on the current rules and Yember's exact supplies before activating tax/invoicing behavior. [Official CBIC circular](https://cbic-gst.gov.in/pdf/Circular-167-17-12-2021-GST.pdf).

## 6. Go/no-go checklist

- Start sandbox E2E when E1–E7 have owners, working credentials/clients and isolated fixtures. Mark unavailable provider/device cases blocked, not passed.
- Complete the acceptance matrix; resolve critical/high defects; reproduce financial totals independently from provider and bank evidence.
- Close or formally accept the section 4 software gaps with a concrete tested mitigation; complete every applicable section 5 gate.
- Run a capped, monitored pilot with support and reconciliation coverage before wider release. A report, successful local tests or adding live keys alone cannot authorize launch.

Historical reports and generated local files were removed. Runtime/API/deployment guides and test source were retained. Git updates for this handoff are local commits only; pushing or deployment is outside the requested action.

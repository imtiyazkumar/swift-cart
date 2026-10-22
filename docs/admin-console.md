# Admin console API

`yember-web` is the independent React Router and TanStack Query application for
this API. Apply all tracked migrations, including `032_partner_workspaces.sql`,
before starting the updated Core binary. Existing databases must follow the
migration/baseline procedure in [platform setup](platform-setup.md).

## Browser sessions

The `/api/v1/auth/admin` endpoints inherit the authentication rate limiter.
They require `X-Yember-Admin: 1`; a supplied Origin must exactly match an entry in
`CORS_ALLOWED_ORIGINS`.

- `POST /login` accepts email/password, verifies an active ADMIN identity, stores
  a hashed refresh session and returns a short-lived access token and safe user
  profile. The refresh credential is only returned in an HttpOnly cookie.
- `POST /session` consumes and rotates a valid refresh session. Replayed refresh
  credentials fail. Current role/account eligibility is checked again.
- `POST /logout` revokes the cookie's refresh credential and clears the cookie.

The cookie is SameSite=Strict, scoped to `/api/v1/auth/admin`, and lasts seven days.
`ADMIN_COOKIE_SECURE` defaults true. Set false only for local HTTP development.
Protected admin APIs continue to require bearer authentication and current ADMIN
authorization. CORS permits credentials and the admin header for explicitly
configured origins.

## Resources

All routes below are under `/api/v1/admin` and require ADMIN authorization.

- `GET /console/overview` returns pending applications, customer/online partner
  counts, unassigned orders, failed emails and seven UTC calendar days of volume.
- `GET /console/integrations` returns configuration-presence flags only, never
  provider secrets. These flags do not prove a provider works.
- `GET /manage/:resource?q=&status=&role=&page=1&limit=25` returns
  `{items,total,page,limit}`. Limit is at most 100. Parameters are bound SQL values;
  resources and selected fields come from a fixed whitelist.
- `GET /manage/:resource/:id` returns `{record,...related}`. Related order/delivery
  history is limited to the latest 50 records. Records exclude passwords, token
  hashes and email bodies.

Resources: `customers`, `users`, `restaurants`, `partners`, `orders`, `payments`,
`reviews`, `coupons`, `audit`, `emails`, `support`. IDs are validated as UUIDs. Payment
projections include order payments and onboarding payments.

## Changes

Management changes validate a reason, execute with a fresh active ADMIN check,
and commit the audit record in the same transaction.

| Method and path                           | Body / behavior                                                                                               |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| `PATCH /manage/:resource/:id/profile`     | `name`, `phone`, `reason`; restaurant `description`, `address`; partner `vehicle_type`, `registration_number` |
| `POST /manage/:resource/:id/status`       | For customers/users: `disabled`, `reason`; cannot disable self or the last administrator                      |
| `POST /manage/:resource/:id/availability` | For restaurants/partners: `available`, `reason`; activation gates are checked                                 |
| `PUT /manage/restaurants/:id/items/:item` | `name`, `description`, `price` in minor units, `is_available`, `reason`; item must belong to restaurant       |
| `POST /manage/coupons`                    | Coupon terms plus `reason`                                                                                    |
| `PUT /manage/coupons/:id`                 | Coupon terms plus `reason`                                                                                    |
| `POST /manage/reviews/:id/operation`      | `enabled` means published; `reason`; preserves original review and moderation audit                           |
| `POST /manage/emails/:id/operation`       | `enabled: true`, `reason`; failed messages less than 24 hours old with retained body only                     |
| `POST /manage/support/:id/operation`      | `enabled: false` resolves; `true` reopens; requires an audit reason                                           |
| `POST /partner-support/:id/messages`      | `body`; writes an operator reply and marks the case in progress atomically with an audit event                |

Existing approval, suspension, user role, fee/dispatch settings and order
cancellation endpoints remain in use. Disabling an account revokes refresh
sessions, closes owned restaurants, sets delivery partners offline and expires
pending offers. Re-enabling an account does not automatically reopen operations.
Role changes and disable operations serialize protection of the last admin.

Restaurant profile editing targets its first undeleted branch, creating that
branch when missing; secondary addresses are retained. Vehicle editing targets
the first vehicle displayed in the profile. Menu creation and onboarding remain
in the existing partner workflows. The console does not delete financial or
historical records, issue refunds, fabricate settlements or bypass onboarding
payments. Paid cancellation requires an implemented verified refund workflow.

## Checks

`YEMBER_INTEGRATION=1 go test ./... -count=1` creates disposable test databases and
applies migrations without demo seeds. The admin suites check management queries,
RBAC, UUID validation, reason guards, activation gates, profile/menu/coupon edits,
branch isolation, review visibility, safe email retries, safe projections and
transactional audit records. The auth suite checks cookie flags, origin rejection,
rotation/replay protection and logout invalidation.

The new migration was validated in isolated databases; installation into an
existing application database is an explicit operator startup step.

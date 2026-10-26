# Local development and testing

The workspace has real `.env` and `.env.test` files in Core, admin web,
the mobile workspace, and each of its customer, restaurant and delivery apps.
Matching `.env.example` and `.env.test.example` templates are tracked. Client
files contain public API URLs only. Core owns database credentials, JWT secrets,
and the local seed password. Existing Core/customer env files were backed up as
ignored `.env.pre-local` files before the local profiles were installed. Private
Core profiles, runnable-app profiles and the mobile workspace's root `.env` are
ignored. The root mobile file is a public endpoint reference only.

## Profiles

| Profile     | Database            | Core API                | Admin web               |
| ----------- | ------------------- | ----------------------- | ----------------------- |
| Development | `yember_local`      | `http://127.0.0.1:8080` | `http://127.0.0.1:5173` |
| Test        | `yember_local_test` | `http://127.0.0.1:8084` | `http://127.0.0.1:5174` |

Both database profiles were initially seeded locally. Keep each profile migrated
to the current checkout before starting its API; the API does not apply migrations
automatically. From `yember-core`, run `go run ./cmd/migrate` using that profile's
environment, then `go run ./cmd/api`. The current schema is through migration 053
(plus the legacy auth migration). Missing schema now stops startup with the
migration command instead of causing authentication requests to return 503.
The old application database was not reset.
Each profile has a different generated JWT secret. Local HTTP uses
`ADMIN_COOKIE_SECURE=false`; live email is disabled and provider credentials are
empty. Cash-on-delivery checkout is enabled for local workflow testing.

From the workspace root, start these in separate terminals:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\yember-core\scripts\local.ps1 -Task api
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\yember-core\scripts\local.ps1 -Task web
```

The execution-policy override applies only to the child PowerShell process.
Nothing changes the machine's policy. If scripts are already allowed, invoke
`./yember-core/scripts/local.ps1` directly.

For the test profile, append `-Environment test` to both commands. Vite selects
its test mode explicitly and proxies to port 8084. Use the same hostname to open
the admin web app during a session so browser cookies stay consistent.

## Local accounts

| App / role                  | Email                    | Initial password   |
| --------------------------- | ------------------------ | ------------------ |
| Admin web / ADMIN           | `admin@yember.test`      | `YemberLocal!2026` |
| Customer / CUSTOMER         | `customer@yember.test`   | `YemberLocal!2026` |
| Restaurant / MERCHANT_OWNER | `restaurant@yember.test` | `YemberLocal!2026` |
| Delivery / DELIVERY_PARTNER | `delivery@yember.test`   | `YemberLocal!2026` |

Passwords are bcrypt hashes in PostgreSQL. The plaintext initial value is the
`LOCAL_SEED_PASSWORD` setting in each ignored Core profile. Change it before the
first seed if desired. Later seed runs preserve existing passwords, account
restrictions, profile edits and operational changes.

Fixtures include one approved local restaurant with two menu items, one verified
customer and address, one approved delivery agent with a vehicle, ten sample
orders, seven completed deliveries with earnings, one assigned order ready for
pickup, a review, a coupon, and an audit marker. Delivery availability starts
offline; turn it online in the delivery app or admin detail page.

The restaurant's paid onboarding flag is explicitly a local fixture. No provider
payment or settlement records are fabricated, and no real onboarding payment was
collected. Sample orders and earnings exist only in the dedicated local databases.

## Mobile apps

Start an Android emulator app, for example:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\yember-core\scripts\local.ps1 -Task customer
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\yember-core\scripts\local.ps1 -Task restaurant
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\yember-core\scripts\local.ps1 -Task delivery
```

The default public endpoint uses `10.0.2.2` to reach the computer from Android.
For browser testing, append `-ApiHost 127.0.0.1` and select Expo's web target.
For a physical phone, pass the computer's LAN IPv4 as `-ApiHost`; the phone must
reach the selected API port. Use `-Environment test` for port 8084.

The runner imports the selected app's env file into the child process rather than
using NODE_ENV to choose an API. This follows
[Expo environment configuration guidance](https://docs.expo.dev/guides/environment-variables/).
The mobile workspace root env files are references; each runnable app reads its own
file. The shared partner package has no separate runtime or private env file.

## Setup again and verify

```powershell
# Idempotent: create if missing, apply tracked migrations, insert missing local fixtures.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\yember-core\scripts\local.ps1 -Task setup
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\yember-core\scripts\local.ps1 -Task setup -Environment test

# Creates disposable yember_test_* databases; leaves persistent fixtures in place.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\yember-core\scripts\local.ps1 -Task test -Environment test

# Temporarily starts its own API, checks all seeded logins and admin data, then stops it.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\yember-core\scripts\local.ps1 -Task smoke -Environment test
```

Smoke checks require the selected port to be free and never stop an existing
server. Setup/seeding accepts only the dedicated loopback local databases and
development/test profiles. No fixture seed runs automatically at API startup.

On a new checkout, copy each relevant example file to its corresponding actual
env filename, configure local PostgreSQL credentials and generate distinct random
JWT secrets of at least 32 characters, then run setup. Node dependencies and Go
must already be installed as described in [platform setup](platform-setup.md).

The admin homepage is `/` after sign-in: live totals, fulfillment statuses,
seven UTC days of order volume, action center, quick actions, recent orders,
recorded delivered value, and audited admin activity. Status links open filtered
order pages. The dashboard polls every 30 seconds and supports manual refresh.

Restaurant and delivery now have protected Expo Router tabs and detail pages.
Restart their Expo servers after this upgrade. The admin support inbox connects
to the same partner case conversations. See [partner workspaces](partner-apps.md)
for tab features, research references, validation and remaining release work.

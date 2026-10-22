# Operational stations

Stations define active circular service areas using a latitude, longitude and radius
in metres (positive and at most 100,000). Core validates coordinates, rejects missing
or non-finite values, and includes the exact radius boundary. Coordinates at zero are
valid. Great-circle distance uses a spherical Earth radius of 6,371,008.8 metres.
Overlaps resolve by distance to the center, then UUID ascending for exact ties. The
same PostgreSQL resolver is used during registration, discovery, cart selection and
checkout. Customers have no permanent station assignment.

## Rollout

Apply tracked migration `033_operational_stations.sql` after migration 032 using the
normal migration runner and baseline procedure. No live database is migrated by
tests. No production station or arbitrary coverage is seeded by this migration.

Existing restaurants without a station are closed and delivery partners are set
offline. Their records, orders and accepted assignments remain intact. Add real
coverage under **Settings → Stations**, then have each partner request an initial
station assignment from their Account page. An administrator reviews the request.
An administrator can also request assignment using the profile's resource ID;
another administrator must approve that request. Existing addresses without
coordinates must be replaced with geolocated addresses before ordering. Historical
orders with no station are retained but do not receive new delivery assignments.

Deploy Core and the updated clients together: old clients omit the now-required
address, registration coordinates, or location metadata and fail closed. The local
development seed explicitly installs a 5 km demo station at 34,74; this fixture is
not production coverage. It gives seeded addresses and restaurants matching
coordinates and installs a fresh location only when explicitly seeded.

## APIs

All paths are under `/api/v1` and require an active authenticated identity.

| Endpoint | Behavior |
| --- | --- |
| `GET /stations` | Active stations for partner registration/transfer selection. |
| `GET /admin/stations` | All station configurations; active ADMIN required. |
| `POST /admin/stations` | Create coverage with `name`, `latitude`, `longitude`, `radius_m`, `active`, `reason`. |
| `PUT /admin/stations/:id` | Update the same fields, atomically audit and remove affected availability. |
| `POST /station-changes` | Owner or admin requests `kind` (`restaurant`/`delivery`), `resource_id`, target `station_id`, `reason`; restaurants also supply coordinates within that station. |
| `GET /station-changes` | Own requests, or all requests for an ADMIN. |
| `POST /admin/station-changes/:id/decision` | `approve` and `reason`; reviews a pending request once, revalidates target and ownership eligibility, and audits atomically. |
| `PUT /customer/cart/delivery-address` | Owned `address_id`; resolves coverage again and removes an incompatible active cart. Even an owned unserved address clears incompatible items before returning an unavailable error. |

Restaurant application submission requires numeric `latitude` and `longitude`.
Core ignores a client-selected restaurant station and assigns the nearest covering
station. Delivery applications require an active `station_id`. Email/account signup
alone creates no restaurant resource; coverage is checked when its application is
submitted. Restaurant coordinate edits preserve its existing station and fail if
outside that station. Address edits require coordinates, including through admin
profile editing. Review and opening recheck station status and coverage. Changing
stations requires an approved request, not a profile edit or GPS movement.

Customer discovery (`/customer/restaurants` and `/customer/restaurants/:id`)
requires the selected owned `address_id` query parameter. Adding cart items requires
`address_id` alongside `item_id` and `quantity`. Address creation requires numeric
coordinates. Core checks restaurant/station eligibility during discovery, adding
items, address selection and checkout; the order stores its station snapshot.
Successful idempotent checkout retries return their original order even if coverage
later changes. An uncovered address never creates a new order.

Station transfers require all active restaurant orders or the partner's accepted
delivery to finish first. Approval leaves the restaurant closed or partner offline;
normal payment, approval, vehicle and availability gates still apply before
reopening. Station edits close restaurants outside their assigned coverage, set
ineligible partners offline and expire pending offers. They never change accepted
assignments or their completion transitions. Coverage checks and configuration
edits use a transaction-scoped coverage lock; registration, orders, online state
and assignment creation also have database guards for alternative write paths.

## Delivery location policy

`POST /rider/location` requires `latitude`, `longitude`, `accuracy_m`, RFC3339
`observed_at`, `permission_granted: true`, and `mocked: false`. Core supplies the
receipt timestamp. A location must be at most two minutes old and no more than
30 seconds in the future, with accuracy between 0 and 100 metres. Core rejects
replayed fixes and jumps beyond 70 metres/second plus a 200 metre tolerance.
Availability requires the full accuracy circle to be within the assigned active
station (`center distance + accuracy <= radius`), so uncertain boundary fixes
cannot enable new work. Invalid reported fixes take the partner offline and expire
offers while retaining accepted deliveries.

The app obtains a fix before requesting ONLINE and refreshes foreground GPS while
online or fulfilling an accepted delivery. Stopping sharing requests OFFLINE.
Background/disconnected apps stop qualifying after the freshness window; the
existing five-second dispatch tick expires stale availability. Dispatch filtering,
offer acceptance and assignment insertion each independently check current station
and location eligibility. Going offline does not prevent delivery completion.

There is no device attestation or trusted hardware location provider in this
platform. Permission/mock flags, coordinates and capture time remain untrusted
device claims: range, freshness, accuracy, replay and movement checks reduce abuse
but cannot prove physical presence against a modified client. Do not describe these
checks as hardware-verified GPS or spoof-proof location.

## Validation

`YEMBER_INTEGRATION=1 go test ./... -count=1` applies migrations in disposable
databases. Station tests cover no coverage, inactive stations, exact boundaries,
overlap/tie resolution, invalid coordinates, restaurant registration and approval,
owned addresses, cart clearing, checkout/discovery isolation, location freshness,
permissions, mocked/poor fixes, replay, impossible movement, stale dispatch,
unauthorized/self/replayed transfers, active-delivery transfer rejection, and
completion of an accepted delivery outside the station. Run `go vet ./...`, web
build/tests, and mobile typecheck/lint/tests as well.

## Station relationships and local fixtures

Migration 034 gives delivery offers, assignments, earnings and support tickets explicit station foreign keys. Orders retain an immutable station snapshot. Delivery children inherit that snapshot server-side; transfers never rewrite historical records. Payments and reviews derive their station through their order; menu items and branches derive theirs through their restaurant. Customers remain address-based and coupons remain global.

Admin lists and record details show station names. Station links open that station's order history; the station filter also scopes restaurants, partners, payments, reviews and support.

After migrations, run `go run ./cmd/seed_stations` against the guarded local development database. It creates Srinagar, Srinagar North and Srinagar East coverage, preserves existing accounts and station edits, and repairs only documented demo entity relationships. Known demo orders propagate their restored station to delivery children. Other historical records remain unknown when their original station cannot be established. New databases can use `go run ./cmd/seed_local` for the complete demo dataset.

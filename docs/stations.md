# Stations and service coverage

stations stores latitude, longitude, radius_km and status. user_addresses stores the exact delivery coordinates. restaurants and riders reference their station by numeric foreign key; API fields contain UUIDs.

The server uses station_covers to calculate Haversine distance against an active, non-deleted station. Restaurant discovery requires a saved address owned by the customer and returns approved, open restaurants in a covering station. Cart mutation and checkout repeat the availability and coverage checks.

Owners select an active station when applying. Restaurant coordinate updates must remain inside that station. Station changes are administrator edits recorded with a reason; there is no station_changes table or extra transfer workflow.

Riders can go online only through their account workflow. Assignment selects approved, available, paid-registration riders in the restaurant's station, locks the rider and excludes active assignments. Orders ready for pickup trigger an eligible assignment when one is available; an administrator can explicitly assign an eligible rider through the order API.

Administrators create and edit stations using /records/stations/new and /records/stations/:id/edit. Distances are entered in kilometres. The development station at 34,74 with a 5 km radius is an explicit local fixture, not production coverage.

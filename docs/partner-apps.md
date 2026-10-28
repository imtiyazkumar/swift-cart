# Restaurant and delivery applications

The Expo monorepo contains apps/customer, apps/restaurant, apps/delivery and the shared packages/partner package. The API base URL comes from each app's EXPO_PUBLIC_API_URL. Android emulator profiles use 10.0.2.2 to reach the local API.

Restaurant onboarding creates the canonical restaurant, assesses the INR 200 non-refundable registration fee, and then permits document upload and bank submission. Rider onboarding creates riders, assesses the INR 150 non-refundable fee, and follows the same independent verification/payment process. Registration capture does not approve or activate either account.

Restaurant screens cover availability, orders, global-category selection, menu editing/photos, account details, verification documents, registration payments, earnings, bank details and gross cash acknowledgement. Rider screens cover availability and location, assigned deliveries, pickup/dropoff, exact cash receipts, earnings, bank details and gross handovers.

The customer screens use live serviceable restaurants, menus/variants, saved addresses, cart/checkout, coupons, order history and tracking. They do not invent ratings, delivery estimates, rewards or balances. Out-of-scope favourites, reviews, notification feeds, support routes, operating-hour tables and separate delivery confirmation codes have been removed from active routing.

Core stores all relationships numerically and the apps use UUID references. Menu prices and variant deltas are displayed in INR; financial transport values use paise. Cart writes fetch the server's current cart instead of assuming a response body or pricing locally.

Run npm run typecheck, npm test and npm run lint at the mobile workspace root. Device payment UI, real SMS delivery and Android/iOS native end-to-end flows require the corresponding configured services/devices; passing transport tests does not certify those flows.

The opt-in `TestRestaurantBrowser` integration harness runs the restaurant app's
Playwright suite against the real API in a disposable PostgreSQL database and
temporary upload directory. It starts an ephemeral API listener and the browser
suite's Expo server on port 8085. Set `YEMBER_INTEGRATION=1` and
`YEMBER_RESTAURANT_E2E=1`, then run
`go test ./internal/platform -run '^TestRestaurantBrowser$' -count=1 -timeout 15m -v`.
See [browser coverage and prerequisites](../../yember-app/docs/restaurant-testing.md).

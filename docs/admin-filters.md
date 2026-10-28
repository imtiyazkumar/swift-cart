# Admin list filters

`GET /api/v1/admin/contract` advertises a `filters` array for every resource.
Each entry supplies `field`, `label`, `kind` (`entity`, `choice`, `boolean`, or
`date`) and, for entity selectors, the target `resource`. The same definitions
form the backend SQL allowlist.

## Requests

Pass exact-match filters as `filter[field]=value`; combine any number with `q`,
`page`, and `limit`. Entity values are public UUIDs while the console displays
searchable names. Multiple filters use AND semantics. List items and totals use
the same predicates before pagination.

```text
GET /api/v1/admin/manage/menu-items?filter[restaurant_id]=<uuid>&filter[category_id]=<uuid>&filter[is_available]=true
GET /api/v1/admin/manage/orders?filter[station_id]=<uuid>&filter[delivery_agent_id]=<uuid>&filter[status]=DELIVERED
GET /api/v1/admin/manage/orders?filter[created_from]=2026-10-01&filter[created_to]=2026-10-28
```

Date boundaries include the entire selected calendar day in Asia/Kolkata (UTC+05:30).
Inverted dates, invalid UUIDs, invalid booleans, unknown fields and repeated filter
values return HTTP 400. Queries bind all values; relationship identifiers and join
paths come exclusively from the server-owned schema.

Choice values come from
`GET /api/v1/admin/manage/:resource/filter-options?field=status&q=...`.
The response is `{ "items": ["PENDING", "APPROVED"] }`, limited to 100 distinct
non-null values. This endpoint has the same administrator access checks as lists.
Entity selectors use paginated, searchable admin lists and cached record lookups.

## Console behavior

The console starts with only a filter icon beside search. Its add-filter menu
offers at most three useful entity/state choices plus one date range. Fields appear
only after being added, and can be removed individually. The API supports the
full allowlist below for saved URLs and other admin consumers.

Search, entity selectors, status/boolean choices and date ranges can be combined.
Changes reset pagination to page one. Paging and search preserve the filters.
Clear filters preserves the text search. URLs and browser history restore the
selections, including names for entities outside the first page of options.

## Coverage

All 38 lists have filters. Owning relationships supply additional context:
variants inherit item/category/restaurant/service area; order lines inherit order
and customer context; orders can filter by assigned delivery agent and menu
selection. Global categories can filter by restaurants using them.

| List | Available filters |
| --- | --- |
| cart items | Cart, Menu item, Menu variant, Customer / user, Restaurant, Service area, Category, Created from, Created through |
| carts | Customer / user, Restaurant, Status, Service area, Created from, Created through |
| cash collections | Order, Delivery agent, Restaurant, Restaurant payout, Status, Customer / user, Service area, Created from, Created through |
| coupon usages | Coupon, Customer / user, Order, Restaurant, Service area, Delivery agent, Created from, Created through |
| coupons | Discount type, Is active, Created from, Created through |
| delivery agent bank details | Delivery agent, Verification status, Customer / user, Service area, Created from, Created through |
| delivery agent earnings | Delivery agent, Order, Status, Customer / user, Restaurant, Service area, Created from, Created through |
| delivery agent payout allocations | Delivery agent payout, Delivery agent earning, Delivery agent, Order, Customer / user, Restaurant, Service area, Created from, Created through |
| delivery agent payouts | Delivery agent, Bank details, Method, Status, Customer / user, Service area, Created from, Created through |
| delivery assignments | Order, Delivery agent, Status, Customer / user, Restaurant, Service area, Created from, Created through |
| financial transactions | Order, Payment, Restaurant earning, Agent earning, Restaurant payout, Agent payout, Registration fee, Transaction type, Account code, Direction, Customer / user, Restaurant, Service area, Delivery agent, Created from, Created through |
| item variants | Menu item, Is available, Restaurant, Category, Service area, Created from, Created through |
| menu categories | Is active, Restaurant, Service area, Created from, Created through |
| menu items | Restaurant, Category, Is available, Service area, Created from, Created through |
| order items | Order, Menu item, Menu variant, Customer / user, Restaurant, Service area, Delivery agent, Category, Created from, Created through |
| order status history | Order, From status, To status, Changed by, Customer / user, Restaurant, Service area, Delivery agent, Created from, Created through |
| orders | Customer / user, Restaurant, Address, Status, Service area, Delivery agent, Menu item, Menu variant, Category, Created from, Created through |
| otp challenges | Customer / user, Purpose, Created from, Created through |
| payment intents | Order, Restaurant, Delivery agent, Purpose, Method, Stage, Status, Customer / user, Service area, Created from, Created through |
| payment webhook events | Provider, Event type, Signature valid, Processing status, Created from, Created through |
| payments | Payment intent, Order, Provider, Method, Status, Received by, Customer / user, Restaurant, Service area, Delivery agent, Created from, Created through |
| reconciliation runs | Source, Status, Created from, Created through |
| refresh tokens | Customer / user, Created from, Created through |
| refunds | Payment, Order, Status, Customer / user, Restaurant, Service area, Delivery agent, Created from, Created through |
| registration fees | Restaurant, Delivery agent, Payment intent, Status, Service area, Created from, Created through |
| restaurant bank accounts | Restaurant, Verification status, Service area, Created from, Created through |
| restaurant documents | Restaurant, Document type, Verification status, Verified by, Service area, Created from, Created through |
| restaurant earnings | Restaurant, Order, Status, Customer / user, Service area, Delivery agent, Created from, Created through |
| restaurant payout allocations | Restaurant payout, Restaurant earning, Restaurant, Order, Customer / user, Service area, Created from, Created through |
| restaurant payouts | Restaurant, Delivery agent, Order, Method, Status, Service area, Created from, Created through |
| restaurants | Owner, Service area, Status, Is open, Approved by, Created from, Created through |
| rider documents | Delivery agent, Document type, Verification status, Customer / user, Service area, Created from, Created through |
| rider locations | Delivery agent, Customer / user, Service area, Created from, Created through |
| riders | Customer / user, Service area, Vehicle type, Verification status, Availability status, Approved by, Created from, Created through |
| stations | Status, Created by, Created from, Created through |
| system activity logs | Actor type, Actor, Action, Entity type, Created from, Created through |
| user addresses | Customer / user, Is default, Created from, Created through |
| users | Role, Status, Created from, Created through |

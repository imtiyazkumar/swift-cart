# Administrator web workspace

The web app obtains its complete resource list from GET /admin/contract. Navigation contains exactly the documented domains and the two approved coupon resources. Support cases, reviews, email queues, MFA, modifiers, addons and generic merchant verticals are absent from the active web application.

Each editable entity uses a dedicated creation/edit URL under /records/:resource. Forms are full pages with related-record selection, typed fields, a change reason and a persistent save area. They do not use edit modals. Read-only workflow entities have searchable lists and detailed history views.

Administrator-only operations cover global categories, stations, users/roles, restaurant and rider reviews, menus and variants, verification documents and coupons. Coupon deletion retains coupon_usages; activation updates do not erase the code or its history.

Bank detail pages provide verification actions. Available earning pages prepare bank payouts. Bank payout detail pages require a second administrator, confirmed status, original bank reference and reason. Captured online payment pages request bounded, idempotent refunds; refund pages reconcile provider results. These are operational forms, not arbitrary financial table edits.

The last active administrator cannot be disabled or demoted. Financial postings, coupon usage, order milestones and activity logs are immutable. Sensitive hashes, encrypted bank data and raw provider payloads are not exposed by record projections.

Run npm run dev in yember-web after starting Core. The local default is http://127.0.0.1:5173. Seeded administrator credentials use admin@yember.test and LOCAL_SEED_PASSWORD from the private Core environment.

Every list supports schema-defined entity, state and date filters. See
[filter contract and per-list coverage](admin-filters.md).

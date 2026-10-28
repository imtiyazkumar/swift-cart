# Database groups

The 38 application tables live in PostgreSQL schemas matching the document sections. There are no application tables or compatibility views in public. Shared trigger/helper functions remain in public.

| Schema | Tables |
| --- | --- |
| users | users, otp_challenges, refresh_tokens, user_addresses |
| stations | stations |
| restaurants | restaurants, restaurant_documents, restaurant_bank_accounts, menu_categories, menu_items, item_variants |
| orders | carts, cart_items, orders, order_items, order_status_history |
| delivery | riders, rider_documents, delivery_agent_bank_details, delivery_assignments, rider_locations |
| payments | payment_intents, payments, payment_webhook_events, refunds |
| registration | registration_fees |
| earnings | restaurant_earnings, delivery_agent_earnings |
| payouts | restaurant_payouts, delivery_agent_payouts, restaurant_payout_allocations, delivery_agent_payout_allocations |
| cash | cash_collections |
| accounting | financial_transactions, reconciliation_runs |
| activity | system_activity_logs |
| promotions | coupons, coupon_usages |

Run `go run ./cmd/migrate` to install or upgrade. The migration validates the previous 38-table public layout before moving relations with ALTER TABLE SET SCHEMA in one transaction. Existing rows, UUIDs, identities, foreign keys, indexes and triggers are retained. Extra tables or schema drift stop the migration.

The database search_path includes all these schemas so API table names continue to resolve. Restart application connection pools after upgrading. Refresh the database browser tree to see the groups. External SQL that explicitly used public.users must use users.users instead, and similarly for the other tables. No API URL or client payload changes are required.

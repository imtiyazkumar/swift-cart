# Admin entity labels

Admin record lists and details include additive presentation metadata:

- `_display_name`: the record's readable name or reference.
- `_relations`: a map from relationship field to `{ id, label, resource }`.

For example, an item's `restaurant_id` remains its public UUID while
`_relations.restaurant_id` supplies the restaurant name and the `restaurants`
resource for navigation. Menu variant labels include the parent menu item.
Order relationships use the order number; delivery agents use their user name.
Admin list search also matches these labels.

This metadata is restricted to admin projections. Public and partner responses
retain their existing projection. No database migration is required, and write
requests continue to use UUIDs.

The web console uses labels in lists, detail cards, and relationship selectors.
When metadata is absent, it resolves a relationship through the existing cached
record query. Full record references remain available in the collapsed reference
section. Missing relationships have an explicit unavailable state.

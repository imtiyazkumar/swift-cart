# Workspace cleanup

The active API is `cmd/api` and `internal/platform`. Retired endpoint packages
that had no consumers in current commands or retained migration/import tests have
been removed. The unused duplicate `pkg/config` loader and in-memory event bus
were also removed. `go mod tidy` updates the dependency graph to match the source.
The Makefile tests all current packages instead of maintaining a stale package list.

The historical finance and authentication code is retained because conversion
checks still use it. The immutable `migrations/legacy` files and manifest checksums
are unchanged. The active schema now has four explicit, numbered stages; see
[migration order](../migrations/README.md). It defines each behavior once and can
refresh behavior on an existing installation without resetting business records.

The admin console removes obsolete selectors from retired screens and the old
detail layout wrapper. Current component styles and generated status/image/button
classes remain. The production CSS bundle is approximately 52 KB before gzip.

The mobile workspace removes the Expo tutorial route, unused starter tabs,
animated logo/theme helpers, template-only images, a duplicate stylesheet and the
starter reset script. Product routes and app identity assets are retained. The
workspace README now describes the current apps instead of obsolete recovery and
migration instructions.

Deletion candidates were checked through import/reference searches. Core's full
PostgreSQL integration suite also passed with those files excluded through a Go
build overlay before removal. Local rollback ZIPs are stored in the parent
workspace's `.cache` directory and are not committed to any repository.

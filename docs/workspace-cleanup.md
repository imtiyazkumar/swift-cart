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

## Final configuration and source audit

Historical authentication, finance and old-schema seed implementations now live
under `internal/legacy`. Their tests are preserved. The current seed package has
only current-schema fixtures and its target guard. Legacy email options are no
longer part of active API configuration. The unused in-memory limiter is removed;
the historical database limiter lives with legacy authentication.

Historical SQL byte protection now targets `migrations/legacy/*.sql` explicitly.
The active four-stage baseline uses LF text and a regression check prevents
superseded function definitions from being replayed in fresh installations.
Source architecture JSON is formatted as UTF-8; the 36-table source document and
the enriched 38-table runtime contract remain distinct, intentional artifacts.

Thirteen unused storage/provider/session keys were removed from both private
Core profiles. Active values and both runtime profiles were retained. Obsolete
mobile root env copies, pre-local backup envs and generated build artifacts were
removed after a private rollback backup. No credentials are added to Git.

The mobile root manifest owns development tooling only. Runtime dependencies are
declared in their consuming workspaces; missing secure-store/task-manager
declarations were added through Expo's compatible installer. Unused notification
plugins, public push-project configuration, template modules, duplicate dev
dependencies, dead UI components and 13 unreferenced style properties are removed.
Eleven unreferenced illustration assets were also removed, retaining configured
app icons. Partner overview readers share one typed query and the current availability/count
fields. The web drops 67 remaining legacy selectors and excludes browser test
artifacts from Docker contexts. Its resulting CSS is approximately 49 KB.

The dependency audit's compatible fix updates node-forge. Remaining Expo 57
transitive advisories involve decode-uri-component, deepmerge and Zod; npm
suggests breaking SDK changes for those. No forced SDK changes are applied.

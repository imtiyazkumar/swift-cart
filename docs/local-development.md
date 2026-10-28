# Local document-schema development

The development profile now uses the existing local yember database. It contains exactly the 38 approved tables in document-based schemas. Core's private .env owns database credentials, JWT/SMS/gateway/bank keys and LOCAL_SEED_PASSWORD. Expo and Vite files contain public API configuration only.

From yember-core:

```sh
go run ./cmd/migrate
go run ./cmd/seed_local --allow-local-yember
go run ./cmd/api
```

The local-yember seed flag is permitted only for the loopback development database. Existing users and passwords are preserved. Fixtures create four accounts, a station, an application for each partner type and a small catalog. Both registration fees remain DUE. No paid fee, payment, order or payout is fabricated. Administrator approval and actual payment configuration are still required for live ordering.

The four local emails are admin@yember.test, customer@yember.test, restaurant@yember.test and delivery@yember.test. Their initial password is LOCAL_SEED_PASSWORD from the private Core profile.

From yember-web, run npm run dev. Defaults are API http://127.0.0.1:8080 and administrator web http://127.0.0.1:5173. The web development server proxies /api to Core.

The integration test helper reads .env.test for PostgreSQL connection credentials and creates isolated disposable databases. Its historical-source databases deliberately exercise legacy conversion; they are not application targets. Existing nonempty databases are never reset by the strict installer.

Backend integration verification uses YEMBER_INTEGRATION=1 with go test ./... -count=1. The web uses npm test and npm run build. The mobile workspace uses npm run typecheck, npm test and npm run lint.

On Windows, use npm.cmd if PowerShell blocks npm.ps1. A writable workspace GOCACHE may be needed when the default Go cache is restricted.

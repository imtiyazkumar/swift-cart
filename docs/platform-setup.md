# Platform setup

1. Configure the private Core environment with a database, JWT secret and allowed administrator/app origins.
2. Run go run ./cmd/create_db if the selected database does not exist.
3. Run go run ./cmd/migrate. A fresh installation contains only the 38 approved tables.
4. Create an administrator with go run ./cmd/create_admin and securely provided ADMIN_EMAIL/ADMIN_PASSWORD. Existing accounts are never promoted or overwritten by that command.
5. Start go run ./cmd/api and the administrator web. Confirm /readyz and the 38 /admin/contract resources.
6. Create stations and global categories through full-page administrator editors.
7. Partners register applications, upload documents, submit bank details and pay assessed registration fees. Administrators review approval and banking separately.
8. Configure Razorpay and TextBee, then verify provider-backed and native workflows before operational use.

Local development may use the guarded document fixture seed described in local-development.md. Never run destructive reset scripts; the old reset command has been removed.

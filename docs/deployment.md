# Deployment Guide

## Local

1. Copy `.env.example` into `.env` and set a strong `JWT_SECRET`.
2. Start dependencies with `make docker-up`.
3. Apply migrations with `make migrate`.
4. Run the API with `make run`.

To apply one repair migration to an existing database without replaying the full migration directory, run from `yember-core`: `go run ./cmd/migrate -file 019_ensure_order_delivery_address.sql`.

To enable password recovery, configure `EMAIL_DELIVERY_ENABLED=true`, `RESEND_API_KEY` and a verified `EMAIL_FROM`. Set `PASSWORD_RESET_URL` to the app deep link (`yember://auth/reset-password` by default). Legacy SMTP fields are not the current queued transactional-email transport. Phone OTP uses TextBee; see [central finance deployment](financial-system.md) for messaging, MFA, banking, scanning and live-activation configuration.

## Docker Compose

`docker-compose.yml` includes PostgreSQL and the API. The API waits for a healthy database before booting.

## Production Notes

- Apply the SQL migrations to the production database before deploying API code that depends on new columns. In particular, checkout requires `orders.delivery_address`; migration `019_ensure_order_delivery_address.sql` repairs databases that missed the original column migration.
- Use a managed PostgreSQL instance with backups and PITR.
- Store secrets outside images and compose files.
- Terminate TLS at a load balancer or reverse proxy.
- Keep `JWT_SECRET` long, random, and rotated through a planned key strategy.
- Add Prometheus scraping and tracing middleware before scaling horizontally.
- Replace the in-process event bus with Kafka or NATS when cross-process delivery is required.
- Add external cache, queue, or pub/sub infrastructure later when cross-instance rate-limit, realtime fanout, or async workload requirements appear.

# Deployment Guide

## Local

1. Copy `.env.example` into `.env` and set a strong `JWT_SECRET`.
2. Start dependencies with `make docker-up`.
3. Apply migrations with `make migrate`.
4. Run the API with `make run`.

## Docker Compose

`docker-compose.yml` includes PostgreSQL and the API. The API waits for a healthy database before booting.

## Production Notes

- Use a managed PostgreSQL instance with backups and PITR.
- Store secrets outside images and compose files.
- Terminate TLS at a load balancer or reverse proxy.
- Keep `JWT_SECRET` long, random, and rotated through a planned key strategy.
- Add Prometheus scraping and tracing middleware before scaling horizontally.
- Replace the in-process event bus with Kafka or NATS when cross-process delivery is required.
- Add external cache, queue, or pub/sub infrastructure later when cross-instance rate-limit, realtime fanout, or async workload requirements appear.

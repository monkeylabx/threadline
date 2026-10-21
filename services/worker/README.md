# Development Outbox Worker

NOT PRODUCTION. Requires the #297 envelope, #329 Stream and existing migrated
PostgreSQL Outbox. Supply `THREADLINE_WORKER_POSTGRES_DSN`,
`THREADLINE_WORKER_NATS_URL` (credential-free `nats://host:port`),
`THREADLINE_NATS_WORKER_USER`, `THREADLINE_NATS_WORKER_PASSWORD`,
`THREADLINE_OUTBOX_POLICY_FILE`, and a unique `THREADLINE_WORKER_OWNER_ID`.
`THREADLINE_WORKER_LISTEN_ADDRESS` defaults to `127.0.0.1:8081`.

Run `go run ./worker` from `services/`. `/healthz` is liveness; `/readyz` requires
startup inspection, a connected broker, and an available destination circuit.
The loop attempts one claim per 250 ms; it does not change persisted retry or
lease policy. SIGINT/SIGTERM cancel the active attempt and close dependencies.
Startup credentials are inspected on the same restricted inbox connection used
for publishing. Production credentials and deployment isolation remain #148.

Run `go test -race ./worker/...`. For the opt-in real-process test, create an empty
PostgreSQL 16.4 database named `threadline_worker_process_test_<suffix>`, install
pgcrypto and apply `db/migrations/*.up.sql` in order. Start/provision the pinned
NATS fixture from `deploy/nats`. Then set `THREADLINE_TEST_WORKER_DSN` and
`THREADLINE_TEST_WORKER_NATS_URL` and run
`go test ./worker/internal/process -count=1 -cover`. The test asserts real delivery
and no republish after restart. Remove the disposable database after testing.

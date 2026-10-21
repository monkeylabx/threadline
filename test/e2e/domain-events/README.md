# Real domain Event acceptance probes

Opt-in development-only process-boundary checks. Ordinary unit/CI test commands
do not execute these probes. No model, Agent runtime, production identity, E2EE,
Mobile or deployment gate is claimed.

Build Core, Worker and Realtime from the integrated tree. Supply absolute paths:

- `THREADLINE_TEST_CORE_BIN`, `THREADLINE_TEST_WORKER_BIN`, `THREADLINE_TEST_REALTIME_BIN`
- `THREADLINE_TEST_NATS_BINARY` (exact NATS 2.10.20)
- `THREADLINE_PG_BIN` (PostgreSQL 16.4 client directory)
- `THREADLINE_TEST_WORKER_DSN` for a disposable database named
  `threadline_worker_process_test_<suffix>` with pgcrypto and all ordered
  `db/migrations/*.up.sql` applied.

For `real_chain.py`, also load `deploy/compose/slice-a-seed.sql` into a fresh
fixture with no tenant-dev Events. Its private NATS includes #329's three scoped
principals. Each probe starts and cleans up its own processes on loopback ports.
Use a separate fresh database for each probe or rerun. For a draft stack,
`THREADLINE_TEST_NATS_CONFIG` may select the exact config exported from the
committed #329 platform branch; the default is the integrated repository config.

```sh
python3 test/e2e/domain-events/worker_outage.py
python3 test/e2e/domain-events/real_chain.py
```

Outage asserts three attempts then stopped claims with liveness 200/readiness
503, followed by all five rows delivered after broker restart. The real chain
asserts two authorized stream clients, exact Send retry deduplication, an
authenticated nonmember denial, cursor-1 reconnect receiving only seq-2, and
two durable delivered Events. Output is bounded receipts, never payloads/tokens.

# Slice A end-to-end (A13a)

Core-in-the-loop check of the plaintext Desktop loop: two Devices exchange
messages through `locald` + `core-client` (the Tauri host's exact path) against a
real Core and PostgreSQL; Core is stopped and restarted mid-run; a lost ACK is
retried. Scenario: `tests/desktop_loop.rs`; harness: `src/lib.rs`.

The test is NOT RUN unless both variables are set, so `cargo test --workspace`
stays hermetic:

| Variable | Meaning |
| --- | --- |
| `THREADLINE_E2E_CORE_BIN` | Path to a built `services/core` binary |
| `THREADLINE_E2E_POSTGRES_DSN` | Database with migrations applied and `deploy/compose/slice-a-seed.sql` loaded; `psql` must be on `PATH` |

CI runs it in the `slice-a-e2e` job of `.github/workflows/build.yml`. Output is
counts, sequences and reason tokens only; tokens, DSNs and envelope bytes are
never printed. The Task → Agent → approval → Artifact half of #260 is A13b.

# Core database foundation

This directory owns the PostgreSQL migration and sqlc inputs for
`threadline-core`. The M1 foundation creates only the physical schema namespace
`domain`; it does not define Organization, Channel, Message, identity, key, or
recovery tables.

## Migration rules

- Migration identifiers are immutable once merged. Every `*.up.sql` file has a
  matching `*.down.sql` file with the same numeric identifier.
- Each migration is transactional. Prefer expand/contract changes once durable
  application data exists; do not combine destructive cleanup with an expand
  step.
- Down migrations must name the exact object they own and must not use
  `CASCADE`. Production rollback or data deletion always requires a separately
  reviewed, visible approval; this local harness does not authorize either.
- Tests use only the generated `threadline_migration_test_<pid>` database name.
  Fixtures must be synthetic and must not contain credentials, tenant data,
  message content, keys, tokens, or production identifiers.

Run the static contract without a database:

```text
make -C db migration-static
```

Run the full `up -> down -> up` round trip against a disposable PostgreSQL 16.4
server. Authentication is supplied by the operator or local environment; do
not put a password in this repository or command history.

```text
PGHOST=127.0.0.1 PGPORT=5432 PGUSER=threadline_postgres_dev \
  make -C db migration-test
```

The live test compares normalized schema dumps and proves that a down migration
fails without deleting an unexpected object in the owned schema.

## Query generation

The reviewed generator is sqlc 1.31.1 from `toolchains.json`. Generated Go uses
`pgx/v5` and is written only to `services/core/internal/dbgen/`. Queries in this
foundation are content-free health checks; later domain tasks own business
tables and queries.

```text
node scripts/toolchain.mjs doctor --scope=database
make -C db generate
make -C db generate-check
```

`generate-check` runs generation twice and requires the tracked generated tree
to remain unchanged. Do not hand-edit generated files.

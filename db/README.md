# Core database persistence

This directory owns the PostgreSQL migration and sqlc inputs for
`threadline-core`. Migration `000001` creates the physical schema namespace
`domain`. Migration `000002` adds the root Organization/Tenant boundary only;
it does not define Member, Space, Channel, Message, Session, Device, key,
recovery, or Outbox tables.

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

## Organization persistence

`domain.organizations` stores the normalized fields of
`threadline.identity.v1.Organization`: stable `tenant_id`, directory
`display_name`, lifecycle `state`, `policy_version`, and server-assigned
`created_at`. State values use the published Protobuf numbers: `1` active, `2`
frozen, and `3` pending deletion. Unspecified and unknown values fail closed.

Generated writes expose creation plus a state/policy update. They do not expose
an identity or creation-time update. Every lookup and update requires an exact
Tenant identifier. This internal argument is not caller authority: a later RPC
layer must derive it from the authenticated P03-02 Session and must never copy a
caller-selected Tenant into these queries.

Run the synthetic lifecycle, duplicate, invalid-state, and wrong-Tenant
exact-key miss cases against the same pinned development server. These storage
checks do not model an authenticated caller; authorization remains the later
P03-02 RPC layer's responsibility.

```text
PGHOST=127.0.0.1 PGPORT=5432 PGUSER=threadline_postgres_dev \
  make -C db organization-test
```

The test creates only disposable identifiers containing `synthetic`. It uses no
production data, member content, message plaintext, credentials, tokens, keys,
Device, Crypto, Recovery, or Outbox data. Organization display names are the
unencrypted enterprise-directory data defined by the published contract.

## Query generation

The reviewed generator is sqlc 1.31.1 from `toolchains.json`. Generated Go uses
`pgx/v5` and is written only to `services/core/internal/dbgen/`. Query inputs
remain schema-qualified and Tenant-scoped; generated files are never hand
edited.

```text
node scripts/toolchain.mjs doctor --scope=database
make -C db generate
make -C db generate-check
```

`generate-check` runs generation twice and requires the tracked generated tree
to remain unchanged. Do not hand-edit generated files.

# Core database persistence

This directory owns the PostgreSQL migration and sqlc inputs for
`threadline-core`. Migration `000001` creates the physical schema namespace
`domain`. Migration `000002` adds the root Organization/Tenant boundary, and
migration `000003` adds Member directory/RBAC metadata. Migration `000004` adds
Space directory/policy-inheritance metadata. It does not define Channel,
Message, Session, Device, key, recovery, or Outbox tables.

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

The generated sqlc API has a separate Go integration test. It is explicitly
gated by an operator-supplied maintenance DSN, creates and drops its own
`threadline_organization_go_test_*` database, verifies PostgreSQL 16.4, and
never prints the DSN or credentials:

```text
THREADLINE_TEST_POSTGRES_DSN='<operator-supplied maintenance DSN>' \
  make -C db organization-go-test
```

It invokes `CreateOrganization`, `GetOrganization`, and
`UpdateOrganizationStatePolicy` directly, including exact-key misses,
duplicate and invalid-state failures, and immutable identity/creation-time
checks. Ordinary `go test ./...` skips this live test when the environment
variable is absent.

The test creates only disposable identifiers containing `synthetic`. It uses no
production data, member content, message plaintext, credentials, tokens, keys,
Device, Crypto, Recovery, or Outbox data. Organization display names are the
unencrypted enterprise-directory data defined by the published contract.

## Member persistence

`domain.members` stores the normalized fields of
`threadline.identity.v1.Member` beneath an existing Organization. Its immutable
identity is `(tenant_id, actor_type, actor_id)`. ActorType values use the frozen
Protobuf numbers `1` human, `2` agent, and `3` service; Role values are `1–5`;
MemberState values are `1–3`. Unspecified and unknown values fail closed.

Directory `display_name` and optional `org_unit_path` are synthetic C2 metadata
in tests. `joined_at` is server-assigned. Generated updates expose only Role and
MemberState, so identity and join time cannot be mutated through the typed
query surface.

Stored Role is not authorization. P03-05 must still intersect current Role,
Channel Membership, resource ACL, and any Capability Grant. Likewise, exact
Tenant query arguments are storage scope, not caller authority; P03-02 must
derive Tenant and Actor from the authenticated Session before calling them.

Run the synthetic duplicate, invalid-enum, Organization-FK, optional-path,
exact-key miss, and role/state lifecycle cases against PostgreSQL 16.4:

```text
PGHOST=127.0.0.1 PGPORT=5432 PGUSER=threadline_postgres_dev \
  make -C db member-test
```

The generated Member API has a separately gated live Go test. It creates and
drops its own `threadline_member_go_test_*` database and never prints the DSN or
credentials:

```text
THREADLINE_TEST_POSTGRES_DSN='<operator-supplied maintenance DSN>' \
  make -C db member-go-test
```

It invokes `CreateMember`, `GetMember`, and `UpdateMemberRoleState` directly and
verifies complete round-trip, exact-key misses, constraints, optional
org-unit-path behavior, and immutable identity/join time. Ordinary Go tests
skip this live test when the environment variable is absent.

## Space persistence

`domain.spaces` stores the normalized fields of `threadline.identity.v1.Space`
beneath an existing Organization. Its immutable identity is
`(tenant_id, space_id)`, identifiers must be nonblank and trimmed, and
`created_at` is server-assigned. Generated updates expose only directory
`display_name` and `discoverable`, so identity and creation time cannot be
mutated through the typed query surface.

Stored discoverability is directory metadata, not authorization. It describes
whether tenant members may find and request to join contained public Channels;
it does not grant Space or Channel access. P03-05 must still evaluate current
Membership, Role and resource ACL. Exact Tenant query arguments are likewise
storage scope rather than caller authority; P03-02 must derive Tenant and Actor
from the authenticated Session before invoking these queries.

Run the synthetic duplicate, Organization-FK, identifier, exact-key miss,
tenant-isolation, metadata lifecycle and immutable-field cases against
PostgreSQL 16.4:

```text
PGHOST=127.0.0.1 PGPORT=5432 PGUSER=threadline_postgres_dev \
  make -C db space-test
```

The generated Space API has a separately gated live Go test. It creates and
drops its own `threadline_space_go_test_*` database and never prints the DSN or
credentials:

```text
THREADLINE_TEST_POSTGRES_DSN='<operator-supplied maintenance DSN>' \
  make -C db space-go-test
```

It invokes `CreateSpace`, `GetSpace`, and `UpdateSpaceDirectoryMetadata`
directly and verifies complete round-trip, exact-key misses, constraints,
same-ID cross-Tenant isolation, and immutable identity/creation time. Ordinary
Go tests skip this live test when the environment variable is absent.

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

# Core database persistence

This directory owns the PostgreSQL migration and sqlc inputs for
`threadline-core`. Migration `000001` creates the physical schema namespace
`domain`. Migration `000002` adds the root Organization/Tenant boundary, and
migration `000003` adds Member directory/RBAC metadata. Migration `000004` adds
Space directory/policy-inheritance metadata. Migration `000005` adds Channel
directory/lifecycle metadata plus Direct Message identity and immutable
participant rows. It does not define Channel Membership, Message, Session,
Device, key, recovery, or Outbox tables.

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

All live shell tests source `tests/postgres_harness.sh` for the PostgreSQL 16.4
version gate, pinned tool resolution, disposable-database lifecycle, cleanup
guards, and secret-safe diagnostics. The Organization, Member, Space, and
Channel/DM typed Go tests likewise share `postgres_integration_test.go` for the
operator-supplied DSN gate, maintenance/test connection lifecycle, migration
loading, version check, and guarded database deletion. Aggregate test files
contain only their domain fixtures and assertions.

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

## Channel and Direct Message persistence

`domain.channels` stores minimal `threadline.channel.v1.Channel` directory and
lifecycle metadata beneath a tenant-scoped Space. Its immutable identity is
`(tenant_id, channel_id)`. Name is normalized nonempty C2 directory data;
visibility uses published values `1` public and `2` private, while lifecycle
state uses `1` active, `2` archived and `3` pending deletion. The opaque
`e2ee_group_id` binding and server-assigned `created_at` are immutable through
the generated API. Encrypted topic, RetentionPolicy and ChannelAgentPolicy are
deliberately outside this slice.

`domain.direct_messages` stores tenant-scoped DM identity, opaque E2EE Group
binding and creation time. `domain.direct_message_participants` normalizes the
immutable Actor set with tenant-scoped foreign keys to both the DM and Member.
DM creation is one transaction: create the unsealed parent, append the intended
participants, then call `FinalizeDirectMessageParticipants`. A deferred
database constraint rejects commit while the parent remains unsealed. Database
triggers reject participant append after finalization, every participant update
or delete, any attempt to reopen a sealed set, and every change to the
tenant-scoped `(tenant_id, dm_id)` identity. Identity immutability also ensures
the deferred sealing check cannot be bypassed by moving an unsealed row to a new
key before commit. The triggers intentionally impose no minimum participant
count because that domain rule is not present in the proto contract. The append
trigger locks the parent DM row, so participant insertion and finalization
serialize on that row and a concurrent insert cannot cross the sealed
transition. The composite participant-to-DM and participant-to-Member foreign
keys remain authoritative after that lifecycle check. Generated queries expose
participant creation and deterministic reads only; there is no participant
update or delete operation. Production
create-or-get concurrency behavior remains a later RPC transaction concern.

Visibility, state, participant rows and E2EE Group identifiers are routing
facts, not authorization or proof of key possession. P03-02 must derive Tenant
scope from the authenticated Session, P03-05 owns effective Membership/RBAC/ACL
decisions, and the Crypto owner admits actual E2EE Group state.

Run the synthetic constraints, exact-Tenant isolation, immutable-field and
failed-DM-transaction cases against PostgreSQL 16.4:

```text
PGHOST=127.0.0.1 PGPORT=5432 PGUSER=threadline_postgres_dev \
  make -C db channel-dm-test
```

The generated Channel/DM API has a separately gated Go integration test. It
creates and drops only a guarded `threadline_channel_dm_go_test_*` database and
invokes every generated Channel/DM operation directly:

```text
THREADLINE_TEST_POSTGRES_DSN='<operator-supplied maintenance DSN>' \
  make -C db channel-dm-go-test
```

Fixtures contain only synthetic C2 directory/routing metadata. They contain no
message or topic plaintext, prompt, credential, token, Device or MLS key,
Epoch/History/Recovery material, or production identifier.

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

# Transactional Outbox Storage And Relay Contract

Status: frozen by P03-08A

This contract turns the Transactional Outbox decision in
[ADR-0002](../adr/0002-server-protocol-storage.md) into an implementation
boundary for Core, PostgreSQL, Worker, and JetStream. It freezes storage and
relay semantics, not a public event Protocol or a Worker implementation.

## 1. Purpose And Sources

PostgreSQL is the fact source. Core commits a Domain Event and its initial
Transactional Outbox Entry in the same caller-owned transaction as the domain
change. Worker later relays that committed fact at least once. JetStream is a
delivery layer that can be rebuilt; it cannot create, widen, or rewrite the
fact.

This contract derives from these accepted ADR-0002 rules:

- Core is the only writer of Domain rows and the initial Outbox Entry.
- PostgreSQL commit precedes Durable Client ACK and broker publish.
- Worker may update only Outbox delivery facts, Job facts, and its own
  projections.
- NATS failure delays delivery without preventing Core from committing.
- Worker and consumers accept duplicates and use business Event identity for
  idempotency.
- A Parking Stream or DLQ is operational delivery state, not a replacement
  fact source.

## 2. Canonical Language

The canonical definitions live in [`CONTEXT.md`](../../CONTEXT.md):

- **Domain Event** is the immutable Tenant-scoped business fact.
- **Transactional Outbox Entry** is mutable delivery work for one Domain Event
  and one destination.
- **Delivery Claim** is short-lived fenced authority for one Worker attempt.

Use **Event Type** for the stable domain name, for example
`message.committed`. Do not call it a topic. A broker subject is a replaceable
relay mapping derived from destination, Event Type, and schema version.

A JetStream PubAck proves that one publish was accepted by the configured
stream. It is not a consumer ACK, a Domain commit, or proof that every consumer
applied the event.

## 3. Non-Negotiable Invariants

1. Domain state, Domain Event facts, and the initial Outbox Entry commit or
   roll back together.
2. Core never returns a Durable Client ACK and Worker never publishes before
   that PostgreSQL transaction commits.
3. Event identity, Tenant, type, version, payload, and occurrence time are
   immutable after insert. Each destination Entry references that one Event.
4. Worker cannot insert a Domain Event, rewrite immutable event facts, or
   select a different Tenant or destination during acknowledgement.
5. One `(tenant_id, event_id, destination)` identifies one Outbox Entry.
6. Only the current, unexpired Delivery Claim may transition, renew, fail, or
   park an entry. Re-observing an already completed exact acknowledgement is a
   non-mutating result, not renewed authority. Entry ID alone is never authority.
7. Delivery is at-least-once. A crash after publish and before PostgreSQL
   acknowledgement can publish the same event again.
8. Every consumer deduplicates database effects by `(tenant_id, event_id)` in
   the same transaction. External effects first create durable downstream work
   atomically, then deliver that work at least once with the same idempotency key.
9. Payload is opaque, versioned bytes. Relay code does not deserialize it to
   make authorization or routing decisions.
10. Logs, metrics, errors, DLQ views, and diagnostic bundles never include
    payload bytes, raw claim tokens, or other C3/C4 data. A live Claim response
    necessarily carries its opaque payload and one-time raw token on the
    authenticated Worker database boundary only.

## 4. Logical Storage Contract

The complete design has four logical relations. P03-08B implements the first
three; the replay relation is deferred until the Approval/Audit dependency is
available. The separate Event and Entry relations retain PostgreSQL's exact
immutable Event plus destination set even when old attempt evidence expires.

### 4.1 `domain.domain_events`

| Fact | Owner | Mutability | Required rule |
| --- | --- | --- | --- |
| `tenant_id`, `event_id` | Core | immutable | Composite primary identity; both canonical and non-empty |
| `event_type` | Core descriptor | immutable | Registered lowercase dotted name; never an RPC name or broker subject |
| `schema_version` | Core descriptor | immutable | Positive registered version; consumers reject unknown values |
| `aggregate_kind`, `aggregate_id` | Core descriptor | immutable | Registered kind plus canonical Tenant-scoped identity; diagnostic, not authority |
| `payload` | Core descriptor | immutable | Opaque versioned bytes within the trusted deployment hard limit |
| `occurred_at` | Core transaction | immutable | Domain occurrence time; never Worker wall-clock time |
| `enqueued_at` | database | immutable | PostgreSQL transaction timestamp; not claimed to be physical commit instant |

The database prevents updates and ordinary deletes. An Event Type registry
owned by the producing Core/Contracts task binds type, schema version,
aggregate kind, permitted logical destinations, and payload ceiling before any
production insert API is exported. Canonical-shape checks alone are not a
registry and cannot make an unknown Event Type valid.

### 4.2 `domain.transactional_outbox`

| Fact | Owner | Mutability | Required rule |
| --- | --- | --- | --- |
| `outbox_entry_id` | database | immutable | Stable non-request-controlled delivery identity |
| `tenant_id`, `event_id` | Core | immutable | Composite foreign key to the exact Domain Event |
| `destination` | Core descriptor | immutable | Registered logical destination; never arbitrary request or broker address |
| `delivery_state` | Worker relay | state machine | Exactly `pending`, `claimed`, `delivered`, or `parked` |
| `total_attempt_count` | database claim query | monotonic | Lifetime count; never reset by replay |
| `replay_generation` | replay command | monotonic | Starts at zero; explicit approved replay increments it |
| `generation_attempt_count` | database claim query | generation-local | Reset only when an approved replay creates a new generation |
| `generation_failure_count` | database failure query | generation-local | Counts only allowlisted event-specific retryable failures |
| `next_attempt_at` | database failure query | mutable | Non-null only for `pending`; computed from trusted policy and database time |
| `current_attempt_id` | database claim query | mutable | Non-null only for `claimed`; references the sole active attempt |
| `last_failure_code` | database failure query | current-generation summary | Allowlisted code or null; never raw text |
| `parked_at` | database failure query | current-generation summary | Non-null only for `parked`; cleared only by approved replay |

Required constraints include unique `(tenant_id, event_id, destination)`, exact
composite Tenant/Event foreign keys, known non-zero state, non-negative counters,
and a partial uniqueness rule allowing at most one active attempt per Entry.
`pending` requires `next_attempt_at`; `claimed` requires `current_attempt_id`;
`delivered` and `parked` require both to be null. PubAck evidence lives only on
the immutable terminal attempt, so replay never clears or overwrites it.

### 4.3 `domain.outbox_delivery_attempts`

Each claim creates one append-only attempt. Identity, Tenant/Event/Entry
binding, generation, ordinals, claim owner label, token digest, claim time, and
absolute lease cap are immutable. Only the current lease expiry may increase
under its absolute cap; terminal fields are filled once.

| Fact | Owner | Required rule |
| --- | --- | --- |
| `delivery_attempt_id` | database | Stable attempt identity and fencing reference |
| `tenant_id`, `event_id`, `outbox_entry_id` | database claim query | Exact composite parent binding used by every later operation |
| `replay_generation` | database claim query | Copies the current Entry generation |
| `total_attempt_number`, `generation_attempt_number` | database claim query | Exact incremented lifetime and generation-local ordinals |
| `claim_owner_id` | Worker adapter | Bounded diagnostic replica label; DB role plus claim token provide authority |
| `claim_token_digest` | database | Digest of a database-generated token with at least 128 bits of entropy |
| `claimed_at`, `lease_expires_at`, `absolute_lease_expires_at` | database | PostgreSQL time; renewal only increases lease expiry up to the immutable cap |
| `outcome` | Worker/database | One of `active`, `delivered`, `transport_unavailable`, `publish_outcome_unknown`, `event_retryable`, `event_permanent`, `lease_expired` |
| `finished_at` | database | Required for every non-active outcome |
| `failure_code` | Worker relay | Same allowlisted code as relay-classified outcomes; null for database-classified `lease_expired` |
| `broker_stream`, `broker_sequence`, `broker_duplicate` | trusted Worker adapter | Bounded normalized PubAck evidence required only for `delivered` |
| `broker_message_id` | database/Worker adapter | Exact deterministic message ID for this Entry generation |

An active attempt has no terminal fields. A delivered attempt has exact PubAck
evidence and no failure code. `transport_unavailable`,
`publish_outcome_unknown`, `event_retryable`, and `event_permanent` have the
corresponding allowlisted failure code and no PubAck evidence. `lease_expired`
is assigned by the database and has null failure code because its outcome is
already the stable reason. Attempt history is not rewritten by retry or replay.
The raw claim token is returned once with the live Claim response and is never
stored in logs, metrics, errors, DLQ data, or diagnostic views.

### 4.4 `domain.outbox_replay_requests`

This relation and its query surface are **not created by P03-08B**. P03-08D is
blocked on P03-07's durable Approval/Audit contract and will freeze its exact
primary/foreign keys, state/result enum, target binding, and concurrent replay
semantics before migration. At minimum, replay evidence is append-only and
binds Tenant, Entry, old/new generation, current Authenticated Principal Actor,
policy version, allowlisted replay reason, Approval ID, Audit ID, database time,
and result. Replay always requires current visible Approval and durable Audit;
request fields cannot provide or replace those facts. Missing, stale,
cross-Tenant, or target-mismatched Approval fails closed without revealing row
existence. Until P03-08D lands, no replay API is exported and delivered or
parked Entries cannot be replayed.

## 5. Ownership And Database Authority

| Actor | May do | Must not do |
| --- | --- | --- |
| Core command transaction | Insert one immutable Event plus its initial `pending` destination Entries | Publish, claim, acknowledge, retry, park, replay, or edit an existing Event |
| Worker relay role | Read committed payload/facts; use reviewed claim/renew/ack/fail operations; append/close attempts | Insert entries; update immutable facts; change Tenant/destination; generic table update/delete |
| Consumer | Atomically record inbox/idempotency plus database effect or downstream work | Mark PostgreSQL Outbox delivery, claim external effects are transactionally applied, or treat broker order as Domain order |
| Future operator replay command (P03-08D) | Resolve trusted Principal/Approval/Audit facts and request one visible replay | Exist before P03-07 dependencies land; accept caller-supplied identity/evidence; edit Event facts; reset attempt history; forge a Worker claim |
| Realtime/Runtime Gateway | Consume broker hints or dispatches | Hold an Outbox database credential or acknowledge Outbox rows |

P03-08B implements a Core insert surface only; P03-08D owns replay. Go
`internal` package rules mean Worker cannot consume Core's current
generated package; Integration must add a Worker-owned sqlc output or reviewed
database functions plus a Worker wrapper as a separate shared-surface task.
Worker receives no generic `INSERT`, `UPDATE`, or `DELETE` authority on Domain
tables. Deployment/Platform provisions credentials and non-login grants;
feature migrations do not create or drop login roles. A migration owner, not
runtime startup, creates tables, functions, grants, and triggers.

## 6. State Machine

```text
pending ----> claimed ---- verified PubAck ----> delivered
   ^             |
   |             +---- transport unavailable / unknown publish
   |             |       (backoff; does not consume event-failure budget)
   |             |
   |             +---- event-specific retryable failure below generation limit
   |             |
   |             +---- event-specific permanent failure or exhausted limit ---> parked
   |                                                                              |
   +---------------- future mandatory approved + audited replay (P03-08D) --------+
   +---------------- future approved + audited delivered replay (P03-08D) <-------- delivered

claimed -- lease expiry --> immediate atomic replacement claim
                           (no backoff; no event-failure budget)
```

The `claimed -> pending -> claimed` replacement of an expired claim is one
PostgreSQL transaction: it closes the old attempt as `lease_expired`, clears
the old fence, creates a new attempt/token, increments lifetime and generation
attempt counters, and returns only the new claim. No observable state exists
where both claims are current. Lease expiry does not consume the
event-specific failure budget.

`delivered` is terminal for automatic relay. Replay of `delivered` or `parked`
is always a high-impact operation: it requires current visible Approval and
durable Audit in the same Core transaction. Replay increments the generation,
resets only generation-local counters/summaries, preserves the Event and every
attempt, and causes the next claim to record the new generation. It is never a
Worker retry endpoint.

Entry postconditions are exact:

| State | `next_attempt_at` | `current_attempt_id` | `last_failure_code` | `parked_at` |
| --- | --- | --- | --- | --- |
| `pending` | required | null | null or the last allowlisted current-generation failure | null |
| `claimed` | null | required active attempt | unchanged summary or null | null |
| `delivered` | null | null | null | null |
| `parked` | null | null | required allowlisted event failure | required |

Approved replay moves `delivered` or `parked` to `pending`, increments the
generation, resets both generation counters, and sets failure/park summary to
null. Lifetime count and all Event/attempt/replay rows remain unchanged.

## 7. SQL-Shaped Operations

These are semantic interfaces, not final sqlc names. Each operation runs at
`READ COMMITTED`, uses PostgreSQL time, and returns a secret-safe typed result.
Core operations derive Tenant/Actor from the Authenticated Principal. Worker
operations use the exact Tenant/Event/Entry/attempt/generation tuple returned by
Claim plus the raw token; they cannot substitute an independently supplied
Tenant.

### `InsertDomainEventAndEntries(transaction, event descriptor, event facts)`

- Called only inside the same caller-owned transaction as the Domain change.
- Accepts a trusted registered descriptor rather than caller-controlled Event
  Type/version/destination strings.
- Inserts one immutable Domain Event and one `pending` Entry for every exact
  descriptor destination, with all counters and replay generation zero.
- A duplicate `(tenant_id, event_id)` returns the existing exact Event only when
  every immutable fact and destination set match; otherwise it fails closed.
- Oversized payload, unregistered descriptor, or malformed identity aborts the
  whole Domain transaction.

### `ClaimBatch(trusted Worker adapter, bounded replica label)`

- Batch size and lease duration come from trusted deployment policy, never a
  client or event payload.
- A Worker-side destination circuit breaker stops claiming while the broker is
  known unavailable. This prevents an infrastructure outage from churning the
  entire Outbox.
- Selects eligible `pending` entries where `next_attempt_at <= database_now` and
  expired `claimed` entries. Expired claims are ordered by
  `lease_expires_at`, then Event enqueue time and Entry ID.
- Uses `FOR UPDATE SKIP LOCKED` and deterministic order:
  eligibility time (`next_attempt_at` or expired `lease_expires_at`), Event
  `enqueued_at`, then `outbox_entry_id`.
- In the same transaction, closes any expired attempt, increments lifetime and
  generation attempt counts, creates a database-generated raw token and stored
  digest plus lease/absolute cap, points the Entry at that attempt, and returns
  immutable Event facts plus the new Claim. The raw token and payload are
  separate response fields.
- Concurrent Workers cannot receive the same current claim.

### `RenewClaim(exact Claim tuple, bounded replica label, raw claim token)`

- Succeeds only for the exact active, unexpired claim and current replay
  generation.
- Extends monotonically to the lesser of `database_now + policy_lease_duration`
  and the immutable absolute lease cap; caller timestamps are ignored. Renewal
  after the cap fails and the Entry becomes eligible for replacement.
- A missing, expired, mismatched, or superseded token changes nothing.

### `AcknowledgePublished(exact Claim tuple, replica label, raw claim token, PubAck)`

- First-transition branch requires the exact active, unexpired Claim. The
  trusted Worker adapter supplies normalized stream, sequence, duplicate flag,
  and exact deterministic message ID from the current publish future; the DB
  verifies message ID and bounds but does not pretend to authenticate NATS.
- A JetStream PubAck with `Duplicate=true` is successful when it is the PubAck
  for that exact message ID and destination stream.
- Atomically closes the attempt as `delivered` and sets the entry `delivered`.
- Cannot supply or change Tenant, Event Type, payload, destination, or times.
- Observation branch may return typed `already-delivered` only when the exact
  completed attempt, Worker label, raw-token digest, and normalized PubAck all
  match. It performs no transition and confers no current authority. Any other
  completed, expired, stale, or mismatched claim is denied.

### `RecordPublishFailure(exact Claim tuple, bounded replica label, raw claim token, failure code)`

- Requires the exact active, unexpired claim.
- Accepts only the frozen enum: `transport-unavailable`,
  `publish-outcome-unknown`, `event-retryable`, or `event-permanent`. Unknown
  strings fail closed; raw broker/SQL errors never cross this boundary.
- `transport-unavailable` and `publish-outcome-unknown` close the attempt and
  return the Entry to `pending` with capped backoff without consuming the
  generation event-failure budget. A long NATS outage therefore cannot park
  every Event. Unknown publish outcome may duplicate on retry.
- `event-retryable` increments the generation failure count. Below the trusted
  generation ceiling it schedules capped backoff; at the ceiling it parks.
- `event-permanent` parks immediately. PostgreSQL parked state remains truth
  even if a Parking Stream notification fails.

### `RequestReplay(context, entry ID, registered reason)`

- Is a P03-08D semantic interface only; P03-08B does not create or export it.
- Is a high-impact Core/operator command, never a Worker operation. Tenant and
  Actor come only from the current Authenticated Principal.
- Resolves current exact Tenant-scoped policy plus target-bound Approval and
  Audit facts inside the caller-owned transaction. Approval is mandatory for
  both `parked` and `delivered` replay; caller-supplied IDs or evidence cannot
  replace current facts.
- Accepts only registered reasons `destination-rebuild`, `consumer-repair`, or
  `operator-recovery`; unknown strings fail closed.
- Appends replay evidence, increments `replay_generation`, resets generation
  attempt/failure counters, clears current-generation failure/park summary,
  and schedules `pending` in one transaction.
- Does not change immutable Event facts, delete attempts, or reuse a claim.
- Missing, stale, cross-Tenant, already-used, or target-mismatched Approval is a
  secret-safe denial indistinguishable from an unavailable target.

### `PurgeDeliveryEvidence(trusted retention policy)`

- Is unavailable in P03-08B and to ordinary Core/Worker roles.
- A later Retention-owned implementation uses database time and a trusted
  policy cutoff, never a request-supplied Tenant or duration.
- It may delete only expired terminal attempt/replay evidence after every
  delivery/audit retention gate passes. It never deletes an Event or Entry.
- The permanently retained Event plus its immutable destination Entries remain
  the exact PostgreSQL rebuild source. It cannot delete facts required to
  explain a Durable Client ACK.

## 8. Publish And Consumer Contract

Worker derives the broker subject from the registered destination, Event Type,
and schema version. Tenant IDs, aggregate IDs, paths, and payload content do not
enter the subject. The v1 logical destination registry starts with
`domain-events`; adding another destination is a Contracts change, not a runtime
string. The published envelope includes at least:

- `tenant_id` and `event_id` as the idempotency key;
- Event Type and schema version;
- occurrence time;
- opaque payload bytes;
- the Outbox Entry ID for diagnostics only.

For one Entry generation, every automatic retry uses the same JetStream message
ID. Its v1 preimage is the ASCII domain separator
`threadline.outbox.msg-id/v1\0`, followed in order by `tenant_id`, `event_id`,
and `destination`; each string is encoded as an unsigned 32-bit big-endian UTF-8
**byte** length followed by those exact bytes. The final field is
`replay_generation` as an unsigned 64-bit big-endian integer. Inputs exceeding
those widths fail before hashing. The message ID is the lowercase hexadecimal
SHA-256 digest of that preimage. Different destinations and approved replay
generations therefore cannot suppress one another inside a broker deduplication
window. Correctness still never relies on that window. P03-08C must commit at
least one shared preimage/digest Golden fixture used by both database and Worker
tests before the generator is reachable.

The required v1 Golden fixture uses `tenant_id="t"`, `event_id="e"`,
`destination="domain-events"`, and `replay_generation=0`:

```text
preimage_hex = 7468726561646c696e652e6f7574626f782e6d73672d69642f763100000000017400000001650000000d646f6d61696e2d6576656e74730000000000000000
sha256_hex   = e57ad815a402753dd7698b0e941f70108383c92afecfc5d0c2b699ac36c82e97
```

Database consumers atomically store their inbox/idempotency record together
with their projection/cursor change. A consumer that needs APNs, dispatch,
webhook, or another external effect atomically stores durable downstream work
instead; a separate relay performs that effect at least once using Tenant +
Event ID + registered effect/destination as its idempotency key. No contract
claims PostgreSQL can atomically commit an external side effect.

## 9. Failure Timeline Matrix

| Failure point | Required result |
| --- | --- |
| Domain transaction rolls back | Domain change, Event, and Outbox Entry are all absent; no Durable ACK or publish |
| Core crashes after commit before client ACK | Client retries the same idempotency key; Core returns the same committed result without a second Event |
| NATS is unavailable | Core keeps committing; destination circuit opens, active claims return to pending with infrastructure backoff, and no event-failure budget is consumed; recovery resumes automatically |
| Worker crashes before publish | Claim expires; replacement attempt closes the old attempt as `lease_expired` |
| Worker crashes during publish with unknown result | Replacement may publish again; consumers deduplicate |
| Worker receives PubAck then crashes before DB ACK | Replacement may publish again; consumers deduplicate |
| JetStream returns `Duplicate=true` PubAck | Exact message-ID/stream correlation accepts it as successful publish evidence |
| Old Worker ACK arrives after renewal expiry/reclaim | Exact token/attempt/generation check rejects it without state change |
| Two Workers claim the same batch | Row locking plus `SKIP LOCKED` returns one current claim per entry |
| PostgreSQL fails during ACK | Worker treats delivery as unacknowledged unless commit is confirmed; later duplicate is acceptable |
| Event-specific retry ceiling is reached | Entry becomes durably `parked`; infrastructure/unknown-outcome attempts never consume this budget |
| Parking Stream publish fails | PostgreSQL remains `parked`; operational notification can be rebuilt |
| Operator requests replay | New replay generation and later attempt are auditable; immutable Event and old attempts remain unchanged |
| Database consumer crashes after transaction before consumer ACK | Redelivery observes the inbox row and does not repeat the database effect |
| External-effect relay crashes | Durable downstream work redelivers with the same Tenant/Event/effect-or-destination idempotency key |
| Fake or cross-Tenant ACK is submitted | Exact Tenant/Entry/Event/attempt/generation/token join rejects it with the same secret-safe `claim-denied` result |

## 10. Retry, Replay, Parking, And Purge

Deployment policy owns a finite event-specific retry ceiling, maximum batch
size, claim lease and absolute lifetime, capped infrastructure/event backoff,
jitter source, payload byte limit, and evidence retention windows. These are
reviewed configuration, never values in a Domain command, payload, Worker
request, or replay request.

Automatic retry requires the exact current Claim and never resets history.
Infrastructure unavailability and unknown publish result can retry indefinitely
with capped backoff/circuit breaking because parking all events during an outage
would violate ADR-0002 recovery. Lease expiry is replaced atomically without
backoff. Only event-specific retryable failures consume the per-generation
ceiling; permanent event failure parks immediately. Deployment alerting must
cover circuit-open duration, oldest eligible Entry age, and total/generation
attempt counts; terminal attempt retention bounds historical storage without
deleting the Entry rebuild fact. Future replay always binds trusted Actor,
Tenant, policy, current Approval, Audit, and an allowlisted reason. It resets
only generation counters and creates a different broker message ID.

Outbox purge is disabled for P03-08B. Ordinary Core and Worker roles cannot
delete Events, Entries, attempts, or replay evidence. A later Retention-owned,
reviewed purge operation may delete expired terminal attempt/replay evidence
only after all configured evidence windows pass. Exact Event and destination
Entry rows remain the PostgreSQL rebuild source. Parked or unresolved replay
data is never automatically purged. Domain Event deletion belongs to the Domain
Retention contract, not Outbox relay, and
cannot be inferred from delivery completion.

## 11. Payload And Observability Safety

The payload may contain a versioned ciphertext event and necessary C1/C2
metadata. It must not contain:

- message, file, Artifact, Context, tool output, or Prompt plaintext;
- bearer tokens, route credentials, Capability Grant bytes, nonce values, or
  claim tokens;
- Device private keys, Content/History keys, MLS secret material, recovery key
  material, or KMS/HSM responses;
- arbitrary stack traces, SQL errors, external service bodies, local paths, or
  environment/configuration dumps.

Logs may include bounded identifiers, Event Type/version, delivery state,
attempt number, duration, payload byte count, and stable error code. They never
include payload bytes, claim tokens, broker credentials, or raw errors. Metrics
avoid Tenant and Event IDs as labels.

## 12. Stable Results And Allowlists

Database/repository operations expose only these stable result categories:

- `invalid-input`: malformed trusted internal arguments;
- `unavailable`: missing, cross-Tenant, policy-denied, or Approval-unavailable
  replay target, deliberately indistinguishable;
- `claim-denied`: missing, expired, mismatched, or replaced Worker Claim;
- `already-delivered`: exact non-mutating observation of the prior identical
  acknowledgement;
- `retry-scheduled`: current attempt closed and pending backoff scheduled;
- `parked`: current generation durably parked;
- `persistence-failure`: a database invariant or transaction prevented a safe
  result.

An error renders only `transactional outbox: <category>`. It never includes a
Tenant/Event/Entry ID, payload, path, token, PubAck, raw database/broker error,
or row-existence detail.

The only relay failure codes are `transport-unavailable`,
`publish-outcome-unknown`, `event-retryable`, and `event-permanent`. The only
replay reasons are `destination-rebuild`, `consumer-repair`, and
`operator-recovery`. Database constraints and domain adapters reject unknown
values; a bounded arbitrary string is not an allowlist.

Normalized PubAck evidence has a destination-registered stream name, positive
broker sequence, boolean duplicate flag, and exact 64-character lowercase-hex
message ID. Broker descriptions, subjects, headers, errors, and caller-selected
ack IDs are not stored.

## 13. Follow-Up Implementation Contract

### P03-08B: PostgreSQL storage and Core insert primitives

The Core Migration owner must reserve the next migration ID and is expected to
own these surfaces:

- `db/migrations/<reserved>_transactional_outbox.up.sql`
- `db/migrations/<reserved>_transactional_outbox.down.sql`
- `db/queries/core/transactional_outbox.sql`
- generated `services/core/internal/dbgen/` output through the repository's
  integration workflow;
- PostgreSQL integration tests for immutable Event facts, exact destination
  foreign keys, entry/attempt/generation state constraints, and rollback.

Before claiming P03-08B, Integration must pin the trusted payload byte limit,
batch limit, claim lease/absolute lifetime, event-failure ceiling,
backoff/jitter policy, token encoding/digest, and delivery-evidence retention.
P03-08B keeps purge, replay, and production insert unexported. A producer
Contract task must register at least one Event
Type/version/aggregate/destination descriptor before a production insert can
become reachable; canonical strings are not a fallback registry.

### P03-08C: Worker query/module integration

Integration must add a Worker-owned sqlc/query surface, expected under
`db/queries/worker/` with generated output consumable from
`services/worker/internal/`. It cannot reuse
`services/core/internal/dbgen` across Go's `internal` boundary. Platform owns
database credential provisioning; the feature migration may grant a reviewed
non-login role but does not create login credentials.

### P03-08D: approved replay storage and command

P03-08D is blocked on P03-07's durable Approval/Audit contract. It must freeze
the replay relation's exact keys, state/result constraints, target binding, and
concurrent replay behavior before adding a migration or exporting
`RequestReplay`. No placeholder Approval or Audit IDs are permitted.

### Later Worker relay

The Realtime/Worker workstream implements JetStream mapping, publish, PubAck
correlation, retry metrics, Parking Stream projection, and consumer fixtures
only after P03-08B/P03-08C land. It consumes reviewed query/domain interfaces
and never receives a generic Domain database writer. Destination circuit-breaker
and PubAck verification live in the trusted Worker adapter, not SQL parameters
pretending to authenticate NATS.

### Later event Contract

A public cross-workload Event envelope, consumer-specific schemas, and Golden
Frames require a separate Contracts task. This document does not authorize a
feature Agent to add or guess Protobuf fields.

# Ciphertext message commit and Durable ACK transaction

Status: Draft for #193, 2026-09-07; baseline
`66a1e75325f05f8ef6c53c2e3f88f185567ebbab`. Documentation only: no schema,
producer registration, message service or client synchronization is implemented
by this task. M0/G0 remains HOLD. Review the proposed decisions below before
splitting implementation; existing Proto and frozen Outbox behavior take priority.

## Inputs and unimplemented prerequisites

The wire facts are [MessageService](../../proto/threadline/message/v1/message_service.proto),
[ChannelEventEnvelope](../../proto/threadline/message/v1/envelope.proto),
[Sync types](../../proto/threadline/sync/v1/sync.proto),
[SyncService](../../proto/threadline/sync/v1/sync_service.proto) and
[errors](../../proto/threadline/type/v1/error.proto).
The [Outbox contract](../architecture/transactional-outbox-contract.md) makes
PostgreSQL authoritative and NATS rebuildable. Durable client ACK, JetStream
PubAck and consumer application are three different facts.

Existing [authorization/current](../../services/core/internal/authorization/current/current.go)
accepts only Space/Channel references and does not check Session/Device/Epoch.
[#192](https://github.com/monkeylabx/threadline/issues/192) supplies a separately
reviewed session/device draft; it must be aligned with this transaction before
implementation. No dependency on its uncommitted files is allowed.
[Outbox insert](../../services/core/internal/outboxstore/store.go) and its
candidate/descriptor types remain package-private until a registered producer
contract exists. A new message command cannot simply call a public insert API:
there is none yet. The producer descriptor, encoder, policy and destination
registration are explicit precursor tasks, never request-supplied routing.

Required new seams: trusted device proof and current session rows; group/epoch
and membership-authorization storage; DM permission evaluator; registered message
producer; message/sequence/idempotency storage; authoritative envelope comparison
and checkpoint encoding. None is assumed to exist because its Proto does.

## Identity and retry decisions

The following key scopes are proposed storage decisions, not new wire fields:

| Fact | Scope / proposed rule |
| --- | --- |
| Logical event | `(tenant_id, event_id)` unique across that tenant, as Proto requires |
| Conversation sequence | `(tenant_id, conversation_kind, conversation_id, channel_seq)` unique; Channel and DM IDs cannot alias |
| Send command | `(tenant_id, sender_device_id, idempotency_key)` unique; bound permanently to one event and conversation |
| Outbox entry | Existing `(tenant_id, event_id, destination)`; initial destination selected from trusted producer policy |

A retransmission preserves the original pending envelope, event ID and key. Same
command plus same sender-authored values/body/signature and unknown bytes returns
the original committed response, `deduplicated=true`, without allocating a new
sequence or outbox entry. Same key with different event, conversation, body or
sender-authored metadata returns `ERROR_CODE_IDEMPOTENCY_CONFLICT`; the first
write stands. Reusing an event ID under a different key/device also conflicts,
without disclosing another sender's event. Another tenant's key never matches.

Equality is over the complete submitted immutable envelope, not content_hash
alone. Known fields compare typed values; bytes (including body/signature and
retained unknown fields) compare exactly. Do not drop unknown data, substitute
JSON or normalize timestamps/body bytes to force equality. A follow-up comparison
fixture must freeze parser edge cases, nested unknowns and duplicate singular
fields before code; client retries reuse durable bytes. ServerCommit must be
absent from any submitted request, including retries. This comparator does not
invent or replace the separate cryptographic signature/AAD canonicalization.

Authorization is checked before revealing a deduplicated result. A previously
committed event does not authorize a revoked caller to retrieve it. After a
current authorized caller is identified, an exact old commit returns its original
receipt even if the current Epoch advanced; it creates no new old-Epoch event.
Only the new-write path requires current Epoch/rekey validation. Otherwise an
ACK lost during an Epoch transition could strand a legitimate pending event.

The retention lifetime and redacted tombstone format for the command-to-event
binding must be reviewed with Retention before implementing deletion. While a
receipt remains valid, never accept its key as a fresh command. Once retention
prevents returning the original event, return a stable retention/unavailable
outcome rather than resurrecting it. This draft does not promise indefinite
ciphertext retention or treat Worker attempt evidence TTL as send dedup TTL.

## Proposed transaction schedule

Use a caller-owned PostgreSQL READ COMMITTED transaction, matching the existing
authorization and Outbox adapters. No broker call or model call occurs in it.
All writers which change protected authorization facts must join the same lock
protocol; merely reading a version then writing later is insufficient.

1. Authenticate and bound-validate the request before acquiring locks. Derive
   expected tenant/actor/device from trusted context; compare rather than trust
   envelope identifiers. Reject client ServerCommit, invalid oneofs, unsupported
   profile/version/category, size violations and malformed required metadata.
2. Lock current organization → member → session → device → conversation →
   membership → current ACL → group/epoch → conversation sequence head. This
   extends the current resolver's order; Session/Device/Group locks and DM support
   are unimplemented. Revoke/offboard/archive/ACL/epoch writers must acquire their
   overlapping locks in this order, with multi-ID resources sorted. Freeze that
   cross-writer schedule in a dedicated integration contract before coding.
3. Re-evaluate active identity and current publish authority while the locks are
   held. DM needs a dedicated evaluator. Agent-attributed sends additionally
   require current Task/Run/Grant/execution-owner/fencing checks; until that lock
   extension is defined, the first slice is human application messages only and
   rejects agent attribution rather than trusting it.
4. Observe the immutable command/event binding. If exact and currently authorized,
   return the original receipt only after ending the transaction successfully;
   do not alter delivery state. Conflicting bindings produce a rollback. Unique
   constraints also arbitrate same-key requests across different conversations.
5. For a new command, verify conversation/group/profile/retention binding,
   current Epoch, authorized sender-signature metadata, category/body consistency
   and content hash over opaque body. On `rekey_required`, reject APPLICATION
   with `ERROR_CODE_REKEY_REQUIRED`; only the authorized ordered membership/MLS
   path may progress. Core does not verify MLS internals or act as signature
   authority. Client cryptographic verification remains mandatory.
6. Increment a transactional sequence-head row and append the opaque event plus
   ServerCommit, command binding, immutable Domain Event and initial Outbox Entry
   atomically. A plain PostgreSQL sequence is insufficient for rollback-contiguous
   allocation because its increments are not rolled back (see [PostgreSQL sequence semantics](https://www.postgresql.org/docs/current/functions-sequence.html)). Schema must represent
   the Proto uint64 range or explicitly freeze a checked upper-bound policy;
   never wrap or truncate to signed int64. At exhaustion fail without writes.
7. Commit durably under the deployment's reviewed durability policy. Only a known
   successful commit permits a success response containing the actual persisted
   event_id/idempotency_key/ServerCommit. If commit outcome is unknown or the
   connection dies, no synthetic ACK: client keeps its pending identity and
   retries. Cancellation before commit rolls back; cancellation racing commit
   may leave a committed event and must not be represented as proven rollback.

On every intermediate error, roll back the whole transaction. Outbox's own
exact-match semantics do not substitute for message deduplication. Deadlock or
serialization retries, if enabled, restart the complete transaction with the
same envelope/key, bounded by caller deadline; never replay only the last write.

The tentative lock schedule is a design proposal, not a claim that calling
EvaluateCurrent now acquires all these locks. Membership removal must commit
`rekey_required` before a successor application event can be accepted; its epoch
transition contract is a separate prerequisite. Server hash-chain fields and
signed checkpoints require a canonical, versioned transcript and signing-key
contract; the schematic hash formula in Proto is not permission to invent one.

## Failure and concurrency fixture table

Counts below are per logical event; no case may leak a raw body/key/token or SQL.

| Scenario | Required observable outcome |
| --- | --- |
| Failure after event insertion but before Outbox insertion/commit | 0 committed event, 0 binding, 0 entry, sequence head unchanged; no ACK |
| Commit succeeds, ACK lost, retry 100 times | 1 event, 1 binding, 1 initial entry/destination, original seq/time; authorized retries deduplicated |
| Two simultaneous same-key identical requests | One creator, one exact observer, same receipt; no second logical event |
| Same key, changed body/metadata or different conversation | One winner; conflict for changed input, no overwrite |
| Same event ID under another key | Conflict, not a second event or an alias giving new authority |
| Two distinct sends to one conversation | Two unique ordered positions; client timestamps do not determine order |
| Same keys/IDs across tenants | Independent tenant identities; cross-tenant requests fail without reading another receipt |
| Revoke/member removal/archive wins locks first | New send denied, no event/entry/ACK; revoke triggers required rekey semantics |
| Send commits before conflicting revoke | That event remains committed; later send denied, no retroactive deletion promise |
| Old exact retry after Epoch advances | If currently authorized and receipt retained, return original receipt; no new old-Epoch write |
| Group is rekey_required, new APPLICATION | Stable rekey-required rejection; sender retains pending logical intent |
| NATS down after DB commit | Durable ACK can succeed; entry remains for later relay; Sync can recover from DB |
| Worker publish succeeds then crashes before DB acknowledgement | May redeliver; consumer dedup by tenant/event in its effect transaction |
| Unknown commit outcome | No fabricated success/failure certainty; same-key retry resolves durable state |
| Key/body/profile/signature metadata invalid | Stable existing error category, no sensitive detail and zero domain mutation |

## Client Pending, ACK and Cursor responsibility

`client-core` durably enqueues before UI/network send. No UI or Agent writes
SQLite directly. Correlate the ACK by tenant/conversation/event/key and the
pending request, not arrival order. With an N-1 unary response lacking the echoed
key, use that request's local key and verify event_id per MessageService. Merge
ACK and echoed event in one local transaction; ACK-first, event-first and duplicate
arrivals all produce one committed logical item. Local crash resumes the durable
pending queue. Keep sender-authored signed fields intact; ServerCommit is outside
signature/AAD and is not itself cryptographic proof of history.

A rekey-required error cannot cause an arbitrary ciphertext rewrite under an
already-used key. First resolve whether the original send committed; only a
proven uncommitted logical intent can follow the separately reviewed client
reseal/identity policy. This requires an explicit successor fixture.

SyncCursor advances only after the client's highest contiguous range has been
applied durably; applying events/materialization/cursor is atomic. Arrival of
sequence 9 after 7 leaves the cursor at 7 until 8 is repaired. ReadCursor is a
separate member-visible maximum. WSS notification is a hint, not a commit or
cursor authority. Persist no server-returned cursor that jumps an unapplied hole.

ListEvents respects its half-open bounds and direction, with a bounded page and
current read authorization. Sync batches stop at holes; malformed/past-retention
cursors receive the existing per-cursor rejection and N-1 fallback. RepairGap
reports unrecoverable retained-history holes instead of retrying forever. A
client marks such a range unavailable; restarting at a checkpoint requires the
verified checkpoint/recovery contract, not silent skipping. Thread-filtered
history is a filtered view, not a contiguous conversation sync feed: Proto's
thread filter and contiguous-page language need a clarifying contract before a
Thread-index implementation; the first slice uses unfiltered history.

Permission checks precede each read page/stream delivery. Ciphertext, MLS bytes,
recovery envelopes and unknown fields remain opaque and preserved, including the
field-50000 canary. Core never inspects MessagePayload, obtains Channel keys or
logs message/file content, Prompt, local paths or credential material.

## Bounded successor drafts

All rows are 0.5–2 day issues to refine after review. No new migration number or
public event name is reserved by this document. Human-message scope prevents
premature dependency on Agent/Task implementations.

| Draft / effort | Primary owner and path | Prerequisites / deliverable | Verification command and behavior |
| --- | --- | --- | --- |
| M1 Comparison + producer registration contract, 1d | Contracts: `docs/contracts/` | This draft + Outbox policy → exact immutable comparison fixtures, payload encoding/size, event type/version/aggregate/destination; include tombstone/dedup retention decisions | `git diff --check`; tables for altered metadata/unknown bytes/duplicate fields and unauthorized route choices |
| M2 Sequence/identity storage, 1–2d | Integration: `db/` | M1 + reviewed Session/Device/Epoch lock schedule → migration with event/command/head constraints; reserve IDs at claim | `make -C db migration-test migration-ledger-test`; real PostgreSQL rollback, simultaneous key collision, uint64 boundary; pinned Atlas required |
| M3 Registered message producer adapter, 1d | Core: `services/core/internal/outboxstore/` | M1 integrated → callable typed producer around private insert, no caller-selected routing/policy | `cd services && go test -race ./core/internal/outboxstore`; invalid descriptor rejected, exact duplicate immutable, integration DB proof separately required |
| M4 Human SendEvent command, 2d | Core: `services/core/internal/messagecommand/` | M2/M3 plus implemented trusted Session/Device/Epoch and reviewed authorization schedule → transaction flow | `cd services && go test -race ./core/internal/messagecommand`; real PostgreSQL failure injection and concurrency table; unsupported categories/DM fail closed until implemented |
| M5 Local pending/ACK contract, 1d | Client-core contract task: `docs/contracts/` | Message wire + this draft → local atomic merge/reseal/cancel/retry fixtures | `git diff --check`; ACK-first/event-first/N-1/unknown commit/rekey schedules; schema prerequisites assigned separately |
| M6 Local merge implementation, 1–2d | Client-core: `crates/client-core/` | M5 and separate migration integrated → one pending-to-committed atomic operation | `cargo test -p threadline-client-core --locked`; crash/reopen duplicate ACK, out-of-order events and key mismatch |
| M7 Unfiltered Sync query slice, 1–2d | Core: `services/core/internal/messagesync/` | M2 and trusted read authorization → bounded contiguous batch | `cd services && go test -race ./core/internal/messagesync`; gap, retention, revoke, mixed valid/invalid cursors and tenant isolation |
| M8 First synthetic message integration, 1–2d | Quality: `test/e2e/` | M4/M6/M7 and independently assembled services/client + admitted crypto → two-device scenario | New test runner exact command frozen at issue creation; two members send, offline restart, retry 100 times, no server plaintext; unavailable Runtime does not affect IM |

M8 is blocked until a runnable harness exists; this Draft does not invent a
passing command. Server registration/transport assembly, DM authorization,
checkpoint signing, Agent attribution and membership handshake are separate
successors. #148 still blocks permission-readiness claims until reviewed;
this does not invalidate database commit semantics or authorize skipping Worker
permission checks. No paid service or physical-device work is required by this
Draft.

## This task's verification

Run `git diff --check`, validate local Markdown targets, and run:

```sh
node proto/tools/verify-message-sync-contracts.mjs
node proto/tools/verify-contracts.mjs
```

These validate the existing contract fixtures, not the proposed implementation.
Review the fixture table against existing Proto before merging; any needed field
or canonical transcript changes belong to a distinct Contracts/Integration PR.

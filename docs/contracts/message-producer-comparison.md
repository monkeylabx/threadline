# Message send comparison and Outbox producer registration

Status: Draft for #201, 2026-09-30. Base
`0d0c77bfd25149916771034182bd53cfb6a57a20` (issue baseline `b045d402` plus
documentation/CI-only commits; no cited input changed). Documentation only: no
comparator, schema, producer, Proto, fixture or generated output is implemented
or changed. M0/G0 remains HOLD. Every "must" is a proposed contract for review;
accepting #201 does not approve decisions D1–D12 below.

## Inputs and current facts

- [#193 draft](message-commit-transaction.md) proposes key scopes and the
  transaction schedule, and requires a follow-up fixture to freeze parser edge
  cases, nested unknowns and duplicate singular fields. This is its M1 draft.
- The [Outbox contract](../architecture/transactional-outbox-contract.md) requires
  a registered Event Type/version/aggregate/destination/payload-ceiling descriptor
  before any production insert (§4.1, §13). The
  [policy](../architecture/transactional-outbox-policy.md) freezes one logical
  destination, `domain-events`, a 262,144-byte payload limit, and a trusted
  startup policy file; Domain Event and Entry rows are permanent (§6).
- [outboxstore](../../services/core/internal/outboxstore/store.go) has only a
  package-private `insert` and `eventDescriptor{eventType, schemaVersion,
  aggregateKind}`. The [query](../../db/queries/core/transactional_outbox.sql)
  hard-codes `domain-events`. An existing `(tenant_id, event_id)` is
  `already-present` only if type, version, aggregate, payload, `occurred_at` and
  the single destination match; otherwise `idempotency-conflict`. Identifiers are
  non-empty, trimmed and control-free but unbounded. No descriptor is registered.
- Wire authority: [ChannelEventEnvelope](../../proto/threadline/message/v1/envelope.proto),
  [MessageService](../../proto/threadline/message/v1/message_service.proto),
  realtime `SendEvent`/`DurableAck` in [frame.proto](../../proto/threadline/realtime/v1/frame.proto),
  [ErrorCode](../../proto/threadline/type/v1/error.proto) and the
  [persisted-message rules](compatibility.md).
- [#192 draft](session-device-verification.md) supplies the Principal
  `{TenantID, ActorType, ActorID, DeviceID, SessionID}`, fail-closed device
  binding until successor S1/S4, and the draft lock order.
- The T015 [behavior verifier](../../proto/tools/verify-message-sync-contracts.mjs)
  models conflict by `contentHash` alone and checks Epoch/rekey before the prior
  commit. It predates #193 and is not this comparator (D10).

No identifier length bound, envelope byte ceiling or SendEvent codec restriction
exists in Proto or code at the base commit.

First slice: Human `EVENT_CATEGORY_APPLICATION` sends into a Channel through
unary `MessageService.SendEvent`. WSS terminates in `services/realtime`, so
`ClientFrame.send_event` joins only after successor RT1 forwards `E` byte-exactly
to Core; until then Realtime rejects it. DM, other categories, attachments,
redaction and Agent attribution fail closed until their own contracts land.

## 1. Exact comparison

### 1.1 Compared value

`E` is the exact byte string carried as the length-delimited value of field 1 of
`SendEventRequest` (unary) or, after RT1, realtime `SendEvent` (WSS): the encoded
`ChannelEventEnvelope` as received, before any decode or re-encode. Core must
capture `E` from the binary transport; re-marshalling a decoded message, in Core
or in a forwarding Realtime process, is not `E`.

1. SendEvent is binary Protobuf only (Connect `application/proto`, gRPC, WSS
   binary frame). JSON or other transcoding cannot preserve unknown fields or
   byte identity, so no JSON codec is registered for SendEvent. Such a request is
   refused before Core and creates nothing (D2).
2. The wrapper contains exactly one field-1 record, and a WSS `ClientFrame`
   exactly one field-3 `send_event` record. Unknown wrapper fields are neither
   persisted nor compared and cannot influence routing.
3. Two submissions under one command key are **identical** if and only if their
   `E` values are byte-for-byte equal. Core does not canonicalize, sort,
   re-encode or compare decoded values. Byte equality implies equal typed values
   under any conformant parser; the converse does not hold, so a semantically
   equal but differently encoded retry is **conflicting**. This refines #193's
   typed known-field comparison (D1).
4. Client-core persists `E` at durable enqueue and retransmits exactly `E` on
   every retry whose commit status is unknown, across restart and upgrade. A
   different `E` under a key with any committed binding conflicts. Resealing a
   proven-uncommitted intent stays with #193's M5 identity policy.

### 1.2 Stored comparison digest

The command binding retains `envelope_length = len(E)` and:

```text
envelope_digest = SHA-256(ASCII "threadline.message.send-comparison/v1\0"
                          || uint32-BE(len(E)) || E)
```

Identical means equal length and all 32 digest bytes equal. The digest outlives
ciphertext retention (Section 5), so classification never requires keeping `E`.
It fingerprints ciphertext and C2 metadata. It is not a signature, AAD, content
hash or checkpoint input and replaces none of them.

Synthetic Golden vectors. A, B and C decode to equal typed values
(`event_id="e"`, `tenant_id="t"`, `channel_id="c"`, `epoch=0`), yet every pair is
conflicting:

| Case | `E` hex | SHA-256 |
| --- | --- | --- |
| A: fields 1, 2, 3 | `0a01651201741a0163` | `bec8e07de54f08e0a74e012968c7a79fe2e1925414771ffdb6f353bb9693eeb8` |
| B: fields 2, 1, 3 | `1201740a01651a0163` | `af172d48bab1323987382c9a93d84f1b422ca2e238d96522e99bc18e18689ce4` |
| C: A plus explicit `epoch` `3000` | `0a01651201741a01633000` | `246055419da7681d57a55fa13fc228534612a9a92facdedea3089da8a3b249af` |

Preimage A is
`7468726561646c696e652e6d6573736167652e73656e642d636f6d70617269736f6e2f763100000000090a01651201741a0163`.
These are digest vectors only; as SendEvent envelopes they would fail Section 1.3.

### 1.3 Pre-lookup checks

These run before any database read and are pure functions of `E` and the
Principal, so an identical retry of a committed `E` always passes them. Failure
means no lookup and no write.

- `len(E)` is `1..262,144` bytes (proposed `MAX_ENVELOPE_BYTES`, D3).
- `E` decodes under proto3 rules, including UTF-8 validation of strings.
- Strict wire profile, applied to `E` and recursively to every known
  message-typed field inside it (`client_sent_at`, `recovery_envelope` and all of
  its known nested messages): no known non-repeated field occurs twice, no oneof
  has two members on the wire, and no known field number uses a wire type other
  than its declared one. Parsers differ on these (last-wins, merge, or keeping a
  wrong-type field as unknown); rejecting them removes parser-differential
  ambiguity between Core, Recovery Control and receiving clients (D8). The bytes
  of unknown fields are not scanned.
- Field 7 `server_commit` does not occur at all, not even as a zero-length record.
- `event_id`, `tenant_id`, `channel_id`, `e2ee_group_id`, `sender_device_id`,
  `sender_actor_id`, `idempotency_key` and `crypto_profile` are 1..128 UTF-8
  bytes, trimmed, without control characters (proposed, D3).
- `tenant_id`, `sender_device_id` and `sender_actor_id` equal the Principal's
  tenant, device and actor, and the Principal's actor type is Human (membership
  is checked in step 3). A present `recovery_envelope` names the same tenant,
  `e2ee_group_id` and `epoch`.
- First-slice gates: category is APPLICATION; conversation is `channel_id`; body
  is non-empty `application_ciphertext`; `mls_message`, `agent_attribution`,
  `redaction_target_event_id` and `attachment_blob_ids` are absent or empty.

Unknown fields, including field `50000` canaries, reserved numbers 13 and 19, and
fields from a newer client, are accepted, preserved inside `E` and covered only by
byte comparison. An unknown `category` value fails the category gate before
lookup. Repeated fields compare in wire order; the envelope has no
packed repeated scalar. Non-minimal varints that decode are accepted; a retry
must repeat the same bytes.

### 1.4 Precedence

1. Transport and authentication (interceptor categories from #192).
2. Pre-lookup checks (1.3).
3. Lock and re-evaluate identity and `channel.publish` authority (#193 steps 2-3).
4. Look up the binding for `(Principal.TenantID, Principal.DeviceID,
   idempotency_key)`: live and identical returns the original receipt; tombstoned
   and identical returns the retention outcome; any conflicting `E` returns
   conflict; absent continues.
5. New-write checks (#193 step 5): conversation/Group binding, current Epoch and
   `rekey_required`, supported Crypto Profile and envelope version, content hash,
   signature metadata, recovery policy. Then event-ID uniqueness, sequence
   allocation and the writes.

Step 5 never runs for an identical retry, so a later Epoch, rekey state or
Profile-support change cannot strand a committed send. A conflicting retry is
reported as conflict without evaluating step 5. #193 step 1 rejected unsupported
Profile/version before locking; moving that check to step 5 is D11.

## 2. Uniqueness and duplicate outcomes

Proposed storage scopes, carried from #193; M2 freezes names and types:

| Fact | Unique scope |
| --- | --- |
| Send command binding | `(tenant, sender_device, idempotency_key)` from the Principal |
| Message event | `(tenant, event_id)`, enforced against bindings and purge reservations (Section 5), not only retained events |
| Conversation position | `(tenant, conversation_kind, conversation_id, channel_seq)` |
| Domain Event | `(tenant, domain_event_id)`, Core-generated (Section 3) |

"None" means no sequence increment, message event, binding change, Domain Event
or Outbox Entry, here and in Section 8.

| Situation | Response | Writes |
| --- | --- | --- |
| Identical, live | Success with the stored `event_id`, `idempotency_key` and ServerCommit exactly; `deduplicated=true` (unary only; WSS `DurableAck` has no such field) | None; the read-only transaction ends successfully first |
| Same key, conflicting `E` | `IDEMPOTENCY_CONFLICT`, reason `idempotency_key_conflict` | None; roll back |
| `event_id` already bound to another key or device | `IDEMPOTENCY_CONFLICT`, reason `event_id_conflict` | None; roll back |
| New | Success, `deduplicated=false` | Exactly one sequence increment, message event, binding, Domain Event and initial Entry |

Errors never carry the original receipt, sender, conversation or another
command's `event_id`; `subject_id` stays empty. A device resolves its own conflict
through the authorized read path (`GetEvent` by its `event_id`). Concurrent
same-key requests for one Channel serialize on the sequence-head lock (#193 step
2), so the second one's lookup sees the committed binding. Across different
Channels they meet at the binding's unique key. Because a unique violation aborts
a PostgreSQL transaction, the insert uses `ON CONFLICT DO NOTHING` plus a re-read,
as outboxstore does; the loser waits, then classifies after the winner commits,
or continues as creator after it rolls back.

## 3. Producer descriptor

The registry entry is a compiled constant in the Core producer adapter (M3). No
runtime registration, configuration key, request field, header or payload byte
can add, select or alter it.

| Field | `message.committed` v1 | Validation point |
| --- | --- | --- |
| Event Type / schema version | `message.committed` / `1` | M3 constant; DB canonical-shape and positive-version checks; consumers reject unknown pairs |
| Aggregate kind | `channel`; `dm` only after the DM evaluator | Derived from the kind of the conversation row locked in #193 step 2 |
| Aggregate ID | That row's `channel_id` in the Principal's tenant | Never copied from `E` without the tenant-scoped lookup |
| Domain Event ID | Core-generated, at least 128 CSPRNG bits, canonical lowercase text (D5) | M3; DB primary key |
| Destinations | Exactly `domain-events` | M3 constant; DB check; Worker alone maps it to a subject |
| Payload | Layout v1 below, at most 141 bytes; descriptor ceiling 256 bytes | M3 preflight; DB check at 262,144 |
| `occurred_at` | ServerCommit `committed_at`: the creating transaction's PostgreSQL timestamp | M3 |
| Policy | Trusted `threadline.outbox.policy/v1` snapshot from `THREADLINE_OUTBOX_POLICY_FILE` | Integration parses, Platform owns values (policy §7); invalid means producer not ready |
| Owners | Contracts (registry), Core (constant and encoder), Realtime-worker (consumer decode) | Changes require a Contracts review |

The Domain Event ID is server-generated because Outbox identity is tenant-wide
across all producers: a client-chosen ID could pre-empt another producer's future
ID and turn its insert into a conflict. Message deduplication is the binding, not
the Outbox exact-match path; an Outbox `(tenant, event_id)` collision for this
producer is `persistence-failure`.

Payload layout v1, integers big-endian:

```text
uint8   conversation_kind      0x01 channel (0x02 dm reserved, not emitted)
uint32  conversation_id_length 1..128
bytes   conversation_id        UTF-8
uint64  channel_seq            >= 1
```

Golden vector: channel `c`, sequence 1 is `0100000001630000000000000001`
(14 bytes). Decoders reject another schema version, another kind, an out-of-range
length, trailing bytes and sequence 0. A new layout is a new schema version,
deployed consumer-first. The payload deliberately excludes ciphertext, content
hash, signature, recovery material, attachment IDs, idempotency key, message
`event_id`, sender and client time: Domain Event rows are permanent and
undeletable, so any content copy would survive retention and redaction. Consumers
treat it as a wake-up pointer and read authorized state through Sync/Message (D4).

The typed adapter accepts only the transaction, the Principal's tenant, the locked
conversation reference, the allocated `channel_seq` and the database commit
timestamp. It accepts no Event Type, version, destination, stream, subject,
policy value, payload bytes, aggregate-kind string or Domain Event ID.

## 4. Durable ACK versus NATS publication

- Success is returned only after a known PostgreSQL commit containing the
  sequence increment, message event, binding, Domain Event and initial Entry.
- No broker call happens in the transaction. NATS outage, an open breaker or
  unready Worker publish permission ([#148](https://github.com/monkeylabx/threadline/issues/148))
  delays fan-out only; Sync recovers from PostgreSQL.
- A JetStream PubAck is neither a client ACK nor consumer application; realtime
  `EventDelivery` and `SyncHint` are hints.
- Unmet durability or an unknown commit outcome returns `NOT_DURABLE`, never
  synthetic success; the client retries the same `E`.
- Producer readiness requires a valid policy file, the compiled descriptor and
  schema checks (policy §7). If not ready, no transaction starts.
- An identical retry reads the receipt only; it never inspects or changes Entry
  delivery state.

## 5. Binding retention and tombstones

- **Live**: the message event is retained; identical returns the original receipt.
- **Tombstoned**: when retention or redaction removes the event's ciphertext or
  row, the same transaction marks the binding tombstoned. It keeps only key,
  `event_id`, conversation reference, `envelope_length`, `envelope_digest` and
  tombstone time. It never returns to live and never frees its key or `event_id`.
- **Purged**: allowed only when the sender Device is terminal (REVOKED or
  REJECTED), because no authenticated retry can follow, or after a trusted horizon
  `H` from creation (`H` unresolved, D6). Purge drops the key, length and digest
  but keeps a permanent `(tenant, event_id)` reservation, so `event_id` stays
  tenant-unique as Proto requires. A later retry of `E0` finds no key and runs
  step 5: it fails as `event_id_conflict`, or earlier on an Epoch/rekey check,
  and never creates a second event. It no longer receives `R0`, so client-core
  stops automatic retry before `H` and resolves by an authorized read of
  `event_id`, or by a user-confirmed resend as a new logical message.

Tombstone and purge never touch Domain Event or Entry rows. Worker attempt
evidence TTL is unrelated to binding lifetime.

## 6. Unimplemented dependencies

| Dependency | Needed for | Owner |
| --- | --- | --- |
| Transport device proof and Session verifier (#192 S1/S4) | Trusted `Principal.DeviceID` | Contracts, then Core |
| Session/Device lock contract, schema and revocation writers (#192 S2/S3) | Step 3 linearization | Contracts, then Integration |
| Group/Epoch and `rekey_required` storage with its lock | Step 5 and Epoch lock | Contracts (epoch transition), then Core/Integration |
| Canonical transcript and checkpoint signing contract (#193) | ServerCommit `chain_hash` fields; absent from receipts until then | Contracts, Crypto-recovery |
| DM permission evaluator; the [current resolver](../../services/core/internal/authorization/current/current.go) supports Space/Channel only | Aggregate kind `dm` | Core |
| Sequence head, message event and binding storage (M2) | Every write | Integration |
| Registered producer adapter (M3) | Domain Event insert | Core |
| Retention/redaction tombstone transition | Section 5 | Retention |

None exists at the base commit, and none is assumed from its Proto.

## 7. #192 identity boundary

The key uses the Principal's tenant and device, not the session: re-login on the
same Device retries the same `E` successfully, while another Device cannot reach
the binding. Envelope identities are compared, never trusted. Until S1/S4 land,
SendEvent is fail-closed. Authorization precedes lookup, and an auth failure
causes no domain write, Outbox insert or ACK. If revoke commits first, both new
sends and identical retries are denied; if the send commits first, the event
stays. Agent and Service actors cannot use this path. No ErrorCode or interceptor
category changes.

## 8. Acceptance matrix

`E0` is committed and live under key `K=(T, D, k)` with event `e0` in Channel `c0`
and receipt `R0`. Rows state the second input and the writes it causes.

| ID | Second input | Expected outcome | Writes |
| --- | --- | --- | --- |
| X01 | `E1 = E0` from `T/D`, still authorized | `R0`, `deduplicated=true` | None |
| X02 | 100 sequential identical retries | Each as X01; totals stay one event, binding and Entry | None |
| X03 | No prior; two concurrent identical `E0` | One creator (`false`), one observer (`true`), same `R0` | One of each in total |
| X04 | One `application_ciphertext` byte changed, hash and signature recomputed | `idempotency_key_conflict` | None |
| X05 | Only `content_hash` changed | Conflict, not `CIPHERTEXT_CORRUPT`; step 5 never runs | None |
| X06 | Only `client_sent_at` nanos, `sender_signature` or recovery `wrapped_material` changed; or `epoch`/`e2ee_group_id` changed together with the recovery envelope's copy | Conflict | None |
| X06b | `epoch` or `e2ee_group_id` changed in the envelope only | `recovery_binding_mismatch` before lookup | None |
| X07 | Same `k`, Channel `c1` | May publish in `c1`: conflict; may not: authorization denial, binding unread | None |
| X08 | `E0` omits `epoch` (Group at Epoch 0); `E1 = E0` plus `3000` (explicit default, as vectors A/C) | Conflict | None |
| X09 | `E0` omits field 12; `E1 = E0` plus empty `client_sent_at` `6200` (absent vs present-empty) | Conflict | None |
| X09b | `E1 = E0` plus empty `recovery_envelope` `a20100` | `envelope_tenant_mismatch` before lookup; `envelope_duplicate_field` if `E0` already had one | None |
| X10 | Field `50000` canary removed, changed or moved before field 1 | Conflict; byte-identical canary is X01 | None |
| X11 | Nested recovery canary removed | Conflict | None |
| X12 | `E1` re-encodes `E0` with fields 1 and 2 swapped (as vectors A/B) | Conflict | None |
| X13 | First submission repeats a singular field (even equal), sets two oneof members, repeats `recovery_envelope`, or sends `epoch` length-delimited (`32…`) | `envelope_duplicate_field` or `envelope_wire_type_invalid` before lookup; no binding | None |
| X14 | `E0` plus a repeated `event_id` record | Same structural rejection; binding untouched; not conflict | None |
| X15 | Wrapper carries field 1 twice, or (after RT1) a `ClientFrame` carries field 3 twice | `envelope_duplicate_field` | None |
| X16 | `E0` used non-minimal `308000`; retry uses `3000` | Conflict | None |
| X17 | Any `attachment_blob_ids` | `attachments_not_enabled`; once enabled, `[a,b]` then `[b,a]` conflicts | None |
| X18 | `len(E)` 262,144 / 262,145; identifier 128 / 129 bytes | Passes / `PAYLOAD_TOO_LARGE`; passes / `envelope_identifier_invalid` | None on rejection |
| X19 | `E0` plus any field 7 record, including `3a00` | `server_commit_supplied`; never deduplicated | None |
| X20 | Sequence head `B-1` / `B` (uint64 maximum or M2 checked bound, D9) | Success with `B` / `channel_seq_exhausted` | One of each / none |
| X21 | New command with `epoch = 2^64-1`; separately X01 after the Epoch advanced or `rekey_required` | Compared without conversion: `EPOCH_STALE`; the retry still returns `R0` | None |
| X22 | Envelope tenant `T2`, Principal `T` | `TENANT_MISMATCH` before lookup; nothing read in `T2` | None |
| X23 | Same `k` and `e0` from `D'` in `T2` | Independent commit; nothing visible across tenants | One of each in `T2` |
| X24 | Channel ID existing only in `T2` | `publish_unavailable`, same as a denied `T` Channel; no cross-tenant query (D12) | None |
| X25 | Another Device in `T`, new key, `event_id = e0`, allowed Channel | `event_id_conflict`; no `R0`, `D` or `c0` disclosed. Residual: confirms `e0` exists somewhere in `T`, bounded by random IDs (D3) | None |
| X26 | `D`, new key, `event_id = e0` | `event_id_conflict` | None |
| X27 | Unknown field or request headers naming a stream, subject, destination or tenant | Ignored; Domain Event carries registry values and trusted policy | As X01 or new |
| X28 | `dm_id`, non-APPLICATION category, Agent attribution or redaction target | First-slice gate rejection | None |
| X29 | Device or session revoked, then `E0` | Unauthenticated; no receipt | None |
| X30 | Publish authority removed, then `E0` | `publish_unavailable` before lookup; client may resolve via `GetEvent(e0)` if still readable | None |
| X31 | Binding tombstoned; `E1 = E0` / `E1 != E0` | `send_receipt_unavailable` / conflict | None |
| X32 | Binding purged (terminal Device, or after `H`), then `E0` | Terminal Device: X29. Otherwise step 5 fails (`event_id_conflict` via the reservation, or an Epoch/rekey code); never a second event | None |
| X33 | Commit outcome of `E0` unknown, then retry | X01 or a single creation; never two events | At most one |
| X34 | NATS and Worker down throughout | Success; Entry stays `pending`; no PubAck needed | One of each |
| X35 | Invalid policy file or failed schema/extension readiness check | `message_producer_not_ready`; no transaction | None |
| X36 | JSON-coded SendEvent | Refused by transport; Core not invoked | None |

## 9. Error mapping

Transport statuses and reasons are proposed; codes reuse the existing enum (D7).
New-write failures keep the #193/T015 codes: `GROUP_MISMATCH`,
`REKEY_REQUIRED`, `EPOCH_STALE`, `CRYPTO_PROFILE_UNSUPPORTED`,
`ENVELOPE_VERSION_UNSUPPORTED` and `CIPHERTEXT_CORRUPT`. The transport column is
unary; after RT1, WSS sends the same `ErrorDetail` in `StreamError` with
`fatal=false`.

| Condition | Transport | ErrorCode / reason | Retry same `E` |
| --- | --- | --- | --- |
| Session missing, invalid or revoked | `unauthenticated` | Interceptor category (#192) | After reauthentication |
| Envelope too large | `invalid_argument` | `PAYLOAD_TOO_LARGE` / `envelope_too_large` | No |
| Decode failure, duplicate field, wrong wire type, field 7, invalid identifier | `invalid_argument` | `CIPHERTEXT_CORRUPT` / `envelope_decode_failed`, `envelope_duplicate_field`, `envelope_wire_type_invalid`, `server_commit_supplied`, `envelope_identifier_invalid` | No |
| Envelope or recovery tenant differs | `permission_denied` | `TENANT_MISMATCH` / `envelope_tenant_mismatch` | No |
| Recovery Group or Epoch differs from the envelope | `invalid_argument` | `CIPHERTEXT_CORRUPT` / `recovery_binding_mismatch` | No |
| Device, actor or actor type differs | `permission_denied` | `PERMISSION_DENIED` / `sender_mismatch` | No |
| First-slice gate | `failed_precondition` | `PERMISSION_DENIED` / `dm_send_not_enabled`, `category_not_enabled`, `attachments_not_enabled`, `redaction_not_enabled`, `agent_attribution_not_enabled` | No |
| Not authorized, not a member, or Channel absent in tenant | `permission_denied` | `PERMISSION_DENIED` / `publish_unavailable`; never `NOT_A_MEMBER` here (D12) | No |
| Key or event-ID conflict | `already_exists` | `IDEMPOTENCY_CONFLICT` / `idempotency_key_conflict`, `event_id_conflict` | No |
| Tombstoned identical | `failed_precondition` | `RETENTION_EXPIRED` / `send_receipt_unavailable` | No; terminal |
| Sequence exhausted | `resource_exhausted` | No existing code (D7) / `channel_seq_exhausted` | No |
| Producer not ready | `unavailable` | `NOT_DURABLE` / `message_producer_not_ready` | Yes |
| Durability unmet or commit outcome unknown | `unavailable` | `NOT_DURABLE` / `commit_not_durable` | Yes; may already be committed |
| Cancellation or deadline | `canceled` / `deadline_exceeded` | None | Yes; may already be committed if it raced commit |

Only the last two rows may leave a committed event (#193); every other row
creates nothing.

## 10. Decisions requiring review

| ID | Proposed decision | Owner |
| --- | --- | --- |
| D1 | Exact received-bytes equality replaces #193's typed known-field comparison; clients retransmit durable `E` | Contracts, Client-core |
| D2 | SendEvent is binary-Protobuf-only on every transport, including web; WSS send is rejected until RT1 forwards `E` byte-exactly | Contracts, Desktop/Web, Realtime-worker |
| D3 | `MAX_ENVELOPE_BYTES = 262,144`; identifiers 1..128 bytes, also enforced when Tenant/Channel/Device IDs are created (existing columns are unbounded); client `event_id` and key carry at least 122 CSPRNG bits (UUIDv4) | Contracts, Core, Client-core, Product |
| D4 | Fixed binary pointer payload instead of a persisted Protobuf message (which would need `reserved 50000`, fixtures and N-1 evidence) | Contracts, Realtime-worker |
| D5 | Core-generated Domain Event ID, not the message `event_id` | Contracts, Core |
| D6 | Tombstone keeps the digest; purge only for terminal Devices or after horizon `H`, keeping a permanent `event_id` reservation; value of `H` | Retention, Security, Client-core |
| D7 | Reuse `CIPHERTEXT_CORRUPT` for envelope shape and recovery binding, `PERMISSION_DENIED` for gates. `NOT_DURABLE` for producer-not-ready and `RETENTION_EXPIRED` for redaction tombstones stretch documented meanings, which v1 forbids; decide new codes for those and for exhaustion | Contracts |
| D8 | Strict duplicate-field and wire-type rejection via a recursive wire scanner | Core, Security |
| D9 | Full uint64 sequence storage or a checked lower bound (inherited from #193) | Integration |
| D10 | Align the T015 fixture: full-envelope comparison and dedup before Epoch checks | Contracts |
| D11 | Profile/version support moves from #193 step 1 (before locks) to step 5, so identical retries survive support-window changes | Contracts, Crypto-recovery |
| D12 | Denied and absent Channels share `PERMISSION_DENIED` / `publish_unavailable`. `TENANT_MISMATCH` is only for a request naming another tenant, never learned by a cross-tenant lookup; this interprets the `error.proto` comment | Contracts, Security |

## 11. Successor tasks

Each is a separate 0.5-2 day issue after review. No migration number or public
event name beyond `message.committed` v1 is reserved here. Every command below is
NOT RUN: the code and cases do not exist at this base.

| Task | Owner and path | Deliverable | Verification entry point |
| --- | --- | --- | --- |
| F1 Comparator fixtures, 1d | Contracts: `test/fixtures/proto/message/` (the T015 manifest names Contracts as owner inside Quality's `test/` tree), `proto/tools/verify-message-sync-contracts.mjs` | Section 1.2 vectors, Section 8 classification and precedence; resolves D10 | `node proto/tools/verify-message-sync-contracts.mjs` |
| M2 Storage (#193), 1-2d | Integration: `db/` (integration-owned migrations per AGENTS.md; Core Migration Owner reserves the ID) | Adds binding digest, length, live/tombstoned state, purge reservations, event-ID uniqueness and D9 | `make -C db migration-test migration-ledger-test` |
| M3 Producer adapter, 1d | Core: `services/core/internal/outboxstore/` | Descriptor constant, payload encoder with vector, Domain Event ID generator, typed facts API | `cd services && go test -race ./core/internal/outboxstore` |
| M4 Human SendEvent (#193), 2d | Core: `services/core/internal/messagecommand/` | `E` capture, wire scanner, Section 1.4 precedence after Section 6 dependencies | `cd services && go test -race ./core/internal/messagecommand` with real PostgreSQL |
| C1 `message.committed` consumer, 1d | Realtime-worker: `services/realtime/` | Strict v1 decode, unknown-version rejection, dedup by `(tenant, domain_event_id)` | `cd services && go test -race ./realtime/...` |
| RT1 WSS send forwarding, 1d | Realtime-worker: `services/realtime/` | Forward `E` to Core as exact bytes without decode/re-marshal; `ClientFrame` field-3 duplicate check; `DurableAck`/`StreamError` mapping; reject WSS send until done | `cd services && go test -race ./realtime/...`; byte-identity of forwarded `E` for vectors A/B/C and a canary frame |
| R1 Binding retention, 0.5d | Architecture: `docs/architecture/` | Resolves D6 | `git diff --check` |

M5 from #193 additionally inherits: persist and retransmit exact `E`, and stop
automatic retry before `H`.

## This task's verification

Run `git diff --check`, resolve every local Markdown link in this file, and run:

```sh
node proto/tools/verify-message-sync-contracts.mjs
node proto/tools/verify-contracts.mjs
```

They validate existing fixtures and prove no wire or generated surface changed;
they do not test this proposal. The Section 1.2 and Section 3 vectors were
recomputed independently in Node and Python.

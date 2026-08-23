# Transactional Outbox Deployment Policy

Status: frozen by P03-08P
Policy identifier: `threadline.outbox.policy/v1`

This document supplies the trusted deployment inputs required by the
[Transactional Outbox Storage And Relay Contract](transactional-outbox-contract.md).
It does not change that contract's Event, Entry, Attempt, replay, or message-ID
semantics. Each statement below is explicitly an **External fact** or a
**Threadline decision**. External defaults are observations, not Threadline
defaults.

## 1. Primary-source findings

### PostgreSQL 16 and `pgcrypto`

- **External fact:** PostgreSQL permits a field of up to 1 GB, while warning
  that practical limits can be reached earlier. This is a storage-engine bound,
  not an appropriate event-payload policy. [PostgreSQL 16 limits](https://www.postgresql.org/docs/16/limits.html)
- **External fact:** `octet_length(bytea)` reports the number of bytes in a
  binary string, so a payload limit can be enforced without interpreting the
  opaque payload. [PostgreSQL 16 binary-string functions](https://www.postgresql.org/docs/16/functions-binarystring.html)
- **External fact:** `pgcrypto.gen_random_bytes(count)` returns
  cryptographically strong random bytes and permits at most 1,024 bytes per
  call. `pgcrypto.digest(bytea, 'sha256')` returns a binary SHA-256 digest.
  `pgcrypto` is a trusted extension but requires an OpenSSL-enabled PostgreSQL
  build. [PostgreSQL 16 `pgcrypto`](https://www.postgresql.org/docs/16/pgcrypto.html)
- **External fact:** `pgcrypto` runs in the database server; its documentation
  requires a local or SSL connection and trusted system/database
  administrators, and says its implementation does not resist side channels.
  It must not be treated as a constant-time token verifier.
  [PostgreSQL 16 `pgcrypto` security limitations](https://www.postgresql.org/docs/16/pgcrypto.html#PGCRYPTO-NOTES)

### NATS and JetStream

- **External fact:** NATS server `max_payload` defaults to 1 MB, may be set up
  to 64 MB, should not exceed 8 MB, applies to client and leaf-node payloads,
  and must not exceed `max_pending`. The server value is independently
  configurable and reloadable. [NATS server configuration reference](https://docs.nats.io/reference/config/)
- **External fact:** JetStream Stream configuration has a separate
  `max_msg_size` setting for the maximum size of one stored message. A Stream
  also has a `duplicate_window`; when omitted, the server default is two
  minutes. [Official NATS Go JetStream API: `StreamConfig`](https://pkg.go.dev/github.com/nats-io/nats.go/jetstream#StreamConfig)
- **External fact:** JetStream deduplication is activated by a caller-supplied
  message ID (`Nats-Msg-Id` / `WithMsgID`), is bounded by the Stream duplicate
  window, and a successful publish acknowledgement reports whether the message
  was recognized as a duplicate. [Official NATS Go JetStream API: publish options and `PubAck`](https://pkg.go.dev/github.com/nats-io/nats.go/jetstream#PubAck)
- **External fact:** the NATS server may impose a maximum permitted Stream
  duplicate window; the default for a new Stream is 120 seconds when no server
  limit overrides it. [Official NATS server configuration: JetStream limits](https://github.com/nats-io/nats.docs/blob/master/running-a-nats-service/configuration/README.md#jetstream-server-settings)

**Threadline decision:** JetStream deduplication is defense in depth only.
Threadline's database idempotency and the stable message ID remain mandatory
before, during, and after the duplicate window. A duplicate PubAck is successful
publish evidence only after the contract's exact stream and message-ID checks.

## 2. Frozen v1 values

Unless a row says otherwise, lower and upper bounds are inclusive. Bytes mean
octets, durations mean elapsed milliseconds or seconds as stated, and counts
are positive base-10 integers. A request, payload, event descriptor, broker
header, or Worker call cannot supply or widen any value in this table.

| Input | v1 default | v1 accepted deployment range / hard rule | Enforcement owner |
| --- | ---: | --- | --- |
| Opaque stored Event payload | 262,144 bytes | Exact v1 schema constant; payload values are `0..262,144` bytes | P03-08B PostgreSQL `CHECK (octet_length(payload) <= 262144)`; producer preflight repeats it |
| Final NATS wire message | 327,680 bytes | Exact v1 protocol constant; message values are `0..327,680` bytes, including encoded envelope and headers | P03-08C Worker before publish plus broker startup validation |
| Claim batch size | 64 Entries | Runtime: `1..256` Entries per claim call | Integration config; P03-08B query rejects out-of-range input |
| Initial claim lease | 30 seconds | Runtime: `5..120` seconds | Integration config; P03-08B database time and query bounds |
| Absolute claim lifetime | 300 seconds | Runtime: `30..900` seconds and at least twice the configured lease | Integration config; P03-08B persists an immutable absolute deadline |
| Event-specific retry ceiling | 8 failures | Runtime: `1..20` failures per Entry generation; the ceiling-th failure parks | Integration policy snapshot; P03-08B constraint/query |
| Transport-unavailable backoff | 1,000 ms base; 60,000 ms cap | Base `100..10,000` ms; cap `1,000..300,000` ms; cap >= base | Integration policy snapshot; database schedules from database time |
| Publish-outcome-unknown backoff | 5,000 ms base; 300,000 ms cap | Base `500..30,000` ms; cap `5,000..900,000` ms; cap >= base | Integration policy snapshot; database schedules from database time |
| Event-retryable backoff | 5,000 ms base; 300,000 ms cap | Base `500..30,000` ms; cap `5,000..900,000` ms; cap >= base | Integration policy snapshot; database schedules from database time |
| Claim-token entropy | 32 random bytes (256 bits) | Schema/protocol hard rule: exactly 32 raw bytes | P03-08B database generates; Security owns the profile |
| Stored token digest | SHA-256, 32 bytes | Schema/protocol hard rule below | P03-08B stores it privately; reviewed database functions compare candidates |
| Terminal Attempt evidence | 90 days after `finished_at` | Trusted Retention setting: `30..365` days | A later Retention task; purge remains disabled in P03-08B |
| JetStream duplicate window | 120 seconds | Deployment must equal 120 seconds for the mapped Stream | Platform config; P03-08C startup inspection |

**Threadline decision:** the 256 KiB stored-payload ceiling is intentionally
far below PostgreSQL's engine limit and the default NATS limit. The 320 KiB
wire ceiling reserves exactly 65,536 bytes above the maximum stored payload for
the versioned envelope and NATS headers. P03-08C measures the complete encoded
message; it must not assume the reserve makes an oversized message valid.

**Threadline decision:** `lease_expires_at` is initialized to
`claimed_at + configured_lease`. `absolute_lease_expires_at` is initialized to
`claimed_at + configured_absolute_lifetime`. Renewal sets
`lease_expires_at = min(database_now + configured_lease,
absolute_lease_expires_at)` and must strictly increase it. Once the absolute
deadline is reached, renewal is denied. Lease expiry creates an immediate
replacement attempt with no backoff and consumes no event-failure budget.

**Threadline decision:** `transport-unavailable` and
`publish-outcome-unknown` never consume the event-specific ceiling and may
retry indefinitely. Only `event-retryable` increments the generation failure
count. Failure numbers 1 through 7 schedule another attempt under the default
ceiling of 8; failure number 8 parks. `event-permanent` parks immediately.

## 3. Exact backoff and jitter

**Threadline decision:** for a scheduled failure with one-based scheduling
ordinal `n`, calculate in unsigned 64-bit integer milliseconds:

```text
require  = 1 <= n <= MaxInt64
shift    = min(n - 1, 20)
scaled   = base_ms * (1 << shift)       // checked multiply
window   = min(scaled, cap_ms)
jitter64 = uint64-BE(first 8 bytes of jitter_digest)
delay_ms = jitter64 mod (window + 1)    // full jitter, closed [0, window]
```

Configuration validation proves `base_ms <= cap_ms <= 900000`, so the checked
multiply cannot overflow after the shift clamp. Implementations must still use
checked/saturating arithmetic and return the contract's `invalid-input`
category rather than wrap.
`next_attempt_at` is `database_now + delay_ms milliseconds`.

All Entry/Attempt counters are non-negative PostgreSQL `bigint` values and Go
`int64` values. They never exceed `MaxInt64`. Increment at `MaxInt64` atomically
returns `persistence-failure` with no state change; no counter wraps, parks, or
resets as an overflow fallback. The positive `int64` ordinal is zero-extended
to `uint64-BE` only for the jitter preimage.

The scheduling ordinal is incremented and persisted on the Entry before the
delay is calculated:

- current-generation `generation_transport_failure_count` after increment for
  transport-unavailable;
- current-generation `generation_unknown_outcome_count` after increment for
  publish-outcome-unknown; and
- current-generation `generation_failure_count` after increment for
  event-retryable.

The two infrastructure counters do not consume the event-specific failure
budget and cannot park an Entry. They exist so lease expiry and unrelated
failure classes cannot silently push another class to its backoff cap, and so
terminal Attempt retention cannot reset the ordinal. Approved replay resets all
three generation-local counters; P03-08B initializes them to zero.

The jitter preimage is exact binary concatenation:

```text
ASCII "threadline.outbox.backoff-jitter/v1\0"
|| 32-byte claim_token_digest
|| one failure-class byte
|| uint64-BE(n)
```

Failure-class bytes are `0x01` transport unavailable, `0x02` publish outcome
unknown, and `0x03` event retryable. `jitter_digest` is SHA-256 of the preimage.
This makes a recorded attempt reproducible in tests while a fresh random claim
token changes the schedule. Jitter is not security authority.

**Threadline decision:** P03-08C also has a process-local breaker per resolved
`(JetStream domain, stream, subject)` mapping. Three consecutive transport or
unknown outcomes open it; no new Entry is claimed for that mapping while open.
The first open interval is 5 seconds and each failed half-open probe doubles it,
capped at 60 seconds. Exactly one half-open probe per Worker process is allowed.
A verified PubAck closes and resets the breaker. The database backoff remains
authoritative; the breaker cannot mutate claims, create delivery success, park
an Entry, or replace cross-process correctness.

### Backoff golden vector

For the claim-token vector in Section 4, `n = 1` yields:

| Class | SHA-256 jitter digest | Default window | Exact delay |
| --- | --- | ---: | ---: |
| `0x01` transport | `6c2e9759ec5c88cd0f690fba0aa6b2080050b3847550e5a8006b99d7ec057a23` | 1,000 ms | 443 ms |
| `0x02` unknown | `0ccaa6097b4a73e6df3054cf407237093ca456671aafd35170722eaed3d10533` | 5,000 ms | 3,268 ms |
| `0x03` event retryable | `134e836dc579d30b984d67f542c19abcf85038886bd4dbb18a46e725aea611cb` | 5,000 ms | 4,312 ms |

## 4. Claim-token profile

**Threadline decision:** each Attempt obtains a fresh 32-byte value from
`pgcrypto.gen_random_bytes(32)` inside the claim transaction. The raw bytes are
returned exactly once to the authenticated Worker and are never stored.

The only wire form is unpadded RFC 4648 base64url: the Go spelling is
`base64.RawURLEncoding.Strict()`. It is exactly 43 ASCII characters from
`[A-Za-z0-9_-]`. Decode rejects `=`, whitespace, CR/LF, non-alphabet bytes,
non-zero trailing padding bits, lengths other than 43, decoded lengths other
than 32, and any string whose re-encoding is not byte-for-byte identical.
Go's official library defines `RawURLEncoding` as unpadded URL-safe base64 and
documents strict trailing-bit validation. [Go `encoding/base64`](https://pkg.go.dev/encoding/base64)

The stored digest is exact binary SHA-256:

```text
SHA-256(
  ASCII "threadline.outbox.claim-token/v1\0"
  || raw 32 token bytes
)
```

The database stores only the 32-byte digest (`octet_length = 32`). Renewal,
acknowledgement, and failure first validate canonical wire form and compute the
domain-separated candidate digest in the trusted Go adapter. The stored digest
is never selected or returned to Worker. A reviewed, least-privilege database
function locks the exact current Claim tuple and compares the fixed 32-byte
candidate with the stored digest as the atomic fencing condition. PostgreSQL
does not promise that ordinary `bytea` equality is constant-time, and this
contract does not claim otherwise. Security rests on the unexposed digest,
256-bit random preimage, exact tuple fence, TLS/local database boundary,
workload credential isolation, bounded connection pool, and secret-safe
result. A malformed token and a well-formed non-match return the same
`claim-denied` category. A future constant-time database primitive requires
its own reviewed implementation and migration.

The Worker database connection carrying the one-time raw token must be local or
TLS-protected, consistent with the PostgreSQL `pgcrypto` security note. Raw
tokens and token-bearing structures must be excluded from logs, metrics,
tracing, errors, DLQ/Parking views, diagnostic bundles, panic rendering, and
broker messages.

Golden vector:

```text
raw bytes (hex): 000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f
wire:            AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8
stored digest:   0fced3787dc44e7855171187da0812df307108fb766c97f6082824715b310994
```

## 5. Destination and broker compatibility

**Threadline decision:** v1 registers exactly one logical Outbox destination:
`domain-events`. It is a descriptor name stored in PostgreSQL, not a NATS
Stream or subject. Core may select it only through a registered
Event-Type/version descriptor; commands cannot supply it as an arbitrary
string.

P03-08C's trusted deployment configuration maps `domain-events` to exactly one
JetStream domain, Stream, and subject derivation rule. Worker alone resolves
that mapping. Tenant IDs, Event/aggregate IDs, payload data, and request fields
cannot select a domain, Stream, or subject. The expected Stream name is sent as
a publish precondition and the returned PubAck Stream must match it exactly.

Before relay readiness, Worker must inspect the effective broker and Stream
configuration and prove all of the following:

1. JetStream is available and the configured credentials can publish to the
   registered mapping;
2. server `max_payload >= 327680` bytes;
3. Stream `max_msg_size` is unlimited or `>= 327680` bytes;
4. Stream duplicate window is exactly 120 seconds;
5. the Stream captures the resolved subject and returns PubAcks; and
6. headers are enabled so the contract's 64-character lowercase message ID can
   be sent as `Nats-Msg-Id`.

Failure of any check keeps Worker non-ready and prevents new claims. An already
open database claim is not converted to success; it follows its persisted
deadline and ordinary expiry/recovery semantics.

The runtime probe does not claim to prove the universal negative that the
credential can publish nowhere else. Platform owns a reviewed NATS permission
configuration plus deployment Contract Tests that prove the registered subject
is allowed and representative forbidden subjects, including another Tenant,
another destination, and wildcard escape attempts, are denied. Failure keeps
the Worker deployment non-ready.

## 6. Retention

**Threadline decision:** Domain Event and Transactional Outbox Entry rows are
permanent Outbox rebuild facts. P03-08B exposes no purge operation and grants no
delete authority to Core or Worker. The default terminal Attempt evidence
window is 90 elapsed 24-hour days after database `finished_at`; a future trusted
Retention deployment may select 30 through 365 days inclusive.

When a terminal outcome is written, its policy identifier and immutable
`evidence_not_before = finished_at + retention_days * 86400 seconds` are
persisted using PostgreSQL time. This is elapsed time, not session-time-zone
calendar arithmetic. Later policy changes do not move that timestamp earlier. A later
Retention-owned purge may remove an Attempt only when database time is at or
after `evidence_not_before` and every longer Domain, security, legal-hold,
Audit, and replay-evidence gate has independently passed. Active Attempts are
never eligible. Parked/unresolved replay evidence is never auto-purged. Deleting
terminal Attempt evidence must not delete or change the Event, Entry, counters,
delivery state, or message-ID inputs.

## 7. Policy source, validation, and rollout

**Threadline decision:** the source is one reviewed, deployment-managed UTF-8
JSON document. The only environment input is
`THREADLINE_OUTBOX_POLICY_FILE`, an absolute path to that file; there are no
per-field environment fallbacks or overrides. Core/Worker load it at process
startup. It is not database content writable by a runtime role and is not an
RPC, event, payload, header, or command field. Deployment/Platform owns values;
Integration owns parsing and semantic validation; Security owns token-profile
changes; Retention owns evidence deletion.

The top-level JSON object contains exactly these required keys and no others:
`policy_id`, `payload_hard_bytes`, `wire_hard_bytes`, `batch_size`, `lease_ms`,
`absolute_lifetime_ms`, `event_retry_ceiling`, `transport_base_ms`,
`transport_cap_ms`, `unknown_base_ms`, `unknown_cap_ms`, `event_base_ms`,
`event_cap_ms`, `retention_days`, and `duplicate_window_ms`. Numbers are JSON
integers, not strings or floating-point spellings. Parsing rejects duplicate
keys before semantic validation. The v1 default document uses the default
values in Section 2 expressed in those units.

Every numeric token must lexically match `0|[1-9][0-9]*` before range
conversion. The strict parser performs token-level duplicate-key detection,
rejects a UTF-8 BOM, and requires EOF after the single top-level object. Thus
exponent, decimal, signed, leading-zero, `null`, boolean, string, array, and
second-document spellings cannot become an integer by decoder coercion.
`payload_hard_bytes` and `wire_hard_bytes` must equal the v1 constants 262144
and 327680 respectively; they are assertions, not operator-tunable ceilings.
Changing either requires a new reviewed policy/protocol version rather than a
v1 reload.

```json
{
  "policy_id": "threadline.outbox.policy/v1",
  "payload_hard_bytes": 262144,
  "wire_hard_bytes": 327680,
  "batch_size": 64,
  "lease_ms": 30000,
  "absolute_lifetime_ms": 300000,
  "event_retry_ceiling": 8,
  "transport_base_ms": 1000,
  "transport_cap_ms": 60000,
  "unknown_base_ms": 5000,
  "unknown_cap_ms": 300000,
  "event_base_ms": 5000,
  "event_cap_ms": 300000,
  "retention_days": 90,
  "duplicate_window_ms": 120000
}
```

The schema hard limits and protocol shapes in this document cannot be widened
by configuration. Runtime values must parse in their stated base unit, be
finite integers, lie within every inclusive range, satisfy cross-field rules,
and include an exact known policy identifier. Unknown keys, unknown policy
identifiers, missing required keys, fractional values, unitless duration text,
overflow, or broker incompatibility fail closed.

Core producer readiness additionally requires the schema/extension checks and
a registered Event-Type descriptor. Worker relay readiness additionally
requires the broker checks in Section 5. A failed check does not make daily IM
unavailable, but the affected producer cannot return a Durable Client ACK for a
transaction requiring an Outbox Event, and Worker cannot claim new work.

Every Entry generation snapshots the trusted `policy_id`,
`policy_snapshot_digest`, and these exact effective values when the generation
is created: `effective_lease_ms`, `effective_absolute_lifetime_ms`,
`effective_event_retry_ceiling`, `effective_transport_base_ms`,
`effective_transport_cap_ms`, `effective_unknown_base_ms`,
`effective_unknown_cap_ms`, `effective_event_base_ms`,
`effective_event_cap_ms`, and `effective_retention_days`. Every Attempt copies
the policy identifiers plus `effective_lease_ms` and
`effective_retention_days`, and persists its exact claim/lease/absolute
deadlines; renewal therefore never reads the current startup policy.
Terminalization uses the Attempt's retained `effective_retention_days` and
persists its exact next-attempt time and evidence cutoff.
Therefore a reload never reinterprets an active Claim, current Attempt,
scheduled retry, current generation budget, or already-terminal retention
date. Batch size and breaker state may change for future calls because neither
changes stored authority.

`policy_id` is the exact ASCII string `threadline.outbox.policy/v1`. The
32-byte `policy_snapshot_digest` is SHA-256 over this exact binary preimage:

```text
ASCII "threadline.outbox.policy-snapshot/v1\0"
|| uint32-BE(payload_hard_bytes)
|| uint32-BE(wire_hard_bytes)
|| uint32-BE(batch_size)
|| uint32-BE(lease_ms)
|| uint32-BE(absolute_lifetime_ms)
|| uint32-BE(event_retry_ceiling)
|| uint32-BE(transport_base_ms) || uint32-BE(transport_cap_ms)
|| uint32-BE(unknown_base_ms)   || uint32-BE(unknown_cap_ms)
|| uint32-BE(event_base_ms)     || uint32-BE(event_cap_ms)
|| uint32-BE(retention_days)
|| uint32-BE(duplicate_window_ms)
```

A changed semantic value produces a new snapshot digest even within the v1
accepted range. Stored effective values, not the digest alone, drive database
behavior; the digest is an integrity/audit binding. Binaries must retain v1
evaluation for every snapshot present in nonterminal rows before rollout.
Rollout order is expand/validate readers, deploy config, then allow new
generations. Rollback must still evaluate the newer snapshot or stop claiming.

The Section 2 default values produce this required Golden vector:

```text
preimage_hex = 7468726561646c696e652e6f7574626f782e706f6c6963792d736e617073686f742f76310000040000000500000000004000007530000493e000000008000003e80000ea6000001388000493e000001388000493e00000005a0001d4c0
sha256_hex   = 9c9d119ee28a1237b0c9b95cdf3a79dff57132dbb871d202cb937d5f5b72dec5
```

## 8. Implementation placement

**Threadline decision:** P03-08B places invariant data-shape checks in
PostgreSQL: payload bytes, exact destination registry, state/tuple constraints,
non-negative and retry-ceiling counters, 32-byte token digests, immutable
policy/deadline snapshots, and database-time scheduling. Its reviewed query
surface accepts only bounded batch/lease policy parameters and rejects all
invalid values atomically. It does not implement purge, replay, broker access,
or production Event descriptors.

P03-08C owns strict config parsing, full wire-size measurement, broker/Stream
inspection, logical-to-physical destination mapping, breaker state, canonical
token decoding/candidate hashing, and exact PubAck normalization. Broker
configuration is defense in depth; database functions remain the only delivery
state authority.

## 9. Executable acceptance matrix

P03-08B and P03-08C inherit these cases. Each invalid case must fail before a
state mutation or publish and must expose only the contract's stable,
secret-safe category.

| Area | Required cases |
| --- | --- |
| Payload | Insert lengths `0`, `262143`, and `262144`; reject `262145`. Confirm limit is bytes, including invalid UTF-8/NUL, and payload is not parsed or logged. |
| Wire message | Accept measured totals `327679` and `327680`; reject `327681`. Reject startup for server/Stream cap `327679`; accept each at `327680` and above. |
| Batch | Accept `1`, `64`, `256`; reject `0`, `257`, negative, fractional, absent, and integer overflow. Claim returns at most the accepted batch. |
| Lease | Accept lease `5`, `30`, `120` seconds and absolute `30`, `300`, `900`; reject one below/above each range and absolute `< 2 * lease`. Test renewal exact cap, expired equality, and database rather than Worker time. |
| Retry ceiling | With default 8, failures 1-7 reschedule and 8 parks; transport/unknown counts never park; permanent parks immediately; lease expiry changes neither failure count nor backoff. Reject ceilings `0` and `21`. |
| Backoff | Golden vectors above; test each class ordinal at `1`, `2`, `21`, and `MaxInt64`; result always lies in closed `[0, window]`, reaches but never exceeds cap, never wraps, and uses only its matching persisted counter. Lease expiry and a different failure class do not change that counter. Increment at `MaxInt64` returns `persistence-failure` with no write. |
| Token generation | Assert 32 random bytes requested, 43-character canonical wire, distinct token/digest over many claims, stored digest length 32, raw token absent from rows/logs/errors. Use the golden vector. |
| Token rejection | Reject padded, 42/44-character, CR/LF/space, alternate alphabet, non-zero trailing bits, 31/33-byte decoded, stale, expired, wrong Tenant/Entry/Event/attempt/generation tokens with identical `claim-denied`. Prove the stored digest is never returned to Worker and only the reviewed atomic database function can compare/mutate; do not assert unsupported timing behavior. |
| Claim fencing | Concurrent claim, renew, ACK, failure, and expiry use the exact tuple plus digest; only one mutation wins; re-observed exact completed ACK is non-mutating. Active deadline is unchanged across policy reload. |
| Broker | Reject absent JetStream, wrong Stream/subject mapping, disabled headers/PubAck, duplicate window other than 120 seconds, malformed/foreign PubAck, and message-ID mismatch. Accept exact `Duplicate=true` PubAck. |
| Circuit and alerts | With a fake clock, prove outcome 2 remains closed and 3 opens; OPEN performs no claim; only one HALF_OPEN probe runs; failures reopen at `5/10/20/40/60` seconds and cap; verified PubAck closes/resets. Restart while broker readiness is unverified starts open, and config reload cannot discard an open state. Assert warning/critical thresholds below. |
| Broker permissions | Runtime proves the configured target publish path. Deployment Contract Tests prove representative cross-Tenant, other-destination, and wildcard-escape subjects are denied by the reviewed NATS permission config. |
| Destination | Accept only registry-derived `domain-events`; reject casing changes, whitespace, arbitrary Stream/subject, unknown Event descriptor, and request-selected destination. |
| Retention | Default cutoff is exactly `finished_at + 90 * 24h`; accept trusted 30/365, reject 29/366; active Attempts and Event/Entry rows never purge; a longer gate/hold wins; reload never shortens a stored cutoff. |
| Config/rollout | Reject missing/unknown/duplicate keys; BOM; empty/trailing/second documents; `null`, boolean, string, array, signed, `-0`, leading-zero, exponent (`1e3`), decimal (`1000.0`), fractional, unitless, out-of-range, cross-field-invalid, uint32/int64 overflow, any v1 payload/wire hard value other than `262144`/`327680`, and unknown-policy inputs. Verify affected component remains non-ready, does not claim/publish/ACK, and still evaluates old active-policy rows during expand and rollback. |

## 10. Fixed handoff inputs

P03-08B consumes: 262,144-byte payload `CHECK`; exact `domain-events`
destination; batch `64 (1..256)`; lease `30s (5..120)`; absolute lifetime
`300s (30..900, >=2x lease)`; event ceiling `8 (1..20)`; the three exact
backoff profiles and jitter construction; 32-byte token generation and digest;
policy snapshots; and a 90-day terminal-evidence cutoff with purge unavailable.

P03-08C consumes: 327,680-byte complete wire cap; exact broker compatibility
checks; 120-second duplicate window; stable message-ID/PubAck rules from the
parent contract; canonical 43-character base64url token decoding and an
unexposed, atomic database digest fence; logical destination mapping; the exact
process-local breaker; and fail-closed startup/readiness behavior.

The only v1 operator-tunable inputs are those with an accepted runtime range in
Section 2, and changing a semantic input requires a new durable policy snapshot.
No operator setting can widen a schema/protocol hard rule.

## 11. Operational alert thresholds

**Threadline decision:** metrics remain bounded and never use Tenant/Event IDs
as labels. For each resolved destination mapping, warning thresholds are:
circuit continuously open for 2 minutes, oldest eligible Entry older than 30
seconds, claim-expiry ratio above 1% over 5 minutes, or any newly parked Entry.
Critical thresholds are: circuit continuously open for 10 minutes, oldest
eligible Entry older than 5 minutes, claim-expiry ratio above 5% over 5
minutes, or at least 10 newly parked Entries in one minute. Backlog size, claim
latency, PubAck latency, and total Attempt growth are always recorded; their
capacity-specific paging thresholds are intentionally deployment-owned.

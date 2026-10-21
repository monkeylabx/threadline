# Domain Event wire envelope v1

Owner: Contracts. Issue: #297. Consumers: Core, Realtime-worker.

## Responsibility and trust boundary

`threadline.outbox.v1.DomainEventEnvelope` transports one immutable domain Event
from the transactional Outbox. It is not a client command, approval, capability,
or substitute for Core authorization. The Encoder takes the claimed immutable
PublishFacts, copies the eight fields below, and never inspects payload content.
Transport failures must report bounded categories without payload or credentials.

| Tag | Field | v1 constraint |
| --- | --- | --- |
| 1 | envelope_version | Exactly 1; missing/zero/unknown version is rejected |
| 2 | tenant_id | Nonempty canonical opaque tenant identifier from the claim |
| 3 | event_id | Nonempty canonical opaque Event identifier from the claim |
| 4 | event_type | Registered exact Event type; no inferred payload schema |
| 5 | schema_version | Positive registered version, at most 2147483647 |
| 6 | occurred_at | Present valid Protobuf Timestamp from immutable Event time |
| 7 | payload | Byte-exact opaque data, 0..262144 bytes; empty is not missing |
| 8 | outbox_entry_id | Positive int64; diagnostic identity only |

Identifiers keep the existing Outbox rules: no leading/trailing whitespace or
control characters. Encoding rejects invalid claim facts or Timestamp overflow.
No aggregate ID/kind, delivery attempt, replay generation, claim token, broker
subject, model name, or authorization decision is added to the wire. A decoder
must bound the complete incoming message to 327680 bytes before parsing; the
publisher measures that cap including NATS headers. Unknown Event type/schema
pairs never become client events or commands: consumers skip unsupported pairs
without projecting them. Malformed/unsupported envelope versions fail closed.

## Destination and delivery

The sole mapping is `domain-events` → JetStream stream
`THREADLINE_DOMAIN_EVENTS_V1` → subject `threadline.domain.events.v1`.
Stream subjects contain exactly that subject, with file storage, limits retention,
PubAck enabled, and duplicate window exactly 120 seconds. Startup validates the
existing [broker policy](../architecture/transactional-outbox-policy.md); Worker
has publish/inspect rights, never Stream creation rights. Platform provisions it.

`Nats-Msg-Id` is the exact claim-provided ID from
[Outbox §8](../architecture/transactional-outbox-contract.md#8-publish-and-consumer-contract).
The shared `t`/`e`/`domain-events`/generation-0 preimage and digest in that section
remain authoritative; this contract does not introduce another hashing scheme.
There is no NATS header derived from payload or a client-selected destination.
A retry may duplicate a delivery after the broker window. Durable consumers
atomically deduplicate by `(tenant_id, event_id)` with their projection change;
`outbox_entry_id`, stream sequence, and timestamps are not domain ordering keys.

Realtime uses this delivery only as a notification. It authenticates the Device
and asks Core for currently authorized ordered events via the reviewed Sync
interface; broker bytes never confer membership or bypass retention/revocation.
Reconnect and gaps use Core cursors; JetStream is not the history authority.

## Compatibility and rollback

The Protobuf source is additive v1. Never reuse tags or JSON names; reserve removed
fields. Field 50000 is permanently reserved for synthetic compatibility evidence.
Known v1 fields keep their meaning when unknown fields are present. Persisting or
forwarding consumers must retain unknown wire fields (or the original bytes);
projection-only readers may discard them after extracting known values.

This is a new package: the N-1 application has no decoder and must not receive
activated production of this type before capability rollout. Within v1, an
N-1 decoder must preserve later additive fields on read/write. The Golden fixture
contains a field-50000 canary and the verifier exercises this raw preservation
seam, malformed frames and schema drift. This is structural/wire evidence, not a
claim of five-language generated-adapter or live broker validation.

Roll out schema/readers before the Encoder. Rollback stops the relay; committed
Outbox facts remain in PostgreSQL for retry. Never purge or reinterpret retained
bytes. Platform stream provisioning is a separate task; no Core or Worker logic
changes are part of #297.

## Verification

- `node proto/tools/verify-domain-event-contract.mjs`
- `node proto/tools/verify-contracts.mjs` (includes the verifier above)
- `node scripts/proto.mjs check` and `node scripts/proto.mjs breaking`
- `buf convert --type threadline.outbox.v1.DomainEventEnvelope --from <fixture.json> --to <output.bin>` verifies the fixture against the compiled schema.

# Crypto and recovery protocol fixtures

These deterministic T019 fixtures contain synthetic identifiers, timestamps,
enum values and short opaque byte patterns only. They contain no real Device
Credential, KeyPackage, MLS frame, History Key, recovery material, approval
note, user content, production tenant identifier or secret.

The scenarios exercise the observable contract boundary: tenant/Device/Profile
binding, the exact inclusive KeyPackage lifetime window, atomic single-use
claims, strict membership sequence and Epoch transitions, `rekey_required`,
PrivateMessage handshake policy, bounded History Sharing, and scope-bound
two-person enterprise recovery to one Device. The evaluator never opens MLS or
wrapped bytes.

Current Recovery Case approvals bind the requester ActorRef and an ascending,
unique list of per-Group `RecoveryScope` entries. v1 requires every entry to
share exactly the same Epoch and half-open time bounds so current writers can
apply one validation rule across entries. Current entries also carry non-empty
typed protected targets and never double-write authority to the legacy N-1
projection: Group IDs are empty, Epochs are zero and TimeRange is absent. These
poison values are included in the current approval hash. N-1 requests can decode
old shared per-Group bounds for audit, but cannot normalize missing targets or
authorize Create, Decide or Execute. Heterogeneous bounds fail closed and are
never widened into a union.

Run from the repository root:

```bash
node proto/tools/verify-crypto-contracts.mjs
```

## Canonical transcript v1

`transcripts.json` freezes signing-input and binding-hash bytes independently
of any language runtime or cryptographic provider. A transcript is UTF-8 bytes
of the ASCII domain prefix `threadline.crypto.transcript/v1/<domain>`, one LF,
then the RFC 8785 JSON Canonicalization Scheme (JCS) encoding of the projection.
The allowed domains are `credential`, `publication`, `membership`, `history`,
`recovery-reason`, `recovery-case`, `recovery-decision`, `recovery-protected-scope`,
`recovery-scope-binding`, `recovery-envelope`,
`channel-event-sender-binding`, `recovery-evidence-grant`,
`recovery-delivery`, and `recovery-commit-attestation`.
Domain labels and version are part of the hashed bytes.

Every vector names one representative scenario in `manifest.json`. The
verifier invokes the same live projection builder used by scenario evaluation,
requires deep equality with `vector.input`, then verifies canonical bytes and
SHA-256. The vectors are therefore generated views of production projections,
not separately hand-maintained examples or partial pseudo-contracts.

Projection rules are presence-sensitive and language-neutral:

- Proto field names use lowerCamelCase; nested messages are nested objects.
- `uint32` values use JCS integers. `uint64` values use unsigned base-10 strings
  with no leading zero, avoiding host-number precision loss.
- Enum values use their unsigned protobuf wire number as a base-10 string;
  symbolic enum names never enter a transcript.
- Timestamps use normalized RFC 3339 UTC strings ending in `Z`; fractional
  seconds are omitted when zero, otherwise trailing fractional zeros are
  removed.
- Bytes use lowercase hexadecimal without `0x`; field names end in `Hex`.
- An absent presence-bearing field or inactive `oneof` arm is JSON `null` when
  it belongs to the frozen projection. Default scalar fields are included with
  their explicit schema value.
- Repeated fields preserve contract order. Fields documented as sets are first
  sorted by unsigned UTF-8 byte order; duplicates are rejected, never removed.
- JCS recursively sorts object member names. Implementations must not hash a
  language's ordinary object serialization.

The vector SHA-256 values freeze only these canonical input bytes. They do not
approve a signature algorithm, key provider, MLS implementation, or production
cryptographic admission.

The membership kind wire map is pinned exactly as ADD=1, REMOVE=2,
UPDATE_DEVICE_KEY=3, SELF_UPDATE=4, RECOVERY_KEY_ROTATION=5, REINITIALIZE=6,
and REVOKE_DEVICE=7. `membershipKindVectors` derives one representative hash
for every value from the same live builder; cross-kind substitution is rejected
even when all target fields would otherwise be meaningful.

`channel-event-sender-binding` covers T015 sender-authored fields 1-6, 8-12,
14-15, 18, 20-23. Conversation/body presence and optional attribution are
explicit; enums, uint64s, timestamps and bytes follow the rules above. Field 20
contributes the exact domain-separated `recovery-envelope` transcript hash.
Core verifies `sender_signature` before producing this JCS SHA-256. Neither
`server_commit` nor `sender_signature` is an input.

## Directional N-1 matrix

- DeviceCredential missing fields 11-12: a current reader verifies the explicit
  legacy fields 1-8 signature and resolves `approved_by` through the authorized
  root directory only as verification provenance. Exactly one matching root
  permits read/sync as legacy format 0; it is never serialized as format 1 and
  requires true reissue before publishing a current KeyPackage or signing.
  Zero or multiple roots fail closed.
- E2EEGroup missing fields 10-11: only a baseline Group with no predecessor,
  successor, or REINITIALIZE evidence normalizes to generation 1. Successor or
  reinitialized Groups are read-only/incompatible.
- MembershipChangeAuthorization missing fields 11-18 remains list/audit data
  only. It cannot authorize execution; the client requests a current grant.
- MlsWireMessage missing fields 7-13 uses a per-type matrix over an authenticated
  T015 ChannelEvent, with `event_id` as replay identity: Commit requires Private
  form and the exact pending authorization; Proposal requires Private form,
  empty authorization and successor Epoch zero; Welcome requires non-empty
  targets and exactly one accepted durable Commit correlation; GroupInfo uses
  Independent form and permits zero or one related Commit. Missing or ambiguous
  bindings remain audit-only and fail closed.
- HistorySharingGrant missing fields 12-15 remains readable for audit only. It
  cannot unwrap or grant History access; the client requests a current grant.

The reverse direction is explicit and server-enforced. Before using current-only
semantics, Core requires the exact `CryptoCompatibilityService` v1 surface floor,
`minimum_active_client_contract_version >= 1`, and
`legacy_mutation_disabled=true`. Missing, duplicate, unknown, zero, or
rollback-lowered entries block delivery/mutation; an old server returns
UNIMPLEMENTED. Per surface:

- A current DeviceCredential cannot be validated by an N-1 signature projection,
  so it is audit/preserve-only and cannot publish or sign.
- An E2EEGroup with generation 1 and no predecessor/successor has the same legacy
  read projection and may remain read-only. A reinitialized/successor Group is
  blocked from N-1 delivery.
- A current MembershipChangeAuthorization is audit-only for N-1 and can never
  execute a Commit; legacy membership mutation stays disabled.
- A current MlsWireMessage is not applied by N-1. It remains audit-only and the
  server blocks delivery until the client floor is satisfied.
- A current HistorySharingGrant is audit-only for N-1 and never unwraps History
  Keys; legacy grant mutation and current grant delivery remain blocked.

Explicit protected targets use an independent server-first gate. A current
administrative client calls `GetTargetedRecoveryCapabilities` and proceeds only
after observing `explicit_protected_targets=true`, contract version 1, and a
recorded `RecoveryProtocolFloor` with minimum targeted version 1 and
`legacy_mutation_disabled=true`. Missing/unknown floors and rollback fail closed.
The separate additive `TargetedRecoveryService` never serializes a targeted Case
into legacy Recovery request/Case bytes; an N-1 server therefore returns
UNIMPLEMENTED without invoking or mutating `RecoveryService`. Legacy Cases remain
decode/audit-only and cannot infer exact object or History targets.

The final compatibility scenario reuses T014's persisted RecoveryEnvelope
Golden Frame and confirms only a local static seam: field `50000` exists and the
fixture field slicer retains its input bytes. It does not prove generated-code
round trips or five-language N-1 compatibility by itself. T019-B separately
records the protected formal result in `formal-codegen-evidence.json` and the
five-language bidirectional result in `generated-n-minus-one-evidence.json`;
the fixture manifest and verifier bind both files by SHA-256.

An N-1 KeyPackage without the additive current publication fields has a narrow
migration path: it must arrive through an authenticated Device-bound publish
session and its outer Device/Profile and validity window must match. The server
first claims the package atomically and terminally, then returns its public
bytes. Before forming a Commit, the current Committer validates the opaque MLS
KeyPackage Credential, Profile, Leaf lifetime, and signature. Failure leaves
the package consumed or invalid and produces no Commit; this trades availability
for single-use safety. These local scenarios exercise the decision seam only.
The client crypto implementation evidence remains `PENDING_IMPLEMENTATION`, so
this fixture does not claim real cryptographic verification.

An N-1 RecoveryEnvelope without fields 11-12, delivery metadata fields 13-17,
or authoritative protected-scope field 18 remains decodable, preservable, and
auditable, but a current scoped Execute
always returns `ERROR_CODE_RECOVERY_UNAVAILABLE`. It never infers object
identity or time scope from legacy Tenant, Group, or Epoch fields. Compatibility
is directional: an N-1 client can ignore additive metadata and consume a current
field-2 result using the established semantics; that does not make an N-1
result executable by a current client.

Current recovery envelopes contain only sender-authored typed object or History
scope. After approval, isolated Recovery Control signs a minimal
`RecoveryEvidenceGrant` over Case/Tenant, canonical scopes, recipient, expiry,
policy, execution ID and Case binding. It contains no reason, note or plaintext.
During Execute, Recovery Control calls Core's internal `RecoveryEvidenceService`
under its workload identity. Core verifies mTLS, the domain-separated grant
signature and expiry, discovers matching records from the single durable event
store, and returns exact envelope/attestation pairs. The caller never supplies
an envelope or precomputed envelope hash, and the services do not share a DB.
Core derives each attestation only after validating the persisted ChannelEvent
sender signature. Recovery Control requires one attestation per envelope,
checks both transcript hashes, and passes still-opaque wrapped material to the
KMS/HSM. Only `server_committed_at` is used for Case TimeRange checks.

Physical iOS/Android, Secure Storage, real KMS/HSM, real-device FFI and
production OpenMLS/Crypto Provider admission are `NOT RUN` or not established.

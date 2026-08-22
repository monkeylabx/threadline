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
double-write sorted Group IDs and those bounds to the legacy N-1 projection.
N-1 requests normalize into the same per-Group list; heterogeneous bounds fail
closed and are never widened into a union. Legacy fields do not participate in
the current approval hash, but their double-written values must match exactly.

Run from the repository root:

```bash
node proto/tools/verify-crypto-contracts.mjs
```

The final compatibility scenario reuses T014's persisted RecoveryEnvelope
Golden Frame and confirms only a local static seam: field `50000` exists and the
fixture field slicer retains its input bytes. It does not prove generated-code
round trips or five-language N-1 compatibility. Those checks remain
`PENDING_INTEGRATION` on the protected formal-codegen workflow.

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

An N-1 RecoveryEnvelope without fields 11-12 or delivery metadata fields 13-17
remains decodable, preservable, and auditable, but a current scoped Execute
always returns `ERROR_CODE_RECOVERY_UNAVAILABLE`. It never infers object
identity or time scope from legacy Tenant, Group, or Epoch fields. Compatibility
is directional: an N-1 client can ignore additive metadata and consume a current
field-2 result using the established semantics; that does not make an N-1
result executable by a current client.

Physical iOS/Android, Secure Storage, real KMS/HSM, real-device FFI and
production OpenMLS/Crypto Provider admission are `NOT RUN` or not established.

# Threadline Protocol

`.proto` is the cross-platform source of truth. Go services, the Rust client
core, Desktop TypeScript, iOS Swift, and Android Kotlin generate from this
module. Generated implementations are adapters and must not leak
generator-specific types back into the contract.

Everything under `proto/` is owned by the `contracts` workstream. Land a
contract change first, then let server and client implementations consume the
same merged seam.

## Layout

```text
proto/
  threadline/<domain>/v1/*.proto  versioned product contracts
  golden/v1/                     persisted-envelope compatibility evidence
  tools/                         repository-local contract/codegen checks
  toolchain.lock.json            exact generator and runtime pins
```

A stable `v1` package is add-only. Incompatible redesigns use a new package;
they never reinterpret existing `v1` bytes.

## Daily contract checks

From the repository root:

```bash
make proto
make proto-breaking
node proto/tools/verify-contracts.mjs
```

`make proto` builds, lints, and format-checks the module. The breaking check
uses the merged `proto/buf.yaml` policy against `main`; it must not be skipped
now that `main` contains the protocol baseline.

```bash
make proto-generate
```

The developer command generates each language whose pinned plugins are
available and names any language it skipped. Generated output is ignored and
reproduced from the contract source.

| Language | Template | Output | Plugins |
| --- | --- | --- | --- |
| Go | `buf.gen.go.yaml` | `services/gen/` | `protoc-gen-go`, `protoc-gen-connect-go` |
| TypeScript | `buf.gen.ts.yaml` | `packages/generated-ts/src/` | `protoc-gen-es` |
| Rust | `buf.gen.rust.yaml` | `crates/client-proto/src/generated/` | `protoc-gen-prost`, `protoc-gen-prost-crate` |
| Swift | `buf.gen.swift.yaml` | `packages/generated-swift/Sources/ThreadlineProto/` | `protoc-gen-swift`, `protoc-gen-connect-swift` |
| Kotlin | `buf.gen.kotlin.yaml` | `packages/generated-kotlin/src/main/` | Java, Kotlin, and Connect Kotlin generators |

Buf runs against the checkout without a public BSR name or dependency.

## Formal release codegen

Daily generation is not formal release evidence. Formal verification requires
an approved clean-environment launcher to authenticate the reviewed manifest
and bundle-absolute Node before starting
`proto/tools/verify-codegen.mjs --mode=verify-only`. A bare `node` invocation
cannot prove that bootstrap boundary.

Only the Integration Owner may use `--mode=repository`. It requires an
explicit acknowledgement, a clean-worktree/repository lock, snapshotted tools,
and destination-symlink checks. The bytes verified in the temporary output are
the bytes installed; do not follow a verified run with a separate naked
`buf generate`.

See [the codegen trust contract](../docs/contracts/codegen.md) and
[contract workflow](../docs/contracts/README.md). Representative concrete
Golden Frames now exist. T014 remains HOLD until the formal plan is reconciled
with the merged multi-package protocol, five-language unknown-field/N-1
evidence exists, and the Swift builder is authenticated on the protected
release runner.

## Packages

| Package | Contents |
| --- | --- |
| `threadline.type.v1` | Actor references, encrypted fields, pagination, and shared errors |
| `threadline.identity.v1` | Organization, Space, Member, Agent, Device, enrollment, and Session |
| `threadline.crypto.v1` | Crypto Profile, Device Credential, KeyPackage, Group, Epoch, History, and Recovery |
| `threadline.channel.v1` | Channel, DM, Thread, membership, workspace binding, and Agent policy |
| `threadline.message.v1` | Ciphertext envelope and encrypted payload schema |
| `threadline.sync.v1` | Sync/read cursors, gap repair, and signed checkpoints |
| `threadline.realtime.v1` | WSS frames, ACK, delivery, presence, and backpressure |
| `threadline.capability.v1` | Grants, workspace leases, fencing, and Execution Owner |
| `threadline.task.v1` | Task, Context Manifest, Approval, Run, Event, Artifact, and Usage |
| `threadline.runtime.v1` | Desktop Runtime enrollment, heartbeat, dispatch, and model route |

## What the server may see

Threadline servers store `ChannelEventEnvelope`, not conversation plaintext.
Human-authored content lives in `threadline/message/v1/payload.proto` and is
sealed before leaving the Device, or in an `EncryptedField` when the control
plane must store a value without reading it.

Every cleartext envelope field is a metadata-boundary decision. In particular,
`redaction_target_event_id` supports retention and `attachment_blob_ids`
supports object lifecycle and ACL enforcement. ADR-0003 records the current M0
candidate boundary: v1 does not hide communication relationships, frequency,
or message size. Adding cleartext metadata needs Threat Model review, not only
contract review.

## Rules for changing this module

- Add fields; never renumber or reuse one. Reserve removed field numbers.
- Persisted `ChannelEventEnvelope`, `MessagePayload`, `RecoveryEnvelope`, and
  realtime frames are Golden surfaces and must preserve unknown fields.
- Server-assigned values live outside the sender signature and AEAD associated
  data; clients must not treat them as sender-authenticated.
- `threadline.type.v1.ErrorCode` must express the crypto failure vocabulary in
  `test/crypto/e2ee-interop-v1.vector`.
- Generated files are installed only by the Integration Owner's fixed,
  reviewed workflow and are never hand-edited.

## Still required before the protocol gate closes

- Implement the selected descriptor-backed Rust persistence seam in the Rust
  client owner's path; direct `prost` generated-struct decode/re-encode remains
  forbidden.
- Descriptor-driven five-language unknown-field round trips and N-1 evidence.
- Formal authenticated release codegen on the protected runner.
- Fake Server and Contract Test coverage for the client-facing slice.
- Connect/gRPC interceptors for auth, retry, deadline, and trace.

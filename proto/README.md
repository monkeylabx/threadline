# Threadline Protocol

`.proto` is the cross-platform source of truth. Go services, the Rust client core,
Desktop TypeScript, iOS Swift and Android Kotlin all generate from this module —
none of them hand-write a wire type, and none of them edit generated output.

Owned by the `contracts` workstream. Everything under `proto/` is a shared
surface: land a contract change first, then implement against it in parallel.

## Commands

```bash
make proto
```

`buf build` compiles every file into a descriptor set, which is the
language-independent proof that the contract parses and all cross-file
references resolve. `buf lint` enforces the STANDARD ruleset with no
exceptions, and `buf format --diff --exit-code` keeps diffs reviewable.

```bash
make proto-breaking
```

Checks `WIRE_JSON` compatibility against `main`. Public fields may only be
added; a removed field must be `reserved`. Skips itself on the first landing,
when `main` has no baseline yet.

```bash
make proto-generate
```

Generates every language whose plugins are installed and names the ones it
skipped. No workstation is expected to hold five toolchains; CI installs them
all. Generated output is git-ignored and reproduced from this module.

| Language | Template | Output | Plugins |
| --- | --- | --- | --- |
| Go | `buf.gen.go.yaml` | `services/gen/` | `protoc-gen-go`, `protoc-gen-connect-go` |
| TypeScript | `buf.gen.ts.yaml` | `packages/generated-ts/src/` | `protoc-gen-es` |
| Rust | `buf.gen.rust.yaml` | `crates/client-proto/src/generated/` | `protoc-gen-prost`, `protoc-gen-prost-crate` |
| Swift | `buf.gen.swift.yaml` | `packages/generated-swift/Sources/` | `protoc-gen-swift`, `protoc-gen-connect-swift` |
| Kotlin | `buf.gen.kotlin.yaml` | `packages/generated-kotlin/src/main/` | `protoc-gen-java`, `protoc-gen-kotlin`, `protoc-gen-connect-kotlin` |

Buf runs entirely offline against this checkout. The module declares no `name`
and no `deps`, so nothing here reaches the public BSR — a requirement for
air-gapped builds.

## Packages

| Package | Contents |
| --- | --- |
| `threadline.type.v1` | `ActorRef`, `EncryptedField`, pagination, and the shared `ErrorCode` vocabulary |
| `threadline.identity.v1` | Organization, Space, Member, Agent directory, Device, enrollment, Session |
| `threadline.crypto.v1` | Crypto Profile, Device Credential, KeyPackage, E2EE Group, Membership Change, History Sharing, Recovery Case |
| `threadline.channel.v1` | Channel, DM, Thread, Channel Membership, Workspace Binding, Agent policy |
| `threadline.message.v1` | Ciphertext Envelope and the encrypted payload schema |
| `threadline.sync.v1` | Sync and read cursors, gap repair, signed checkpoints |
| `threadline.realtime.v1` | WSS binary frames: hello, send, Durable ACK, delivery, presence, backpressure |
| `threadline.capability.v1` | Capability vocabulary, Grants, Workspace Leases, fencing tokens, Execution Owner |
| `threadline.task.v1` | Task, Context Manifest, Approval, Run, Step, Run Event, Artifact, Usage |
| `threadline.runtime.v1` | Desktop Runtime enrollment, heartbeat, dispatch, model route |

## What the server may see

Threadline servers store `ChannelEventEnvelope` and nothing else about a
conversation's content. Everything a member authored lives in
`threadline/message/v1/payload.proto`, which is sealed before it leaves the
Device, or in a `threadline.type.v1.EncryptedField` for values like a Channel
topic or a Task title that the control plane must store but must not read.

Every cleartext field in the envelope is a decision about what an operator can
learn, and the two that are not structural are marked as such in the file:

- `redaction_target_event_id`, because retention has to know which stored
  ciphertext a withdrawal applies to.
- `attachment_blob_ids`, because object lifecycle and ACL enforcement have to
  reach the blobs.

ADR-0003 records the accepted limit: v1 does not hide communication
relationships, frequency or message size. Adding a cleartext field to the
envelope changes that boundary and needs a Threat Model review, not just a
contract review.

## Rules for changing this module

- Add fields; never renumber or reuse one. A removed field number becomes
  `reserved` in the same change.
- Persisted envelopes are Golden Frame surfaces. `ChannelEventEnvelope`,
  `MessagePayload`, `RecoveryEnvelope` and the realtime frames must decode
  identically in all five languages before a change is considered landed.
- Unknown fields are preserved on persist and re-emitted verbatim, so an older
  client never destroys material a newer one wrote.
- Server-assigned values live in `ServerCommit`, outside the sender's signature
  and outside the AEAD associated data. A client must not treat them as
  authenticated.

## Not yet in this module

- Golden Vectors and Fake Server (P02-08). The canonical encodings that
  `sender_signature` and the AEAD associated data refer to are specified in the
  proto comments but are not frozen until fixtures exist under `test/crypto/`
  and `test/contract/`, and until P00-08 shows them agreeing across Rust, a
  Swift host and a Kotlin host.
- Connect/gRPC interceptors for auth, retry, deadline and trace (P02-06).
- File upload sessions and blob metadata (P06). Messages reference attachments
  through `AttachmentRef` inside the encrypted payload; the file service that
  creates those blobs is a separate contract.
- Model Control's registry, evaluation and routing API (P10). Only the
  `ModelRoute` a Run executes under is defined here.

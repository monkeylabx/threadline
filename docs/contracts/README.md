# Protocol contracts

Threadline uses versioned Protobuf packages as the stable seam between the IM control plane, realtime transport, clients, Agent runtimes, and local connectors. This directory records the rules around that seam; `proto/` remains the executable source of truth.

## Required checks

| Check | Fixed command | Purpose |
| --- | --- | --- |
| Style | `buf lint` | Enforce package, file, enum, RPC, and comment conventions |
| Compatibility | `make proto-breaking` | Reject source and wire changes covered by the merged `WIRE_JSON` policy |
| Golden Frames | `node proto/tools/verify-contracts.mjs` | Verify persisted-envelope unknown-field canaries, hashes, local-only generation, and output mappings |
| Release verification | approved clean-env launcher + bundle-absolute Node + `proto/tools/verify-codegen.mjs --mode=verify-only` | Verify schema-v4 source/provenance/authentication records and snapshotted runtime closures, reject protocol stubs, require exact generated file sets, and compile the Java/Kotlin pair |
| Generation commit | approved clean-env launcher + bundle-absolute Node + `proto/tools/verify-codegen.mjs --mode=repository` | Install the already-verified temporary output into the six declared generated directories; clean worktree, repository lock, safe destination paths, and explicit Integration Owner acknowledgement required |

The schema bootstrap has already landed on `main`. T014 no longer has an empty
baseline exception: it must build and lint the current module and run the fixed
breaking command against the merged protocol baseline.

The schema-independent canaries do not yet satisfy Issue #28's concrete
Ciphertext/Crypto Envelope Golden Frame acceptance item. The concrete
`ChannelEventEnvelope` and `RecoveryEnvelope` schemas are now present; Issue
#28 remains open until representative frames and cross-language/N-1 evidence
exist, or Contracts and Product explicitly approve and record a scope split.

The workspace intentionally has no BSR module name, BSR dependency, or remote generation plugin. Private and air-gapped builds consume only the repository plus the pinned local toolchain.

A bare `node proto/tools/verify-codegen.mjs ...` run is useful for local
diagnosis but is not formal evidence: only the reviewed launcher can
authenticate Node and sanitize preloads before JavaScript starts. See [Code
generation](codegen.md) for the exact bootstrap boundary and manifest-v4
provenance rules. The branch-local formal generation plan predates the merged
multi-domain templates and must not be used to install SDK output until it is
reconciled with them.

- [Package and field rules](compatibility.md)
- [Code generation](codegen.md)
- [Golden Frame protocol](golden-frames.md)
- [Error model](error-model.md)

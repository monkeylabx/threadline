# Protocol contracts

Threadline uses versioned Protobuf packages as the stable seam between the IM control plane, realtime transport, clients, Agent runtimes, and local connectors. This directory records the rules around that seam; `proto/` remains the executable source of truth.

## Required checks

| Check | Fixed command | Purpose |
| --- | --- | --- |
| Style | `buf lint` | Enforce package, file, enum, RPC, and comment conventions |
| Compatibility | `make proto-breaking` | Reject source and wire changes covered by the merged `WIRE_JSON` policy |
| Golden Frames | `node proto/tools/verify-contracts.mjs` | Verify representative persisted-envelope values, unknown-field canaries, hashes, local-only generation, and output mappings |
| Rust persistence seam | pinned absolute Buf and Cargo + `node proto/tools/verify-rust-envelope-preservation.mjs --offline` | Prove descriptor-backed Rust no-op and known-field mutations preserve both exact field `50000` canaries |
| Release verification | approved clean-env launcher + bundle-absolute Node + `proto/tools/verify-codegen.mjs --mode=verify-only` | Verify schema-v5 source/provenance/authentication records and snapshotted runtime closures, reject protocol stubs, require exact generated file sets, and compile the Java/Kotlin pair |
| Generation commit | approved clean-env launcher + bundle-absolute Node + `proto/tools/verify-codegen.mjs --mode=repository` | Install the already-verified temporary output into the six declared generated directories; clean worktree, repository lock, safe destination paths, and explicit Integration Owner acknowledgement required |

The schema bootstrap has already landed on `main`. T014 no longer has an empty
baseline exception: it must build and lint the current module and run the fixed
breaking command against the merged protocol baseline.

Representative synthetic frames now bind the concrete `ChannelEventEnvelope`
and `RecoveryEnvelope` schemas to known values, hashes, and the exact field
`50000` canary bytes. The descriptor-backed Rust persistence seam and the
five-language bidirectional N-1 matrix are independently reproducible. The
replacement protected-runner formal codegen evidence also passes after the
authoritative error-schema cleanup; Issue #28 may close after final review and
required PR checks pass.

The workspace intentionally has no BSR module name, BSR dependency, or remote generation plugin. Private and air-gapped builds consume only the repository plus the pinned local toolchain.

A bare `node proto/tools/verify-codegen.mjs ...` run is useful for local
diagnosis but is not formal evidence: only the reviewed launcher can
authenticate Node and sanitize preloads before JavaScript starts. See [Code
generation](codegen.md) for the exact bootstrap boundary and manifest-v5
provenance rules. The formal generation plan exactly matches the merged
multi-domain templates; repository installation still requires the separate
Integration Owner acknowledgement and clean-worktree gate.

- [Package and field rules](compatibility.md)
- [Code generation](codegen.md)
- [Golden Frame protocol](golden-frames.md)
- [Error model](error-model.md)

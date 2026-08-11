# Protocol contracts

Threadline uses versioned Protobuf packages as the stable seam between the IM control plane, realtime transport, clients, Agent runtimes, and local connectors. This directory records the rules around that seam; `proto/` remains the executable source of truth.

## Required checks

| Check | Fixed command | Purpose |
| --- | --- | --- |
| Style | `buf lint` | Enforce package, file, enum, RPC, and comment conventions |
| Compatibility | `buf breaking --against '.git#branch=main'` | Reject source and wire changes covered by Buf `FILE` rules |
| Golden Frames | `node proto/tools/verify-contracts.mjs` | Verify persisted-envelope unknown-field canaries, hashes, local-only generation, and output mappings |
| Generation | `buf generate` | Rebuild all five language adapters; Integration Owner only |

T014 is the one-time schema bootstrap. Because its `main` baseline has no `.proto` files, it runs `buf build` instead of claiming a breaking comparison succeeded. Every subsequent contract change must run the fixed breaking command against the merged T014 baseline.

The workspace intentionally has no BSR module name, BSR dependency, or remote generation plugin. Private and air-gapped builds consume only the repository plus the pinned local toolchain.

- [Package and field rules](compatibility.md)
- [Code generation](codegen.md)
- [Golden Frame protocol](golden-frames.md)
- [Error model](error-model.md)

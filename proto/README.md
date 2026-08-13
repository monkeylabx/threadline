# Threadline Protocol

`proto/` is the source of truth for Threadline's cross-platform wire contracts. The stable seam is the versioned Protobuf package; generated Go, TypeScript, Rust, Swift, and Kotlin implementations are adapters and must not leak generator-specific types back into the contract.

## Layout

```text
proto/
  threadline/<domain>/v1/*.proto  versioned product contracts
  golden/v1/                    persisted-envelope compatibility canaries
  tools/verify-contracts.mjs    repository-local structural checks
  toolchain.lock.json           exact generator versions and output paths
```

Package names and paths must agree: `proto/threadline/common/v1/error.proto` declares `threadline.common.v1`. A stable `v1` package is add-only. Incompatible redesigns use a new package such as `v2`; they never reinterpret `v1` bytes.

## Local checks

From the repository root, run the structural checks with the pinned Buf CLI and local Node:

```sh
buf lint
buf breaking --against '.git#branch=main'
node proto/tools/verify-contracts.mjs
```

Formal codegen verification is different: an approved clean-environment launcher must authenticate the reviewed schema-v4 manifest and bundle-absolute Node **before** starting `proto/tools/verify-codegen.mjs --mode=verify-only`. A bare `node` invocation cannot establish that bootstrap fact and is diagnostic only. See [the codegen trust contract](../docs/contracts/codegen.md).

T014 is the one-time bootstrap: its `main` baseline contains no `.proto` files, so Buf has no module to compare. Run `buf build` for this initial change. The breaking command becomes mandatory as soon as T014 is merged and must not be skipped by later contract changes.

Only the Integration Owner may use `--mode=repository`, with the approved launcher, explicit acknowledgement, clean-worktree/repository-lock gate, snapshotted tools, and destination-symlink checks documented in `docs/contracts/codegen.md`. Do not follow a verified run with a separate bare `buf generate`.

Only the Integration Owner may install and commit generated SDK changes through verified repository mode; a separate naked `buf generate` is not an accepted workflow. See [the contract documentation](../docs/contracts/README.md) for the compatibility and generation workflow.

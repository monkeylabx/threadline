# Local code generation

Code generation is deliberately local and deterministic. `buf.gen.yaml` contains no `remote:` entry, and `buf.yaml` contains no BSR module name or dependency.

## Pinned toolchain

The machine-readable pins live in `proto/toolchain.lock.json`. The pinned set is:

| Tool | Version | Role |
| --- | --- | --- |
| Buf CLI | 1.72.0 | Build, lint, breaking check, orchestration |
| protoc | 35.1 | Kotlin built-in generator and descriptor compiler |
| protoc-gen-go | 1.36.11 | Go messages |
| protoc-gen-es | 2.12.0 | TypeScript messages |
| protoc-gen-prost | 0.5.0 | Rust Prost messages |
| protoc-gen-swift | 1.38.1 | Swift messages |

Use exact versions from the lock file; floating `latest`, Git branches, and globally preinstalled unverified versions are not accepted. Tool binaries are provisioned into the controlled integration image or offline bundle and placed on `PATH`. The Kotlin generator is built into the pinned `protoc`, so it has no separate plugin version.

Before generation, record and compare:

```sh
buf --version
protoc --version
protoc-gen-go --version
protoc-gen-es --version
protoc-gen-prost --version
protoc-gen-swift --version
node proto/tools/verify-contracts.mjs
```

## Outputs

| Language | Generated directory |
| --- | --- |
| Go | `services/gen/proto` |
| TypeScript | `packages/generated-ts/src` |
| Rust | `crates/generated-proto/src/generated` |
| Swift | `packages/generated-swift/Sources/ThreadlineGenerated` |
| Kotlin | `packages/generated-kotlin/src/main/kotlin` |

The directories are adapters around the versioned Protobuf seam. Application code should expose domain-level interfaces rather than pass generator-specific reflection objects across module boundaries.

## Ownership workflow

Only the Integration Owner may execute the fixed generation command and commit its output:

```sh
buf lint
buf breaking --against '.git#branch=main'
buf generate
node proto/tools/verify-contracts.mjs
```

For the T014 bootstrap only, replace the breaking line with `buf build` because the pre-T014 `main` branch has no `.proto` baseline. This exception expires when T014 merges.

Generated files are never hand-edited. A contract PR changes `proto/`, fixtures, and contract documentation first. The Integration Owner then regenerates all five outputs in one commit, runs their native builds/tests, records tool versions and schema diff, and rejects any unexplained output drift. Feature worktrees consume the merged generated commit; they do not layer a second generator or handwritten mirror on top.

The current T014 task defines output locations but does not create or edit those shared generated surfaces. Their package manifests and workspace registration remain Integration-owned.

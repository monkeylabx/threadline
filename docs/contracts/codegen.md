# Local code generation

Code generation is deliberately local and deterministic. The merged templates
under `proto/buf.gen.*.yaml` contain no `remote:` entries, and
`proto/buf.yaml` contains no public BSR module name or dependency.

The root `buf.gen.yaml` is the machine-readable union of the five developer
templates. It preserves their managed Go/Java package options, exact output
directories, Connect plugins, and the all-files strategy required by
`protoc-gen-prost-crate`. Both structural verification and the formal verifier
compare this plan as an exact object, so there is no second drifting topology.

## Pinned toolchain

The machine-readable pins live in `proto/toolchain.lock.json`. The pinned set is:

| Tool | Version | Role |
| --- | --- | --- |
| Buf CLI | 1.72.0 | Build, lint, breaking check, orchestration |
| protoc | 35.1 | Kotlin built-in generator and descriptor compiler |
| Temurin JDK | 17.0.19+10 | Eclipse Adoptium Java/Javac compile smoke; must match `toolchains.json` |
| protoc-gen-go | 1.36.11 | Go messages |
| protoc-gen-connect-go | 1.20.0 | Go Connect clients and handlers |
| protoc-gen-es | 2.14.0 | TypeScript messages |
| protoc-gen-prost | 0.5.0 | Rust Prost messages |
| protoc-gen-prost-crate | 0.5.0 | Rust module tree |
| protoc-gen-swift | 1.38.1 | Swift messages |
| protoc-gen-connect-swift | 1.2.3 | Swift Connect clients |
| protoc-gen-connect-kotlin | 0.9.0 | Kotlin Connect clients |
| Git CLI | 2.50.1 | Repository-mode clean-worktree gate only; supplied in the verified release manifest |
| Kotlin compiler | 2.4.10 | Kotlin generated-source compile smoke |
| protobuf-java | 4.35.1 | Java message runtime used by the Kotlin SDK |
| protobuf-kotlin | 4.35.1 | Kotlin DSL runtime used by the Kotlin SDK |
| kotlin-stdlib | 2.4.10 | Explicit Kotlin compile/runtime classpath; never inherited from the compiler process |

The root `toolchains.json` remains the workspace toolchain source of truth. `proto/toolchain.lock.json` is narrower: it locks only contract-generator tools and their runtime artifacts, and its overlapping JDK pin is mechanically checked against the root. Floating `latest`, Git branches, and globally preinstalled unverified versions are not accepted. The Kotlin generator is built into the pinned `protoc`, so it has no separate plugin version.

## Verified offline inputs

The Integration Owner supplies a platform-specific tool bundle plus a manifest. The manifest SHA-256 comes from separately reviewed release metadata; this repository does not claim a signature unless the release process actually supplies and verifies one. Nothing is discovered from the current `PATH`.

Manifest schema 4 records the exact platform/profile, bundle-local source archives or packages, every tool's provenance and invocation, and a complete recursive file/symlink manifest for every runtime closure. A normal source entry is an exact `{kind,file,url,sha256}` record: the bundle-local regular file is hashed, the URL must be HTTPS, and `kind` distinguishes an official binary/package, a source archive, or a protocol-only fixture. A `builder-toolchain` additionally carries machine-checked `authentication`: the exact `static.rust-lang.org` distribution `.sha256` sidecar, the exact pinned `go.dev` release JSON entry, or the macOS Swift installer signature with the `Developer ID Installer: Swift Open Source (V9AUD2URP3)` identity. Other builder authorities require an explicit schema/reviewer update; arbitrary HTTPS labels or self-authored checksum text are rejected. A closure references one or more source names, which lets the JavaScript generator closure account for the main `protoc-gen-es` npm tarball and its complete dependency-tarball set instead of pretending one archive represents the closure. This covers the JDK `lib/modules`, the Node runtime, JavaScript plugin modules, native generators, and their bundle-local files—not just launcher filenames. A repository-mode manifest additionally contains the pinned Git CLI used for the clean-worktree check; verification-only and protocol-smoke manifests omit Git because they cannot update generated surfaces:

```json
{
  "schemaVersion": 4,
  "platform": "darwin-arm64",
  "profile": "release",
  "sources": {
    "protoc-gen-prost-source": {
      "kind": "source-archive",
      "file": "sources/protoc-gen-prost-0.5.0.crate",
      "url": "https://static.crates.io/crates/protoc-gen-prost/protoc-gen-prost-0.5.0.crate",
      "sha256": "..."
    },
    "rust-builder": {
      "kind": "builder-toolchain",
      "file": "sources/rust-1.97.1-aarch64-apple-darwin.tar.xz",
      "url": "https://static.rust-lang.org/dist/rust-1.97.1-aarch64-apple-darwin.tar.xz",
      "sha256": "...",
      "authentication": {
        "kind": "rust-distribution-sha256",
        "file": "sources/rust-1.97.1-aarch64-apple-darwin.tar.xz.sha256",
        "url": "https://static.rust-lang.org/dist/rust-1.97.1-aarch64-apple-darwin.tar.xz.sha256",
        "sha256": "..."
      }
    }
  },
  "closures": {
    "prost-plugin": {
      "root": "closures/prost-plugin",
      "sources": ["protoc-gen-prost-source", "rust-builder"],
      "treeSha256": "...",
      "files": [
        { "path": "bin/protoc-gen-prost", "type": "file", "sha256": "..." }
      ]
    }
  },
  "tools": {
    "protoc-gen-prost": {
      "path": "closures/prost-plugin/bin/protoc-gen-prost",
      "sha256": "...",
      "closure": "prost-plugin",
      "provenance": {
        "kind": "source-built",
        "source": "protoc-gen-prost-source",
        "builders": ["rust-builder"],
        "buildCommand": "cargo build --release --locked --offline -p protoc-gen-prost --bin protoc-gen-prost",
        "reproducibility": "single-build-output-verified"
      },
      "invocation": "native"
    }
  }
}
```

The ellipses are schematic, not accepted values. Provenance is one of four exact forms. `official-binary` and `official-package` reference all applicable source names. `source-built` records one source archive, every authenticated builder archive, the exact build command, and only the evidence claim `single-build-output-verified`; it does **not** call the resulting binary an upstream release or claim reproducible builds. The verifier proves that the reviewed bundle contains those exact inputs and output, and rejects a builder whose pinned checksum metadata does not contain its digest or whose pinned Swift installer identity is invalid. It still does not independently observe the historical build command; the release record must retain the clean-runner build log separately. `protocol-stub` references protocol fixtures and is rejected by both formal modes. An invalid, unverifiable, or incompletely sourced builder cannot be promoted by adding a self-authored attestation.

The verifier rejects missing/extra source, closure, provenance, or tool fields;
a missing bundle-local source archive; digest or platform drift;
overlapping/escaping closures; an unpinned archive; missing/extra tools;
symlink escape; version drift; Java vendor/runtime drift; or Node/JDK drift
from `toolchains.json`. The v4 formal release profile is currently restricted
to the protected `macos-26` Integration runner on `darwin-arm64`, because Swift
source builds require the pinned macOS installer identity; Linux or Windows
formal generation requires a reviewed builder-authentication schema extension
first. Cross-platform protocol-smoke bundles may still use native Windows
`.exe` files, but `.cmd` wrappers are forbidden because they introduce an
unverified command-interpreter dependency. A JavaScript generator is invoked
as `[verified-node, verified-plugin]`, so its `env node` shebang is never used
for version inspection or generation. The Connect Kotlin executable JAR is
invoked as `[verified-java, -jar, verified-plugin]`; native generators and
`protoc` are invoked by verified absolute paths. The exact root plan is then
rewritten in memory with only these verified invocations.

Formal verification has a bootstrap trust boundary outside JavaScript. An approved clean-environment launcher must first authenticate the reviewed manifest digest, verify the bundle's absolute Node executable, clear preload/search-path variables, and only then start that absolute Node. A check inside `verify-codegen.mjs` can reject a suspicious environment after startup, but it cannot prove that `NODE_OPTIONS`, `LD_PRELOAD`, or an equivalent preload did not already execute. Invoking the script with a bare `node`, a shell-discovered launcher, or the current environment is therefore diagnostic only, never formal release evidence.

After startup, every child receives a minimal environment: only verified tool directories on `PATH`, private `HOME`/`TMPDIR`, fixed `LANG`/`LC_ALL`/`TZ`, and explicit Git configuration that disables system/global config and filesystem monitors. The verifier copies every verified closure and the pinned Kotlin artifacts into a private temporary snapshot, re-verifies that snapshot, and executes only snapshot paths, closing the hash-to-execute gap against ordinary bundle mutation. This is a build-pipeline control, not a defense against a hostile same-UID process. Repository mode holds a cooperative repository lock from the initial clean check through installation, rechecks cleanliness immediately before synchronization, rejects symlinked destination ancestors or realpath escape, and preserves rollback backups on failure.

The committed SHA-256 values for the Kotlin compiler classpath and Protobuf/Kotlin runtime JARs live in `proto/toolchain.lock.json`. Each supplied JAR must match both the committed SHA-256 and the recorded Maven Central SHA-1. The compiler directory is an exact set: missing, extra, renamed, symlinked, or changed JARs fail.

Formal release verification requires a `release` manifest; protocol stubs are
rejected. It generates to a temporary directory using the provenance-recorded
plugins, checks the pinned file count and canonical SHA-256 tree for every
merged output surface, checks every source against an accepted generator
signature and real Envelope/service structures, then compiles all generated
Java messages, Kotlin DSL and Connect clients plus a Kotlin consumer. The root
plan and exact output assertions now cover the merged multi-domain protocol.
Replace the two
angle-bracket launcher arguments with values from the approved release runner;
they are trust-root placeholders, not shell commands:

```sh
THREADLINE_PROTO_TOOL_MANIFEST=/reviewed-bundle/proto-tools.json \
THREADLINE_PROTO_TOOL_MANIFEST_SHA256=<digest-from-release-metadata> \
THREADLINE_KOTLIN_COMPILER_DIR=/reviewed-bundle/kotlin-compiler-classpath \
THREADLINE_KOTLIN_STDLIB_JAR=/path/to/kotlin-stdlib-2.4.10.jar \
THREADLINE_PROTOBUF_JAVA_JAR=/path/to/protobuf-java-4.35.1.jar \
THREADLINE_PROTOBUF_KOTLIN_JAR=/path/to/protobuf-kotlin-4.35.1.jar \
THREADLINE_CONNECT_KOTLIN_JAR=/path/to/connect-kotlin-0.9.0.jar \
<approved-clean-env-launcher> <bundle-absolute-node> proto/tools/verify-codegen.mjs --mode=verify-only
```

`--mode=protocol-smoke` is a deliberately weaker harness for exercising the plugin protocol with declared stubs. Its output is labelled `PROTOCOL-SMOKE ONLY` and is never release evidence. `--mode=verify-only` requires non-stub provenance. The separate `verify-generated-envelope-compat.mjs` harness generates and compiles Go, TypeScript, Rust, Swift, and Kotlin adapters, then relays the representative `ChannelEventEnvelope` and `RecoveryEnvelope` in both directions between the current schema and the exact pre-T014 main commit. Every hop mutates a known field and must preserve the exact field-50000 unknown canary. This closes the generated-adapter and N-1 acceptance items, but it is local compatibility evidence rather than the protected-runner formal codegen attestation.

The canonical command and the exact required environment-variable names are
machine-readable in `proto/golden/v1/manifest.json`. Each variable points to a
pinned executable, JDK/SDK, verified Kotlin JAR directory, or the committed
SwiftProtobuf source-archive digest; optional Go cache variables and
`THREADLINE_ENVELOPE_COMPAT_OFFLINE=1` make the same matrix repeatable without
network access after preload.

The Kotlin generator extends Java-generated message classes; it does not
replace them. `proto/buf.gen.kotlin.yaml` therefore emits both
`src/main/java` and `src/main/kotlin` inside the single Kotlin SDK. Its compile
classpath separately names `kotlin-stdlib`, `protobuf-java`, and
`protobuf-kotlin`; it never relies on compiler-process internals leaking into
the compiled SDK. Missing inputs are failures, not skips.

## Outputs

| Language | Generated directory |
| --- | --- |
| Go | `services/gen` |
| TypeScript | `packages/generated-ts/src` |
| Rust | `crates/client-proto/src/generated` |
| Swift | `packages/generated-swift/Sources/ThreadlineProto` |
| Kotlin | Java messages: `packages/generated-kotlin/src/main/java`; Kotlin DSL: `packages/generated-kotlin/src/main/kotlin` |

The directories are adapters around the versioned Protobuf seam. Application code should expose domain-level interfaces rather than pass generator-specific reflection objects across module boundaries.

## Ownership workflow

Only the Integration Owner may execute the fixed generation command and commit its output:

```sh
buf breaking --against '.git#branch=main'
node proto/tools/verify-contracts.mjs
THREADLINE_INTEGRATION_OWNER_GENERATION=I_ACKNOWLEDGE_SHARED_GENERATED_SURFACES \
THREADLINE_PROTO_TOOL_MANIFEST=/reviewed-bundle/proto-tools.json \
THREADLINE_PROTO_TOOL_MANIFEST_SHA256=<digest-from-release-metadata> \
THREADLINE_KOTLIN_COMPILER_DIR=/reviewed-bundle/kotlin-compiler-classpath \
THREADLINE_KOTLIN_STDLIB_JAR=/reviewed-bundle/kotlin-stdlib-2.4.10.jar \
THREADLINE_PROTOBUF_JAVA_JAR=/reviewed-bundle/protobuf-java-4.35.1.jar \
THREADLINE_PROTOBUF_KOTLIN_JAR=/reviewed-bundle/protobuf-kotlin-4.35.1.jar \
THREADLINE_CONNECT_KOTLIN_JAR=/reviewed-bundle/connect-kotlin-0.9.0.jar \
<approved-clean-env-launcher> <bundle-absolute-node> proto/tools/verify-codegen.mjs --mode=repository
```

Repository mode requires a clean worktree and the explicit Integration Owner acknowledgement. The clean check runs through the Git binary and runtime closure already verified from the repository-mode manifest, never a `git` discovered from `PATH`. It first performs the same formal temporary generation and checks as `verify-only`, prints before/after tree digests, then synchronizes only the six declared output directories with per-directory atomic renames and rollback on a handled failure. It never follows with a second naked `buf generate`, so the bytes checked are the bytes installed. `node proto/tools/verify-contracts.mjs` runs fault-injection coverage for lock contention, a failure between backup and installation rename, and destination symlink escape. A process crash or `SIGKILL` can still leave `.git/threadline-codegen-*.lock`; after confirming no codegen process is alive, the Integration Owner may remove that exact lock and rerun from a clean worktree. A process or filesystem failure between directory renames is still recoverable Git worktree state, not a claimed cross-directory transaction; review the printed digests and `git diff` before commit.

The T014 bootstrap exception has expired because the complete protocol baseline
is already on `main`. T014 must run the normal breaking check. The T014 feature
worktree itself does not run repository mode or edit generated SDKs.

Generated files are never hand-edited. A contract PR changes `proto/`, fixtures, and contract documentation first. The Integration Owner then regenerates all five outputs in one commit, runs their native builds/tests, records tool versions and schema diff, and rejects any unexplained output drift. Feature worktrees consume the merged generated commit; they do not layer a second generator or handwritten mirror on top.

The current T014 task defines output locations and provides the verified generation mechanism, but does not create or edit shared generated surfaces. Their package manifests, workspace registration, native builds, and committed generation remain Integration-owned.

## T014 acceptance blocker

The merged schema now contains `ChannelEventEnvelope` and `RecoveryEnvelope`.
Representative synthetic frames now bind both messages to known semantic
values, exact source/frame digests, and the family field-50000 canary. The
five-language unknown-field decode/mutate/re-encode and bidirectional N-1
matrix now pass for both representative frames. The formal plan now exactly
matches the merged templates. The remaining blocker is protected-runner formal
evidence.
`proto/golden/v1/manifest.json` records these as `issueMayClose: false`.

Issue #28 must remain open unless one of two things happens: protected-runner
formal evidence is added; or Contracts and Product
explicitly approve moving exact acceptance items elsewhere and update the
Issue. Merely naming a follow-up task in this document is not approval.

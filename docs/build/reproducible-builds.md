# Reproducible build runbook

Status: T009 baseline

The machine-readable source of truth is `toolchains.json`. Native manifests repeat only the values required by their own ecosystem, and `node scripts/toolchain.mjs verify` rejects drift between them.

## Bootstrap

All dependency installation must start from a clean checkout and committed lockfiles. Do not use `latest` or a globally installed package manager as an implicit input.

```text
nvm install 24.19.0
nvm use 24.19.0
npm install --global corepack@0.35.0
corepack enable
corepack install --global pnpm@11.20.0
pnpm install --frozen-lockfile
rustup show
```

Install Go 1.26.5 from the official archive or through the CI setup action. Set `GOTOOLCHAIN=local` in CI so the Go command cannot silently download a different toolchain. Rustup reads `rust-toolchain.toml` and installs Rust 1.97.1 with rustfmt and clippy.

Database code generation uses sqlc 1.31.1 and `github.com/jackc/pgx/v5`
5.10.0. Download sqlc only from the release asset named in
`toolchains.json`, verify the recorded SHA-256 before extraction, and run it
with `node scripts/toolchain.mjs doctor --scope=database`. Do not substitute a
Homebrew, Snap, Docker, or `go install ...@latest` binary as release evidence.
P03-01A owns generated-query determinism and migration round trips; this
toolchain baseline does not create or alter a database.

The Rust SQLite API boundary is pinned to `rusqlite` 0.40.2 with default
features disabled. P05-01B-1 selects its
`bundled-sqlcipher-vendored-openssl` feature as the source-built encryption
candidate in the ordinary `client-core` dependency graph. The resulting lock
graph contains `libsqlite3-sys` 0.38.2 with bundled SQLCipher 4.14.0 Community
Edition and a vendored OpenSSL source build. `rusqlite` and `libsqlite3-sys` are
MIT licensed, the bundled SQLCipher source carries its BSD-style license, and
OpenSSL is Apache-2.0 licensed. These are free/open-source components: no
commercial SQLCipher artifact, account, download, or service is required.

The workspace pin keeps `rusqlite` default features disabled, while the ordinary
`client-core` dependency explicitly enables only the source-built SQLCipher
candidate. Therefore a non-test `cargo tree -p threadline-client-core -e=no-dev`
does not select a plain system-SQLite backend. This is a dependency-graph
invariant, not production-provider admission: `client-core` still exposes no
database constructor or key adapter, and this task does not decide OS key
lifecycle. The evidence harness remains test-only. Locked builds compile the
SQLCipher amalgamation and OpenSSL locally through the crates.io source archives
and the checked-in Cargo lockfile.

`libsqlite3-sys` retains `pkg-config` and `vcpkg` as portable discovery build
dependencies, but its enabled `bundled-sqlcipher` feature selects the bundled
build branch rather than either discovery path. Reproducible builds must leave
`LIBSQLITE3_SYS_USE_PKG_CONFIG` unset (or set it to `0`); a nonzero override is
forbidden because it bypasses the selected source backend. The executable
`cipher_version` assertion below also fails if a plain system library is linked.

| Locked input | Source | License used for distribution |
| --- | --- | --- |
| `rusqlite` 0.40.2 | `crates.io` archive from [`rusqlite/rusqlite`](https://github.com/rusqlite/rusqlite) | MIT |
| `libsqlite3-sys` 0.38.2, including SQLCipher 4.14.0 Community amalgamation | `crates.io` archive from [`rusqlite/rusqlite`](https://github.com/rusqlite/rusqlite); embedded source from [`sqlcipher/sqlcipher`](https://github.com/sqlcipher/sqlcipher) | MIT wrapper; bundled SQLCipher BSD-style license |
| `openssl-sys` 0.9.117 | `crates.io` archive from [`rust-openssl/rust-openssl`](https://github.com/rust-openssl/rust-openssl) | MIT |
| `openssl-src` 300.6.1+3.6.3, including OpenSSL 3.6.3 | `crates.io` archive from [`alexcrichton/openssl-src-rs`](https://github.com/alexcrichton/openssl-src-rs); embedded source from [`openssl/openssl`](https://github.com/openssl/openssl) | MIT/Apache-2.0 wrapper; OpenSSL Apache-2.0 |
| `getrandom` 0.3.4 (test key/canary generation only) | `crates.io` archive from [`rust-random/getrandom`](https://github.com/rust-random/getrandom) | MIT OR Apache-2.0 |

Cargo verifies each archive against `Cargo.lock`. The source build requires the
pinned Rust toolchain plus a supported native C compiler, Perl, and the host
make tool selected by `openssl-src`; it does not download a prebuilt database or
cryptographic library. The selected Cargo feature enables the bundled SQLCipher
amalgamation and vendored OpenSSL source explicitly.

Run the candidate evidence with:

```text
cargo test -p threadline-client-core --test sqlcipher_backend --locked
```

The test obtains a fresh database key and fixture canary from the OS CSPRNG,
confirms the linked `PRAGMA cipher_version`, performs a byte-exact Golden
Envelope keyed reopen, scans the live DB/WAL/SHM family for the SQLite header,
fixture canary, and key, and confirms empty/wrong keys fail without rewriting
any member of the live DB/WAL/SHM family. A correct-key reopen then verifies the
schema objects, `user_version`, migration ledger, and Golden Envelope remained
unchanged. Diagnostics intentionally omit keys, canaries, complete database
paths, and reusable secrets.

The P05-01B-1 candidate evidence was run locally on `aarch64-apple-darwin`
(Darwin 25.4.0) with Rust 1.97.1, Apple clang 21.0.0, Perl 5.34.1, and GNU Make
3.81. P05-01B-2A adds one explicit evidence step to each of three existing
protected `build.yml` jobs, using their standard pinned runner labels and the
same locked command:

| Host evidence | Runner | Protected job | Run binding |
| --- | --- | --- | --- |
| Ubuntu | `ubuntu-24.04` | `workspace-linux` | `NOT RUN` — populate the hosted run ID and job URL after Integration triggers this commit |
| Windows | `windows-2025` | `desktop-windows` | `NOT RUN` — populate the hosted run ID and job URL after Integration triggers this commit |
| macOS | `macos-26` | `apple` | `NOT RUN` — populate the hosted run ID and job URL after Integration triggers this commit |

The existing jobs retain the workflow's read-only `contents` permission and
pinned Actions. They now have 45-minute bounds for `workspace-linux` and both
`desktop` matrix hosts, and a 60-minute bound for `apple`. Each dedicated
SQLCipher step sets `LIBSQLITE3_SYS_USE_PKG_CONFIG=0` and uploads no artifact.
Runtime database keys, canaries, database paths, and DB/WAL/SHM files are not
retained; standard job logs contain only the non-secret test name and diagnostics
designed not to expose those values. The three run bindings above must reference
one immutable run of the reviewed commit before P05-01B-2A can be accepted.

This remains desktop-host candidate evidence, not production-provider or
release-platform admission. iOS and Android runtime/simulator/physical-device
SQLCipher evidence, OS Secure Storage and key lifecycle, the production
adapter, P05-01, and M0/G0 remain `NOT RUN` or out of scope.

Android uses the committed Gradle Wrapper, Temurin 17.0.19+10, `compileSdk = 37` with SDK package `platforms;android-37.0`, Build Tools 36.0.0, and NDK 28.2.13676358. Apple builds use `/Applications/Xcode_26.6.app/Contents/Developer`, Xcode build 17F113, and its bundled Swift 6.3; do not mix a swift.org toolchain into Apple builds.

Android dependency locking runs in strict mode. When an Android dependency changes intentionally, regenerate `apps/android/gradle.lockfile` with `./gradlew :apps:android:assembleDebug :apps:android:testDebugUnitTest :apps:android:lintDebug --write-locks --no-daemon`, review the complete lock diff, and commit it with the manifest change. Normal CI commands must never use `--write-locks`.

## Commands by target

| Target | Exact verification | Build and test |
| --- | --- | --- |
| Workspace / Linux services | `node scripts/toolchain.mjs doctor --scope=workspace` | TypeScript package scripts; `cargo test --workspace --exclude threadline-desktop-host --locked`; `go test ./...` and `go build ./...` in `services/` |
| PostgreSQL code generation | `node scripts/toolchain.mjs doctor --scope=database` | `sqlc generate` from the reviewed `db/sqlc.yaml`; a second generation must produce no diff |
| macOS Desktop | `node scripts/toolchain.mjs doctor --scope=desktop,apple` | `pnpm --filter @threadline/desktop build:native` |
| Windows Desktop | `node scripts/toolchain.mjs doctor --scope=desktop` | `pnpm --filter @threadline/desktop build:native` in an MSVC runner |
| Linux Desktop | same desktop doctor after installing Tauri WebView prerequisites | `pnpm --filter @threadline/desktop build:native` |
| iOS | `node scripts/toolchain.mjs doctor --scope=apple` | `swift test --package-path apps/ios`; unsigned iOS Simulator `xcodebuild` command from `.github/workflows/build.yml` |
| Android | `node scripts/toolchain.mjs doctor --scope=android` | `./gradlew :apps:android:assembleDebug :apps:android:testDebugUnitTest :apps:android:lintDebug --no-daemon` |

The Desktop web page and icon are build-only placeholders. They establish a real Tauri host but do not define product design, navigation, IPC commands, permissions, or business behavior.

## Native bridge boundary

T009 pins Xcode, the Android NDK, Rust, SwiftPM, Gradle, and target triples. CI cross-compiles the version-only `client-ffi` facade for one iOS Simulator and one Android ABI as a compiler/linker smoke test.

T010 owns generated Swift/Kotlin bindings, XCFramework/AAR packaging, device and simulator architecture coverage, async/cancellation/error behavior, crash/resume, memory ownership, and real-device evidence. A successful T009 cross-compile must not be reported as a successful Native Bridge Spike.

## CI and cache boundary

The build workflow uses explicit runner labels and full Action commit SHAs. Every job verifies actual tool versions before building. PR jobs are unsigned and have only `contents: read` permission.

Cache is disposable performance state. Cache misses must still build successfully. Never cache `node_modules`, credentials, signing material, release bundles, provisioning profiles, keystores, Swift DerivedData across Xcode versions, or Cargo `target` directories across target triples.

Gradle is the only baseline cache enabled initially and is owned by `gradle/actions/setup-gradle`. The other ecosystems can add caches only with OS, architecture, exact toolchain, target, features, and lockfile in the key.

## Signing boundary

T009 does not create signing jobs or secrets. P00-06 must place Apple, Android, Windows, and Tauri updater signing in separate protected release environments with required reviewers. Fork and pull-request jobs never receive those environments.

The placeholder secret names and destruction requirements are documented in `toolchain-research.md`. No certificate, private key, provisioning profile, keystore, password, token, or base64-encoded equivalent belongs in the repository, cache, build log, or unsigned artifact.

## Failure diagnosis and upgrades

1. Run `npm run toolchain:verify` to distinguish repository pin drift from a missing local tool.
2. Run the narrow `doctor:<scope>` command and compare its `missing` or `mismatch` output with `toolchains.json`.
3. Reproduce with cache disabled before treating a cache miss as a product failure.
4. Record runner image, OS build, tool versions, target triple, Xcode/SDK or Android SDK package list, and the first failing command. Do not include environment values or user data.

Toolchain upgrades use a dedicated Integration PR. It must cite official release and compatibility sources, update all native pins, Action SHAs and checksums, regenerate lockfiles, pass the complete matrix, and name the tested rollback version. Scheduled drift checks may report an update, but must never edit or push version files automatically.

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

Android uses the committed Gradle Wrapper, Temurin 17.0.19+10, `compileSdk = 37` with SDK package `platforms;android-37.0`, Build Tools 36.0.0, and NDK 28.2.13676358. Apple builds use `/Applications/Xcode_26.6.app/Contents/Developer`, Xcode build 17F113, and its bundled Swift 6.3; do not mix a swift.org toolchain into Apple builds.

Android dependency locking runs in strict mode. When an Android dependency changes intentionally, regenerate `apps/android/gradle.lockfile` with `./gradlew :apps:android:dependencies --write-locks --no-daemon`, review the complete lock diff, and commit it with the manifest change. Normal CI commands must never use `--write-locks`.

## Commands by target

| Target | Exact verification | Build and test |
| --- | --- | --- |
| Workspace / Linux services | `node scripts/toolchain.mjs doctor --scope=workspace` | TypeScript package scripts; `cargo test --workspace --exclude threadline-desktop-host --locked`; `go test ./...` and `go build ./...` in `services/` |
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

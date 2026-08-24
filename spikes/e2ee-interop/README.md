# T011 E2EE interoperability spike

This directory contains a disposable feasibility harness for ADR-0003. It is
not a production cryptography module.

- `rust/` runs OpenMLS 0.8.1 against the `tl-mls-1` ciphersuite and exercises
  KeyPackages, epoch progression, out-of-order commits, offline join, replay,
  corruption containment, and device revocation. It also runs the RFC 9420
  Known Answer Tests (`tests/rfc9420_kat.rs`), a crash/resume and key-erasure
  suite (`tests/persistence_resume.rs`), and an `#[ignore]`d group-size
  performance profile (`tests/perf_profile.rs`).
- `interop-mls-rs/` drives the same `tl-mls-1` group from both OpenMLS and
  mls-rs 0.55.3, exchanging only serialized `MLSMessage` bytes. This is the
  independent-implementation evidence ADR-0003 asks for, and it pins the two
  policies whose library defaults turned out not to be interoperable.
- `vectors/rfc9420/` holds the upstream IETF Known Answer Test vectors, filtered
  to cipher suite 1; see its README for provenance and checksums.
- `swift/` and `kotlin/` independently parse the public semantic Golden Vector,
  recompute the recovery-envelope binding digest, and validate stable errors.
- `sbom/cargo.cdx.json` is a deterministic CycloneDX inventory generated from
  the locked Rust dependency graph by `generate-sbom.mjs`.
- `verify.sh` runs all three host harnesses when the pinned repository
  toolchains are available.

The canonical vector is `../../test/crypto/e2ee-interop-v1.vector`. It contains
only public metadata, hashes, and synthetic opaque values. No message content,
private key, recovery key, or live ciphertext is persisted by the harness.

Run the individual checks from the repository root:

```sh
cargo test --manifest-path spikes/e2ee-interop/rust/Cargo.toml --locked
cargo test --manifest-path spikes/e2ee-interop/interop-mls-rs/Cargo.toml --locked
cargo test --manifest-path spikes/e2ee-interop/rust/Cargo.toml --locked --release \
  -- --ignored --nocapture
swift run --package-path spikes/e2ee-interop/swift T011SwiftHarness \
  test/crypto/e2ee-interop-v1.vector
./apps/android/gradlew -p spikes/e2ee-interop/kotlin run \
  --args="$PWD/test/crypto/e2ee-interop-v1.vector"
node spikes/e2ee-interop/generate-sbom.mjs
```

See `docs/spikes/e2ee-interop.md` for the first round of evidence, and
`docs/spikes/e2ee-library-selection.md` plus `docs/adr/0004-e2ee-crypto-library-selection.md`
for the library-selection evidence, the pinned `tl-mls-1` policies, and the
remaining P00-08 gaps.

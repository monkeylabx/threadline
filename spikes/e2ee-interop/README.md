# T011 E2EE interoperability spike

This directory contains a disposable feasibility harness for ADR-0003. It is
not a production cryptography module.

- `rust/` runs OpenMLS 0.8.1 against the `tl-mls-1` ciphersuite and exercises
  KeyPackages, epoch progression, out-of-order commits, offline join, replay,
  corruption containment, and device revocation.
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
swift run --package-path spikes/e2ee-interop/swift T011SwiftHarness \
  test/crypto/e2ee-interop-v1.vector
./gradlew -p spikes/e2ee-interop/kotlin run \
  --args="$PWD/test/crypto/e2ee-interop-v1.vector"
node spikes/e2ee-interop/generate-sbom.mjs
```

See `docs/spikes/e2ee-interop.md` for evidence, limitations, and the production
readiness decision.

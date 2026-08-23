# Crypto semantic fixtures

`e2ee-interop-v1.vector` is the immutable T011 semantic-spike vector. Its
machine-readable companion manifest is
`e2ee-interop-v1.manifest.json`; the repository verifier checks the exact file
digest, complete key set, classification, frozen protocol/profile values, and
the evidence boundary.

The reproducible source is the exact Git object recorded by the manifest. The
verifier independently pins that historical commit, the vector SHA-256 and the
complete key schema, scans both the working-tree family and every reachable
historical vector blob for prohibited material, and rejects manifest drift.

The vector contains public metadata, stable error names, and irreversible
digests only. It contains no plaintext, private key, credential, token, or
production export. It is not a persisted Protobuf frame and does not replace
the T014/T019 generated-adapter N-1 evidence.

Run:

```sh
node test/crypto/verify-e2ee-interop-fixture.mjs
```

Passing this verifier preserves the T011 spike result; it does not admit
OpenMLS 0.8.1 or any Crypto Provider to production, prove KMS/HSM recovery,
accept ADR-0003/ADR-0004, or turn physical-device evidence into `PASS`.

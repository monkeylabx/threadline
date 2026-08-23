# Message and sync protocol fixtures

These fixtures are synthetic contract evidence for T015. They contain no user
messages, credentials, real tenant identifiers, keys, signatures, or production
telemetry. Values that look like ciphertext, hashes, and signatures are short
fixed byte patterns used only to exercise the protocol boundary.

`scenarios.json` describes the observable result of message submission and
cursor recovery. The verifier deliberately treats `applicationCiphertext` as
opaque bytes: authorization, tenant, Group, Epoch, Device signature metadata,
idempotency, ordering, and durability are decided without parsing plaintext.

Run the fixed check from the repository root:

```bash
node proto/tools/verify-message-sync-contracts.mjs
```

The unknown-extension case reuses T014's representative
`ChannelEventEnvelope` Golden Frame and its exact field `50000` canary. It does
not create a second persisted-envelope format. T019-owned Device, KeyPackage,
Epoch transition, History Sharing, and Recovery Envelope semantics remain
outside these fixtures.

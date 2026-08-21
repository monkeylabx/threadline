# Golden Frame protocol

Golden Frames protect the serialized bytes and unknown-field behavior of persisted Protobuf messages. They are compatibility evidence, not cryptographic Known Answer Tests and never contain production identifiers, plaintext, credentials, keys, or real ciphertext.

## Canary

Every persisted message reserves field `50000`. The field is absent from the schema and is injected as a length-delimited unknown field in compatibility tests. The repository seeds two versioned canaries:

- `proto/golden/v1/ciphertext-envelope.canary.hex`
- `proto/golden/v1/crypto-envelope.canary.hex`

Their names identify the contract families; they do not define the concrete
Envelope fields. `manifest.json` pins the payload and SHA-256 of each raw
canary.

T014 currently provides only the executable, schema-independent canary
baseline: canonical bytes, field `50000`, payload, and digest are checked now.
The merged protocol now defines `threadline.message.v1.ChannelEventEnvelope`
and `threadline.crypto.v1.RecoveryEnvelope`, but representative frames for
those messages and cross-language decode/re-encode and N-1 evidence are still
missing. Issue #28 therefore remains open. The Integration Owner must run every
concrete historical frame through every generated adapter before committing
generated SDKs; failure remains a contract gate.

## Test contract

For each concrete persisted Envelope, its owner must add a canonical binary/hex
frame containing representative known fields followed by the appropriate
canary. Each generated-language adapter must prove:

1. decode succeeds without knowing field `50000`;
2. known fields have the expected semantic values;
3. a no-op decode/encode retains the canary field and payload;
4. mutation of one known field retains the canary;
5. an N-1 reader can consume current additive frames, and the current reader can consume N-1 frames;
6. malformed, cross-Tenant, cross-Group, unknown Crypto Profile, and rollback cases fail closed where applicable.

The encoded order of ordinary fields is not a general protocol guarantee. Tests compare semantic values and explicit canary survival; an exact byte comparison is used only where the owning contract declares canonical serialization.

## Breaking strategy

- Additive fields extend the existing frame set and retain all previous frames.
- Retired fields become `reserved`; their old frames remain readable for the documented retention window.
- A package-major change adds parallel fixtures and an explicit migration/rollback path. It does not rewrite or reinterpret old bytes in place.
- A generator or runtime upgrade runs every historical frame in all five languages before generated code can be updated.
- Failure to retain an unknown field blocks persistence writes from that adapter, even if Buf's schema-level breaking check passes.

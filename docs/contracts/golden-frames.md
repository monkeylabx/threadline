# Golden Frame protocol

Golden Frames protect the serialized bytes and unknown-field behavior of persisted Protobuf messages. They are compatibility evidence, not cryptographic Known Answer Tests and never contain production identifiers, plaintext, credentials, keys, or real ciphertext.

## Canary

Every persisted message reserves field `50000`. The field is absent from the schema and is injected as a length-delimited unknown field in compatibility tests. The repository seeds two versioned canaries:

- `proto/golden/v1/ciphertext-envelope.canary.hex`
- `proto/golden/v1/crypto-envelope.canary.hex`

Their names identify the contract families; they do not define the concrete Envelope fields assigned to T015 and T019. `manifest.json` pins the payload and SHA-256 of each raw frame.

T014 currently provides only the executable, schema-independent canary baseline: canonical bytes, field `50000`, payload, and digest are checked now. Concrete Ciphertext/Crypto messages do not yet exist, so this does not satisfy Issue #28's concrete Envelope Golden Frame acceptance item and does not claim cross-language decode/re-encode or N-1 success. Issue #28 remains open until those items exist, or Contracts and Product explicitly approve and record an Issue scope change moving them to T015/T019. The Integration Owner must run every concrete historical frame through every generated adapter before committing generated SDKs; failure remains a contract gate.

## Test contract

When a concrete persisted Envelope is introduced, its owner must add a canonical binary/hex frame containing representative known fields followed by the appropriate canary. Each generated-language adapter must prove:

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

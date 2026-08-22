# Golden Frame protocol

Golden Frames protect the serialized bytes and unknown-field behavior of persisted Protobuf messages. They are compatibility evidence, not cryptographic Known Answer Tests and never contain production identifiers, plaintext, credentials, keys, or real ciphertext.

## Canary

Every persisted message reserves field `50000`. The field is absent from the schema and is injected as a length-delimited unknown field in compatibility tests. The repository contains two versioned canaries and two representative frames:

- `proto/golden/v1/ciphertext-envelope.canary.hex`
- `proto/golden/v1/crypto-envelope.canary.hex`
- `proto/golden/v1/channel-event-envelope.golden.hex`
- `proto/golden/v1/recovery-envelope.golden.hex`

The canary names identify contract families; they do not define concrete
Envelope fields. Each representative frame is generated from its adjacent
synthetic `.golden.json` source using the real schema, then receives the exact
family canary bytes as its final unknown field. `manifest.json` pins every
source and frame SHA-256, its schema file, known-field set, and canary.

`verify-golden-frames.mjs` decodes the wire format without a generated adapter,
checks all representative known values and nested messages, proves the exact
field `50000` bytes survived, and rejects any byte or source drift. The fixtures
are synthetic compatibility data: they contain no production identifier,
plaintext, credential, key, or real ciphertext.

Regeneration requires an absolute Buf 1.72.0 executable; the script does not
discover one from `PATH`:

```sh
THREADLINE_BUF=/absolute/path/to/pinned/buf \
  node proto/tools/generate-golden-frames.mjs --check
```

Only a reviewed contract change uses `--write`. Issue #28 remains open because
repository-level wire validation is not the required five generated-language
decode/mutate/re-encode evidence, N-1 evidence, or protected-runner formal
codegen record. The Integration Owner must run every historical frame through
every generated adapter before committing generated SDKs; failure remains a
contract gate.

## Test contract

For each concrete persisted Envelope, its owner adds a canonical binary/hex
frame containing representative known fields followed by the appropriate
canary. The repository verifier proves the source, schema reservation, known
wire values, digest, and canary. Each generated-language adapter must still
prove:

1. decode succeeds without knowing field `50000`;
2. known fields have the expected semantic values;
3. a no-op decode/encode retains the canary field and payload;
4. mutation of one known field retains the canary;
5. an N-1 reader can consume current additive frames, and the current reader can consume N-1 frames;
6. malformed, cross-Tenant, cross-Group, unknown Crypto Profile, and rollback cases fail closed where applicable.

Rust now has a selected and tested persistence seam: descriptor-backed
`prost-reflect::DynamicMessage`. The configured `prost` generated structs still
do not retain arbitrary unknown message fields and remain forbidden as a
decode/mutate/re-encode persistence path. The exact boundary, pins, connected
preload, and offline verification command are recorded in `compatibility.md`.
This result closes the Rust decision, not the remaining generated-adapter or
N-1 matrix.

The encoded order of ordinary fields is not a general protocol guarantee. Tests compare semantic values and explicit canary survival; an exact byte comparison is used only where the owning contract declares canonical serialization.

## Breaking strategy

- Additive fields extend the existing frame set and retain all previous frames.
- Retired fields become `reserved`; their old frames remain readable for the documented retention window.
- A package-major change adds parallel fixtures and an explicit migration/rollback path. It does not rewrite or reinterpret old bytes in place.
- A generator or runtime upgrade runs every historical frame in all five languages before generated code can be updated.
- Failure to retain an unknown field blocks persistence writes from that adapter, even if Buf's schema-level breaking check passes.

# Protobuf compatibility rules

## Packages and files

- Product packages use `threadline.<domain>.v<major>` and matching paths under `proto/threadline/<domain>/v<major>/`.
- Stable packages use numeric suffixes such as `v1`. Alpha/beta package suffixes are not accepted for persisted or public contracts.
- A package major version is a wire contract, not a release train or generator version. Generator upgrades cannot change its meaning.
- Each file declares the fixed Go, Java/Kotlin, and Swift naming options needed for deterministic cross-platform output.

## Change classes

An additive change may merge into a stable package only when old readers safely ignore it and new readers define the absent-field behavior. Examples are a new optional field, a new message, or a new RPC whose authorization has been specified.

A breaking change requires a new package major version and a migration plan. Breaking changes include changing a tag number or field type, reusing a deleted tag/name, changing a field's meaning or authorization boundary, moving a type between files, and making an absent value invalid.

Fields are never silently removed. If a field is retired, its number and name remain `reserved` in the same message. Enum zero values end in `_UNSPECIFIED`; clients must handle unknown enum and error-code values without crashing or downgrading.

`proto/buf.yaml` enforces Buf's `WIRE_JSON` category for the merged protocol
baseline. The fixed repository command is:

```sh
make proto-breaking
```

The check supplements design review. It cannot detect semantic reinterpretation, authorization expansion, secret exposure, or unsafe absent-field defaults.

The bootstrap exception has expired: `main` now contains the complete M0
protocol baseline under `proto/threadline/`. A missing, skipped, or empty
breaking baseline is a failure. The lower-level command executed by the
repository wrapper is `buf breaking --against '.git#branch=main'` from the
`proto/` module.

## Persistent data

Any Protobuf message stored in PostgreSQL, SQLite, object storage, a local outbox, or an event log is a persisted contract. It must:

1. reserve field `50000` for the repository Golden Frame unknown-field canary;
2. preserve unknown fields through decode, mutation, and re-encode paths;
3. add a canonical fixture containing representative known fields plus the matching canary bytes;
4. pass N-1 read/current-write and current-read/N-1-write tests in every consuming language;
5. document storage migration and rollback behavior before a package-major change.

Ciphertext, MLS/application crypto, history, and recovery envelopes are
persisted contracts. `ChannelEventEnvelope` and `RecoveryEnvelope` now have
representative synthetic frames containing known fields plus the exact unknown
field `50000` canary. The remaining T014 work is generated-adapter evidence,
not schema invention: five-language unknown-field preservation and N-1
read/write results must be recorded against those exact messages.

The current T014 branch proves schema binding, representative wire values,
source/frame digests, and library-independent canary preservation. It does not
claim that a generated adapter retains unknown fields. The Integration Owner
must record five-language decode/mutate/re-encode and N-1 evidence before the
generated SDK update can merge. Assigning that missing acceptance work
elsewhere requires explicit Contracts and Product approval plus an Issue
update.

### Rust persistence blocker

The generated Rust surface currently uses `prost`. The upstream project
documents preservation of unknown enum values, not unknown message fields, and
its generated-message unknown-field implementation is still an open change:

- [`prost` capabilities](https://github.com/tokio-rs/prost#readme)
- [upstream unknown-field support PR](https://github.com/tokio-rs/prost/pull/1340)
- [`prost-reflect::DynamicMessage`](https://docs.rs/prost-reflect/latest/prost_reflect/struct.DynamicMessage.html), which explicitly preserves unknown fields

Consequently, decoding a persisted Envelope directly into a generated `prost`
struct and re-encoding it is not an admissible persistence path. T014 selects
descriptor-backed `prost-reflect::DynamicMessage` as the Rust persistence
boundary. The generated `prost` type may be a transient typed view, but it is
never the source of bytes written back to SQLite, an Outbox, an event log, or
object storage.

The boundary has four invariants:

1. the descriptor set is built from the exact checked-in schema with pinned Buf;
2. the original `DynamicMessage` remains authoritative for all writes;
3. a known-field change is applied through a descriptor-checked, contract-specific patch to that same dynamic message; and
4. JSON conversion or transcoding a generated struct back into a fresh dynamic message is forbidden because either path can discard unknown wire material.

The isolated harness pins Cargo 1.97.1, `prost` 0.14.1, and
`prost-reflect` 0.16.5. It proves both representative frames preserve the exact
field `50000` bytes through a no-op round trip and a type-checked known-string
mutation, while a type-confused mutation fails closed:

```sh
THREADLINE_BUF=/absolute/path/to/buf-1.72.0 \
THREADLINE_CARGO=/absolute/path/to/cargo-1.97.1 \
  node proto/tools/verify-rust-envelope-preservation.mjs --connected

THREADLINE_BUF=/absolute/path/to/buf-1.72.0 \
THREADLINE_CARGO=/absolute/path/to/cargo-1.97.1 \
  node proto/tools/verify-rust-envelope-preservation.mjs --offline
```

This closes the Rust seam decision only. The five-language Gate still requires
the other generated adapters, the Rust client owner to implement the selected
boundary in its owned path, and the full N-1 matrix before persistence writes
may ship.

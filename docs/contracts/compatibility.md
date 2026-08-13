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

`buf.yaml` enforces Buf's conservative `FILE` category. The fixed baseline command is:

```sh
buf breaking --against '.git#branch=main'
```

The check supplements design review. It cannot detect semantic reinterpretation, authorization expansion, secret exposure, or unsafe absent-field defaults.

The only bootstrap exception is T014 itself: the pre-T014 `main` tree contains no Protobuf module, so Buf correctly reports that there is nothing to compare. T014 must pass `buf build`; from the first follow-up contract onward, a missing or skipped breaking baseline is a failure.

## Persistent data

Any Protobuf message stored in PostgreSQL, SQLite, object storage, a local outbox, or an event log is a persisted contract. It must:

1. reserve field `50000` for the repository Golden Frame unknown-field canary;
2. preserve unknown fields through decode, mutation, and re-encode paths;
3. add a canonical fixture containing representative known fields plus the matching canary bytes;
4. pass N-1 read/current-write and current-read/N-1-write tests in every consuming language;
5. document storage migration and rollback behavior before a package-major change.

Ciphertext, MLS/application crypto, history, and recovery envelopes are persisted contracts. T015 and T019 must add their concrete canonical frames when they define those messages; T014 deliberately does not guess their business or cryptographic fields.

Accordingly, T014 proves only the library-independent field-50000 canary encoding and manifest integrity. This is not yet a pass for Issue #28's concrete Envelope Golden Frame acceptance item. Cross-language unknown-field preservation and N-1 evidence become executable only when concrete message schemas and representative known fields exist; the Integration Owner must record that evidence before their generated SDK update can merge. Assigning that missing acceptance work to T015/T019 requires explicit Contracts and Product approval plus an Issue update.

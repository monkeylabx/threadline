# Threadline test fixture policy

Status: T017 Final

Owner: Quality / SDET; fixture family owners are listed below

Test plan: [Private Enterprise v1.0 test plan](../../docs/quality/test-plan.md)

## Purpose

`test/fixtures/` is the registry and policy boundary for versioned cross-workstream test inputs. A fixture represents public contract data, a deterministic state transition, an expected stable result, or an irreversible digest. It is not a copy of a production Tenant, a plaintext conversation, an Agent Session, a credential store, or a key archive.

Contract owners have added the authoritative message and crypto contract
families under `test/fixtures/proto/` after T014/T015/T019 froze their schemas,
error semantics, formal code generation and Golden/N-1 workflow. This registry
defines the shared policy and does not duplicate or reinterpret those fixtures.

## Non-negotiable data rules

Committed fixtures must never contain:

- real customer, employee, device, repository, model, endpoint or infrastructure identifiers;
- message, file, Artifact, search query, Context or Prompt plaintext, even when synthetic;
- a usable Token, Cookie, password, credential, signing key, Device key, MLS/Channel/Epoch/Content/History key, Recovery key, Route/Capability Grant or production-shaped secret;
- live Ciphertext, Recovery output, production log, database page, SQLite file, crash dump, packet capture, screenshot or support bundle;
- an absolute local path, user name, e-mail, phone number, notification content or personal device identifier.

Committed fixtures may contain only:

- namespaced synthetic IDs whose format cannot be confused with production IDs;
- public enum/status/error values, sequence numbers, versions, lengths and policy decisions;
- deterministic opaque test bytes and irreversible digests with no recoverable source content;
- public standards Known Answer Test material that is explicitly classified `public_test_material`, cites its specification and was never used as a real credential;
- malformed bytes created solely to exercise parsers, with a manifest that proves they contain no encoded C3/C4 content.

Synthetic does not automatically mean safe. A synthetic plaintext message, Prompt, private key or bearer Token is still forbidden as a persisted fixture because it trains code and evidence pipelines to accept the wrong data shape.

## Ephemeral C3/C4 inputs

E2E and leakage tests sometimes need content canaries. The test runner generates them at run time from an approved generator and keeps raw values only in process memory or an encrypted, run-scoped tmpfs. They are never committed, cached, uploaded as CI artifacts, printed, included in command arguments, or copied into a persistent test report.

Public standards KATs and purely `synthetic_opaque` parser/state-machine inputs may use a recorded deterministic seed. C3/C4 canaries, test private keys, nonces and any sensitive or recoverable fuzz input must instead use a runtime CSPRNG and a fresh per-run value. A sensitive seed needed to reproduce a fuzz failure may live only in an access-controlled, short-retention encrypted test vault outside Git; normal evidence records only the seed policy, minimized shrink trace and a keyed digest.

Allowed handling sequence:

1. Generate a unique value in the isolated test process; immediately compute a keyed or approved irreversible digest.
2. Use raw content only at the authorized client boundary. Message/file/Artifact content must cross Server, queue, object storage and backup boundaries only as ciphertext.
3. Scan every prohibited surface for the raw canary while the test environment exists. Record only classification, surface, hit count and digest.
4. Fail on any unexpected hit. Do not redact a hit and continue as `PASS`.
5. Clear memory where the runtime permits, cryptographically erase the run volume, destroy run credentials and verify teardown.

Test credentials are minted per run with the least scope and short expiry. Their repository fixture is a descriptor such as `expired-capability` or `cross-tenant-route-grant`, never a serialized bearer value.

## Required manifest

Every fixture family must have a machine-readable manifest before data files are added. Manifests are versioned, owner-specific contracts rather than one universal JSON shape. The integrated T014/T015/T019 v1/v2 manifests remain authoritative and conform through the semantic aliases below; they must not be rewritten merely to normalize casing. CI validates each declared manifest version and the equivalent meaning, while new generic registry manifests should prefer the canonical names in the first column.

| Canonical meaning | Accepted representation / requirement |
| --- | --- |
| `fixture_set` | Stable namespaced ID, or the versioned manifest path plus `owner` and `contracts` as the stable family identity |
| `schema_version` | `schema_version` or `schemaVersion`; independent of product version |
| `contract_version` | `contract_version`, `contracts`, Profile/Schema fields, or a content-addressed formal-generation target that identifies the interpreting contract |
| `owner` | Workstream accountable for semantic changes |
| `reviewers` | Contracts, Security, platform or Product reviewers required by risk |
| `classification` | `public_metadata`, `public_test_material`, `synthetic_opaque`, or a narrower namespaced `synthetic-*-no-secrets` value validated by the family schema |
| `provenance` | Generator/source specification and license where applicable; never a production export |
| `generator` | Top-level field or `provenance.generator`; exact source/version, or explicit `none` for reviewed hand-authored inputs |
| `seed_policy` | Explicit when generation, KATs, fuzzing or sensitive inputs apply. Hand-authored `synthetic-*-no-secrets` families may inherit this policy when their manifest constrains `allowedData`/`forbiddenData` and contains no seed |
| `allowed_surfaces` | Explicit family field when access differs from this registry; otherwise inherited from the family verifier and this policy |
| `forbidden_surfaces` | Explicit family field when stricter; otherwise logs, telemetry, crash artifacts and production import are forbidden by this policy |
| `cleanup` | Ephemeral state, credential and volume destruction requirements |
| `sha256` | Top-level digest map or content-addressed `source`/required-file/evidence entries covering every committed fixture and generator input |
| `expected` | `expected`, `expectedBehavior`, required cases or transcript outcomes describing stable external state/error; never an implementation-internal call sequence |
| `n_minus_one` | `n_minus_one`, `nMinusOne` or linked generated N-1 evidence with supported reader/writer pairs and retained historical files |
| `reopen_triggers` | Explicit family field, or inherited from the test plan's reopening rules plus owner/reviewer approval on contract, generator, provider or security changes |

Each committed public/opaque file must be reproducible from reviewed source. A generator update is a semantic change: review the generator diff, every output diff, classification scan and historical compatibility result. A public/opaque regression seed may be retained as numeric/text metadata. A sensitive regression seed is never retained in Git or normal CI artifacts: keep it only in the short-retention encrypted vault and retain the seed policy, minimized shrink trace and keyed digest in evidence, never the sensitive generated payload.

## Fixture families and owners

| Family | Content | Owner | Required consumers / proof |
| --- | --- | --- | --- |
| Contract / Golden Frame | Representative public fields, unknown field `50000` canary, expected semantics | Contracts + Integration | Go, TypeScript, Rust, Swift, Kotlin decode/mutate/re-encode and N-1 |
| Stable errors | Domain/code/retryability/user key and unknown fallback | Domain contract owner + FFI | Connect/gRPC/WSS/IPC/FFI adapters produce the same safe error |
| Crypto Golden Vector | RFC/public KAT reference, public metadata/digests, Epoch/History/Recovery expected state | Crypto-recovery + independent Crypto Reviewer | Rust/Swift/Kotlin hosts and an independent RFC 9420 implementation |
| FFI fault cases | Fixed fault name, operation timing/state, expected status/cursor/resource count | Client-core / FFI | Rust, Desktop IPC, iOS and Android; cancel, panic, backpressure, Crash/Resume |
| Sync state machine | Event IDs, sequence graph, duplicate/gap/fault schedule and final digest | Client-core + Core/Sync | Property, integration and five-client E2E |
| Authorization | Synthetic Actor/resource relation, policy version and allow/deny result | Core Authorization + Security | Cross Tenant/Channel/Object, expiry, revoke and replay negative tests |
| Runtime / Connector | Synthetic Task/Run/Lease generation, logical workspace and normalized relative path cases | Runtime + Connector | Grant/Fencing/transfer, traversal/symlink/TOCTOU; no physical user path |
| Load / Chaos scenario | Workload shape, rates, distributions, fault schedule and SLO threshold | SRE/Performance | Reproducible generator, steady state, recovery invariant and capacity report |
| E2E topology | Synthetic logical Tenant/Actor/Device/Channel/endpoint descriptors | Quality + Product capability owners | AC scenario setup; real credentials and content minted ephemerally |
| Real-device descriptor | Platform/OS/ABI/capability requirements, no personal device identity | iOS/Android/Desktop Release Owners | Device-lab selection and result manifest; no UDID or user data |

The Crypto owner controls `test/crypto/`; Contracts controls concrete Protobuf fixture paths; Quality owns this policy and cross-suite registry. No team may copy a crypto vector or Golden Frame into a second mutable location to avoid the owning contract.

## Contract and N-1 retention

Persisted Protobuf fixtures must preserve the field `50000` unknown-field canary. Tests compare semantic values and canary survival; exact byte equality is asserted only when the owning contract explicitly defines canonical serialization.

For every supported compatibility window, retain:

- the oldest supported N-1 frame and its manifest digest;
- the current additive frame;
- expected absent-field and unknown enum/error behavior;
- N-1 read/current write and current read/N-1 write outcomes for all consuming languages;
- mutation cases that retain unknown fields;
- upgrade, rollback and support-window metadata.

Historical fixture removal requires a package-major migration/retention decision and Integration approval. Generator upgrades replay every retained frame before output can merge. An adapter that drops unknown persisted fields is read-only until repaired.

## Crypto-specific rules

Crypto fixtures describe external behavior, not OpenMLS or another provider's internal types. Required classes cover KeyPackage/Welcome, Epoch progression, add/remove/revoke, offline and concurrent Commit, replay/fork/rollback/unknown Profile, History Sharing, Device History Sharing, Recovery binding/failure, retention and Crash/Resume.

Private test material is generated in memory from a runtime CSPRNG with a fresh nonce; it is not deterministically derived from a committed seed. A committed file may hold public KAT material only when the manifest identifies the public source and Security confirms that it is non-secret test material. No fixture may become a Device identity, recovery recipient, package signing key or environment credential.

The existing `test/crypto/e2ee-interop-v1.manifest.json` and its `.vector` payload are semantic spike evidence. The T019 contract fixture at `test/fixtures/proto/crypto/manifest.json` adds frozen schema and five-language compatibility evidence. Neither proves production readiness, KMS/HSM recovery, real Swift/Kotlin Crypto FFI or an approved production MLS provider; fixture descriptions must retain those limitations until the corresponding evidence exists.

## FFI Crash/Resume fixtures

FFI fault descriptors must express observable states rather than native pointers or host objects. At minimum they cover:

- cancel before commit, cancel after commit and idempotent repeated cancel;
- isolated native panic and reserved unknown error;
- bounded reliable stream, explicit backpressure, duplicate sequence rejection and Cursor resume;
- stale/generation-mismatched handle, exactly-once release and 1,000 create/start/close cycles;
- process termination after a synchronized Cursor, fresh-process resume at `cursor + 1`, and no callback after close;
- iOS/Android background eviction and real-device memory pressure as separate evidence, not simulator fixture results.

Crash reports and native symbols remain build artifacts subject to content scan. The fixture never includes a message payload, key, local path, host callback object or raw memory dump.

## Load, chaos and real-device descriptors

Load fixtures store the shape of a workload, not captured traffic: logical tenant/channel counts, event-size buckets, rates, duration, expected percentiles and capacity budget. Generated payloads are opaque/ciphertext at service boundaries. A report records generator/version, public/opaque seed when allowed, seed policy and digest; sensitive payload reproduction uses the encrypted-vault seed plus the recorded shrink trace, without retaining content or exposing the seed in the report.

Chaos fixtures store the fault schedule and invariants: target component, start/end condition, loss/delay/restart parameters, steady state, expected degradation, recovery limit and forbidden outcomes such as fake ACK, old-writer commit or plaintext fallback. They never ship cluster credentials or real endpoint addresses.

Real-device descriptors contain only platform, supported OS/build range, architecture, capabilities and anonymized lab asset reference. Result manifests may include product version, Bridge/Core/Profile, signed artifact digest, metrics and reviewer; they exclude UDID, phone number, system account, personal notification content and production provisioning material.

## Review and CI gates

Fixture changes fail review when any of the following is true:

- the input contract is not merged or the fixture guesses fields/errors;
- provenance, owner, version, digest, classification or expected result is missing;
- a secret/content scanner finds prohibited plaintext, token-shaped data, key material, absolute paths or production identifiers;
- an existing historical/N-1 fixture changes in place instead of adding a new version;
- public/opaque output depends on wall-clock time, network access, floating tool versions or an unrecorded seed, or sensitive output uses a fixed/committed seed instead of runtime CSPRNG and the encrypted-vault policy;
- the test asserts implementation internals instead of the public seam;
- evidence is only a simulator when a real-device or external-review Gate applies.

CI should validate the declared family manifest version and semantic aliases above, file digests, deterministic regeneration, classification policy, no forbidden extensions, links to existing owners/contracts and all historical consumer tests. The scanner must inspect Git blobs as well as the working tree so deletion in the latest revision does not hide an accidentally committed secret.

## Incident handling

If real or reusable sensitive material is found, stop test publication, revoke/rotate it, preserve only approved incident metadata, remove access to affected artifacts/caches and follow the repository's security incident process. A later Git deletion does not make an exposed credential safe. Do not copy the value into an issue, chat, log or fixture-cleanup report.

If only forbidden synthetic plaintext is found, fail the gate, remove it from all retained artifacts, fix the generator/evidence path and rerun the content scan. The result remains `FAIL` until the clean candidate is independently verified.

## Dependency convergence

Convergence status at policy freeze:

1. **Complete:** T002 acceptance scenarios are merged; future fixture families must map to those AC IDs and evidence requirements, and CI must detect drift.
2. **Complete:** T011's Crypto Owner published a manifest-backed semantic spike fixture without turning it into production evidence.
3. **Complete:** T014/T015 integrated the required manifest, hardened verifier, formal five-language generation and Golden/N-1 workflow.
4. **Complete:** T019 froze Device/Epoch/History/Recovery contracts with Architecture/Security review and generated compatibility evidence.
5. **Gate remains HOLD:** ADR-0003 and ADR-0004 remain `proposed`; OpenMLS `0.8.1` is rejected and the `0.9.0` final provider implementation and production admission evidence are not established.
6. **Gate remains NOT RUN:** T010-B / Issue #41, currently labeled `ready-for-human`, must supply real-device FFI results; simulator/emulator fixtures cannot satisfy the device Gate.

T017 owns only this policy and registry description. It does not edit T011/T014/T019 artifacts, invent schema values, or copy them into a parallel location. A final policy is not Gate PASS evidence: unresolved Security/provider and real-device results remain external `HOLD/NOT RUN` inputs.

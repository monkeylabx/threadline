# Crypto Provider version and transaction profile

Status: Draft for #202, 2026-09-30. Base
`31d6b2b47bd98eb6eb68c18b23ecdfc08d220f51`: issue baseline `b045d402` plus
documentation/CI-only commits, with no cited crypto input changed.

Documentation only: no crate, dependency, lockfile, Proto, fixture or schema
changes.
- ADR-0003 and ADR-0004 stay **proposed**.
- M0/G0 stays HOLD.
- [#41](https://github.com/monkeylabx/threadline/issues/41) stays
  physical-device NOT RUN.
- Production Crypto Provider admission stays NOT RUN.

This is the bounded input C1 needs. Every "must" is a proposal; accepting #202
approves none of D1-D13.

A fake provider that satisfies this contract proves interface shape only. Fake
success is never security, interoperability or admission evidence, and it
changes no row of the [admission matrix](../architecture/crypto-provider-admission.md).

## Inputs and current wire authority

**[ADR-0003](../adr/0003-group-e2ee-recovery.md)**
- §1: only the `client-crypto` Adapter may see OpenMLS types.
- §5: each message uses an independent random **Content Key**, wrapped by the
  current Epoch's **History Key**. History Keys are Threadline application-layer
  material retained for the Retention period, not MLS ratchet secrets, which
  are deleted per RFC 9420. The exact KDF, labels, AEAD/HPKE construction and
  anti-rollback rules must come from the M0 protocol specification, Golden
  Vectors and an independent review, never from ad-hoc implementation.
- §9: `tl-mls-1` is an immutable tuple. A library release is not a wire
  version. An unknown Profile is incompatible and never downgraded. A Profile
  upgrade happens only through a REINITIALIZE successor generation.

**[ADR-0004](../adr/0004-e2ee-crypto-library-selection.md)**
- OpenMLS tracks the 0.9.0 stable line.
- Handshakes (Commit/Proposal) use PrivateMessage, and the LeafNode lifetime
  policy is shared by issuer and verifier.
- §5 requires `client-crypto` to implement its own encrypted `StorageProvider`,
  because library persistence is plaintext.

**Wire authority**
- [crypto.proto](../../proto/threadline/crypto/v1/crypto.proto) defines:
  - the `CryptoProfile` tuple;
  - `E2eeGroupState`: `ACTIVE`, `REKEY_REQUIRED`, `INCOMPATIBLE`, `SUPERSEDED`;
  - `E2EEGroup.generation`, which changes only on REINITIALIZE, and
    `current_epoch`, which is a routing hint, not authority;
  - the seven `MembershipChangeKind` values, `MembershipChangeAuthorization`
    and `MlsWireMessage`.
- [envelope.proto](../../proto/threadline/message/v1/envelope.proto):
  `application_ciphertext` is AEAD-protected, with the canonical sender fields
  as AAD, and `sender_signature` is covered by the same fields.
- [key_service.proto](../../proto/threadline/crypto/v1/key_service.proto): the
  `CryptoCompatibilityService` rollout floors.

**T019 [scenarios](../../test/fixtures/proto/crypto/scenarios.json)** are 253
synthetic cases checked by
[verify-crypto-contracts.mjs](../../proto/tools/verify-crypto-contracts.mjs).
They freeze the wire error semantics used in Section 4 and are not provider
evidence.

**Current code**
- [client-crypto](../../crates/client-crypto/src/lib.rs) contains only
  `ADAPTER_CONTRACT_VERSION = 1`.
- [client-core storage](../../crates/client-core/src/storage/mod.rs):
  - `EncryptedDatabase` owns a private connection;
  - only `record_cursor` and migration use IMMEDIATE transactions, and the
    other writes are single-statement autocommit;
  - the schema is pinned to migration 0001, with no crypto-state table;
  - neither crate depends on the other.
- [client-ffi](../../crates/client-ffi/src/runtime.rs) linearizes cancel against
  a `Pending → Committed` phase change under one lock. After that point,
  `cancel()` returns `AlreadyCommitted` and the request still succeeds.

## 1. Version axes and capability

These axes are independent. None may stand in for another, and none is
negotiated member-by-member.

| Axis | Identifies | Authority | Change rule | Mismatch |
| --- | --- | --- | --- | --- |
| Crypto Profile | Wire semantics: the full `tl-mls-1` tuple, including envelope versions | ADR-0003 §9 and `CryptoProfile` | Fixed per group; new Profile only through a REINITIALIZE successor | `ProfileUnsupported`; durable `INCOMPATIBLE` marker (Section 5) |
| Crypto contract surface | Which additive Proto semantics may run | `CryptoCompatibilityService` floors | Server-enforced rollout | Read/audit-only (T019) |
| Adapter contract | Rust seam of the provider | `ADAPTER_CONTRACT_VERSION` | Additive within a version | Build or startup failure |
| Provider library | OpenMLS or another implementation, and its build | C2 admission packet | Rerun the evidence per the admission invalidation rules | Not a runtime input; must not change wire output |
| State format | Encoding of Threadline-owned provider state | D5; C3 schema | Forward-only migration inside a CAS commit | `StateFormatUnsupported`; no write, no downgrade |

**Capability** means the provider implements one operation for one Profile.
The provider exposes a fixed compiled capability set per Profile. A missing
capability returns `CapabilityUnsupported` before any state is read. There is no
partial execution and no fallback to another operation or Profile. The
first-slice set is D10.

## 2. Minimum caller-visible opaque types

No type exposes OpenMLS values, SQLCipher connections, raw keys, or ratchet or
tree state.

| Type | Visible content | Rule |
| --- | --- | --- |
| `ProfileId` | Validated full tuple, built only by `from_wire(CryptoProfile)` | Exact match or error; no name-only lookup |
| `GroupRef` | `tenant_id`, `e2ee_group_id`, `generation` | Checked before state load |
| `DeviceContext` | `tenant_id`, `actor_id`, `device_id`, credential version | From the local enrolled identity; the provider checks it against its own leaf and keystore |
| `TrustedGroupStatus` | Server-ordered `E2eeGroupState`, current epoch, `change_seq` head | From verified server facts (D13); never inferred from ciphertext |
| `ServerOrder` | Authorization, expected and successor epoch, `change_seq`, `handshake_id` | Binding verified per the T019 transcripts before use |
| `DeviceAuthorityFacts` | Credentials and approval chain for leaves being added or joined | Verified per the T019 `credential-*` cases; a leaf without them is rejected |
| `ScopeRef` | `Group(GroupRef)` or `DeviceKeystore(tenant_id, device_id)` | Unit of versioned state and CAS |
| `StateVersion` | Opaque monotonic token per scope | Never interpreted by callers |
| `StagedTransition` | `(ScopeRef, base, new)` entries, opaque bytes, erasure set, outputs | Outputs visible only inside the commit transaction (Section 5) |
| `WireFrame` / `SealedBody` / `PlaintextBuffer` | Opaque MLS bytes, sealed application fields, zeroizing plaintext | Plaintext is zeroized on drop |
| `CryptoError` | Closed enum (Section 4) | No panic path and no library text |

## 3. Operations

Every operation takes `ProfileId`, `DeviceContext` and a cancellation handle.
Mutating operations run on private copies and return a `StagedTransition`.
Failed validation leaves stored state untouched.

**State transitions (committed per Section 5):**

| Operation | Preconditions | Success postcondition | Failures |
| --- | --- | --- | --- |
| `join_from_welcome` | Welcome is `INDEPENDENT`, targets this device and matches the `ServerOrder`; the keystore holds its KeyPackage private key; profile is exact; every leaf passes `DeviceAuthorityFacts` | One commit: group scope created at epoch `e` with History Key(`e`), and the KeyPackage key compare-and-deleted from the keystore scope | `KeyPackageConsumed`, `KeyPackageUnavailable`, `ProfileUnsupported`, `GroupMismatch`, `TenantMismatch`, `CiphertextCorrupt`, `NotAuthorized` |
| `create_commit` | Verified `MembershipChangeAuthorization`: this device is committer, not expired, supported kind, expected epoch equals current; no own pending commit; claimed KeyPackages match the targets; every added leaf passes `DeviceAuthorityFacts` | Pending commit plus the handshake sender-ratchet advance committed. The Commit and Welcome frames are released only after commit (D1) | Mapped per Section 4; `PendingCommitExists` |
| `process_handshake` | PrivateMessage frame; `change_seq` next; epoch equals current; binding verified; committer as authorized; new leaves pass `DeviceAuthorityFacts` | COMMIT: epoch `e+1`, History Key(`e+1`) derived and stored, MLS past-epoch secrets deleted per D3, `REKEY_REQUIRED` cleared when the successor exists. PROPOSAL: stored, epoch unchanged | Section 4 handshake and authorization rows; `ForkDetected` |
| `resolve_own_commit` | Pending commit exists. Merge when our `handshake_id` returns from the server order; discard on server `EPOCH_STALE` | Merge is equivalent to `process_handshake` of our commit; discard restores the pre-pending state, then the caller resyncs and regenerates | `NoPendingCommit`, `StateConflict` |
| `mark_group_status` | Local evidence of fork, incompatible Profile or supersession | Durable quarantine or status marker in the group scope, so restart never resumes sending | `StateConflict` |
| `prune_history_keys` | Trusted Retention decision | History Keys outside retention erased in one commit | `StorageUnavailable` |
| `erase_group` | Trusted removal, retention or supersession decision | All group-scope state erased; device-keystore scope untouched | `StorageUnavailable` |

**Read-only operations (no provider state change):**

| Operation | Preconditions | Result | Failures |
| --- | --- | --- | --- |
| `load_group` | Stored state for exactly this `GroupRef`; supported format; stored Profile equals `ProfileId` | Handle at `StateVersion`, including any quarantine marker | `GroupNotFound`, `StateFormatUnsupported`, `ArgumentMismatch`, `GroupMismatch`, `GroupSuperseded`, `StateCorrupt` |
| `seal_application` | `TrustedGroupStatus` is `ACTIVE`; local epoch equals the trusted current epoch; no quarantine; History Key(current) and device signing key present | Fresh random Content Key per message; `SealedBody` (ciphertext, `content_hash`, `sender_signature`) bound to the canonical sender fields | `RekeyRequired`, `EpochAhead`, `GroupQuarantined`, `CapabilityUnsupported` |
| `open_application` | AAD binds tenant, group, epoch and conversation; History Key(epoch) retained; sender device was a member at that epoch; signature verifies and `sender_actor_id` matches the credential | Plaintext | `CiphertextCorrupt`, `EpochUnavailable`, `EpochAhead` (caller queues), `GroupMismatch`, `TenantMismatch`, `NotAuthorized` |

**Rules for application messages:**
- The Content Key/History Key construction and labels are undecided (D11).
  Until they are decided, the C1 fake implements `seal_application` and
  `open_application` only as a labeled non-cryptographic placeholder.
- A sealed envelope reaches durable storage only through client-core's Local
  Outbox (#193). Resealing after `REKEY_REQUIRED` follows #193's M5 policy,
  which is a prerequisite here.
- Duplicate application events are handled by client-core's
  `(tenant, conversation, event_id)` uniqueness, not by the provider.
- Checking `AgentAttribution` against Task/Run/Grant facts is a caller duty;
  the provider only proves the signing device.

**Other operations:**
- An exact duplicate `handshake_id` that was already applied returns its
  recorded outcome with no new transition (D4, T019 exact-duplicate
  idempotency).
- Group creation, REINITIALIZE (which touches two groups), History Sharing and
  Recovery wrapping return `CapabilityUnsupported` in the first slice (D10).

## 4. Error vocabulary

`CryptoError` is closed. Wire codes follow the T019 verifier; local errors never
cross the network as invented codes (D6).

| Condition (T019 source) | Error | Wire `ErrorCode` |
| --- | --- | --- |
| Unknown kind, non-`tl-mls-1` group Profile, successor Profile mismatch | `ProfileUnsupported` | `CRYPTO_PROFILE_UNSUPPORTED` |
| Unknown MLS message type | `EnvelopeVersionUnsupported` | `ENVELOPE_VERSION_UNSUPPORTED` |
| Tenant differs (authorization, wire, successor group) | `TenantMismatch` | `TENANT_MISMATCH` |
| Group or parent differs; successor parent or predecessor wrong | `GroupMismatch` | `GROUP_MISMATCH` |
| Caller is not committer; wire sender differs; duplicate or kind-inconsistent targets | `NotAuthorized` | `PERMISSION_DENIED` |
| Authorization expired | `AuthorizationExpired` | `GRANT_EXPIRED` |
| Binding hash invalid, kind substitution, bytes touched, handshake not PrivateMessage, Welcome/GroupInfo not `INDEPENDENT`, malformed fields | `CiphertextCorrupt` | `CIPHERTEXT_CORRUPT` |
| `change_seq` not next; conflicting duplicate authorization; recovery version not increasing | `Replay` | `REPLAY_DETECTED` |
| Expected epoch below current; successor not current + 1; generation below | `EpochStale` (committer side) | `EPOCH_STALE` |
| Expected epoch above current; generation skips ahead; application epoch not reached | `EpochAhead` | `EPOCH_AHEAD` |
| Application epoch never held or its History Key pruned | `EpochUnavailable` | `EPOCH_UNAVAILABLE` |
| ADD without KeyPackages | `KeyPackageUnavailable` | `KEY_PACKAGE_UNAVAILABLE` |
| KeyPackage already consumed | `KeyPackageConsumed` | `KEY_PACKAGE_CONSUMED` |
| Application send while `REKEY_REQUIRED` | `RekeyRequired` | `REKEY_REQUIRED` (stays local) |
| Revoked device (trusted facts) | `DeviceRevoked` | `DEVICE_REVOKED` |
| `CapabilityUnsupported`, `StateFormatUnsupported`, `ArgumentMismatch`, `GroupSuperseded`, `GroupQuarantined`, `ForkDetected`, `PendingCommitExists`, `NoPendingCommit`, `StateConflict`, `StateCorrupt`, `StorageUnavailable`, `Canceled` | Local | None |

`AlreadyCommitted` is not an operation error. It is what `cancel()` returns once
the commit point is passed, and the operation itself still succeeds, matching
`client-ffi`.

## 5. Transaction requirements

The provider owns no storage. It requires a port, proposed as
`CryptoStateStore` and defined in `client-crypto` (D2). Semantics:

1. **Scoped state.** State lives in scopes: one per group, plus one
   **device-keystore** scope per device that holds KeyPackage private keys and
   device signing material. `read(ScopeRef)` returns one committed snapshot
   `(StateVersion, bytes)`.
2. **Atomic multi-scope CAS.** `compare_and_commit(entries, companion)` takes
   entries sorted in canonical `(kind, tenant, id)` order. It commits only if
   every scope's stored version equals its `base`. All new state, erasures and
   the implementer's companion writes land in one durable transaction.
   - The companion callback receives the staged outputs **inside** that
     transaction: for example, the outgoing Commit frame into the Local Outbox,
     or the applied handshake event plus its cursor advance.
   - Outputs are released to the network only after the transaction commits.
   - On any error, nothing is visible.
3. **Serialization and conflicts.** The caller serializes operations per scope
   in-process, and CAS is the cross-process guard. A mismatch returns
   `StateConflict`, and the caller reloads and reruns the whole operation. A
   staged transition is never re-based, and its outputs are zeroized.
4. **Cancellation is linearized under one lock** with the phase change
   `Pending → Committing`, taken immediately before the durable COMMIT.
   - Cancel while `Pending`: no change, result `Canceled`.
   - Cancel at or after `Committing`: `cancel()` returns `AlreadyCommitted`,
     and the operation reports the real COMMIT outcome.
   - If COMMIT itself fails, the operation returns `StorageUnavailable` and
     nothing is committed.
5. **Validation failures never write.** Libraries that mutate on failure run
   only on private copies.
6. **Quarantine and incompatibility** are persisted by `mark_group_status` in
   their own CAS commit. A caller argument mismatch (`ArgumentMismatch`) is not
   a group state and writes nothing.
7. **Crash.** A crash before COMMIT leaves every scope at `base`; nothing was
   released. A crash after COMMIT leaves the new versions, with outputs held by
   companion records.
8. **Whole-database rollback** (an older file restored) is invisible to CAS.
   It could replay handshake ratchet state, so anti-rollback is a separate
   requirement (D12).
9. **Isolation.** The port never exposes SQL, a connection or a key.

**Why C1 can precede C3 without a cycle.**
- C1 defines the port types and semantics above, plus an in-memory fake that
  reproduces Section 7.
- C3 then specifies how `client-core` implements the port on
  `EncryptedDatabase`: a new migration (the schema is pinned to 0001), key
  purposes, crash and migration behavior, and companion atomicity.
- `client-core` depends on `client-crypto`'s port types, never the reverse.
  That manifest edge and the migration are separate Integration-owned tasks.
- D2 reads ADR-0004 §5 as follows: the OpenMLS `StorageProvider` lives inside
  the `client-crypto` adapter and is backed by this port, and at-rest encryption
  comes from `EncryptedDatabase`. The owners must confirm that reading.

## 6. Support negotiation and fail-closed cases

| Case | Outcome |
| --- | --- |
| Exact `tl-mls-1` tuple, capabilities present, compatibility floors satisfied | Supported |
| Name `tl-mls-1` with any different member, zero version or unknown suite | `ProfileUnsupported`; `INCOMPATIBLE` marker; no alias or partial match |
| Caller-supplied `ProfileId` differs from the stored group Profile | `ArgumentMismatch`; no state effect |
| KeyPackage support range excludes the group Profile | Device cannot join; the group is never downgraded |
| Provider library changed, Profile unchanged | Not a negotiation input; wire output must be identical (C5 interop gate) |
| Operation missing from the compiled set, including REINITIALIZE and group creation in the first slice | `CapabilityUnsupported` before state read |
| Stored state format newer than supported | `StateFormatUnsupported`; no write, no downgrade |
| Stored state format older | Forward migration only inside a CAS commit, with C3 crash evidence |
| Compatibility floor missing, lowered or `UNIMPLEMENTED` | Read/audit-only (T019) |
| Requested `generation` differs from stored | `GroupMismatch` before load |
| Stored group marked `SUPERSEDED` | `GroupSuperseded`, carrying the successor `GroupRef` |

## 7. Deterministic transition examples

For the C1 fake. `gN` and `kN` are symbolic group-scope and keystore-scope
versions.

| # | Start | Input | Result |
| --- | --- | --- | --- |
| T1 | g7, epoch 3, trusted `ACTIVE` | `seal_application` | Placeholder `SealedBody`; g7 unchanged |
| T2 | g7, trusted `REKEY_REQUIRED` | `seal_application` | `RekeyRequired`; no sealed bytes; the intent stays pending under M5 |
| T3 | g7 | `open_application` of a tampered body | `CiphertextCorrupt`; no plaintext |
| T4 | g7, History Key(1) pruned | Open an epoch-1 body; then an epoch-5 body | `EpochUnavailable`; then `EpochAhead` (queue) |
| T5 | g7 | `create_commit` (self-update), no cancel | g7→g8 with the Commit frame in the companion Outbox; released after commit |
| T6 | g7 | `create_commit`, cancel while `Pending` | `Canceled`; g7; nothing released |
| T7 | g7 | `create_commit`, cancel after `Committing` | `cancel()` returns `AlreadyCommitted`; the operation succeeds; g8 |
| T8 | g8, pending | A second `create_commit` | `PendingCommitExists` |
| T9 | g8, pending | Own `handshake_id` returns; or the server rejects `EPOCH_STALE` | Merge g8→g9, epoch 4 with History Key(4); or discard g8→g9 with the pre-pending state, then regenerate |
| T10 | g7, epoch 3 | `process_handshake` COMMIT, `change_seq` next | g7→g8, epoch 4. The same `handshake_id` again: recorded outcome, g8. A PROPOSAL: stored, epoch 4 unchanged |
| T11 | g7 | Handshake whose `change_seq` skips ahead | `Replay`; g7 |
| T12 | k5; Welcomes A and B both use KeyPackage P | Join A and join B concurrently | A commits (group A g1, k5→k6 deletes P). B gets `StateConflict`; rerun finds P gone: `KeyPackageConsumed` |
| T13 | g7 | Divergent transcript detected | `ForkDetected`; `mark_group_status` g7→g8 quarantined; after restart, `seal_application` returns `GroupQuarantined` |
| T14 | g7, newer stored format | `load_group` | `StateFormatUnsupported`; no write |
| T15 | g7 | Crash after staging, before COMMIT | Restart reads g7; nothing was released |
| T16 | g7, generation 1 | `GroupRef` with generation 2 | `GroupMismatch` before load |

## 8. Evidence gaps mapped to the admission matrix

C1 and its fake close no row.

| Admission row | What this contract adds | Still NOT RUN |
| --- | --- | --- |
| Library, checksum, SBOM, advisories | None | C2 packet on the stable graph |
| Malformed ciphertext returns an ordinary error | Closed `CryptoError`; no panic path | Stable-provider debug/release and FFI runs |
| RFC known-answer vectors | None | Remaining KAT coverage on the final build |
| Independent interoperability | "Library is not a negotiation input" rule | C5 on the stable provider |
| Epoch/add/remove/revoke, replay, ordering | Pre/postconditions and T9-T11 | Real persistence plus server ordering |
| Concurrent Commit and Group Fork | Pending rule, discard/resync, quarantine | C6 two-writer evidence |
| Profile, PrivateMessage, lifetime | Exact-tuple and fail-closed tables | Swift/Kotlin provider runs |
| Crash/resume and deletion | Commit point, crash rules, erasure in commit, rollback gap (D12) | C3/C4 crash-mid-write, rollback detection, key erasure |
| Encrypted storage and key custody | Port with no connection or key exposure; keystore scope | SQLCipher adapter, OS key custody, lock/unlock |
| macOS/Windows/Linux hosts | None | Same admitted provider on all three |
| Swift/Kotlin, iOS/Android | Cancellation aligned with `client-ffi` | MLS through FFI; #41 physical devices |
| Independent security review | Application construction left to review (D11) | Not begun |

The History Sharing, Recovery isolation, Retention/backup and performance rows
are unaffected.

## 9. Decisions requiring review

Each blocks production adoption until its owner records approval.

| ID | Proposed decision | Owner |
| --- | --- | --- |
| D1 | Commit-before-release for state transitions whose outputs leave the device; outputs exposed only inside the companion transaction | Crypto, Security, Client-core |
| D2 | Port in `client-crypto`, implemented by `client-core`; confirm the ADR-0004 §5 reading | Crypto, Client-core, Security |
| D3 | History Key retention and pruning, and the MLS past-epoch secret deletion schedule | Crypto, Security, Retention |
| D4 | Local idempotency of an exact duplicate applied `handshake_id` | Crypto, Contracts |
| D5 | State format versioning and forward-only migration | Client-core, Crypto |
| D6 | Local-only errors get no new wire code | Contracts |
| D7 | Plaintext buffer ownership and zeroization across FFI | Client-core, native hosts, Security |
| D8 | Per-scope serialization plus canonical-order multi-scope CAS | Client-core, Crypto |
| D9 | Device-keystore scope and KeyPackage compare-and-delete in the join commit | Crypto, Security |
| D10 | First-slice capabilities: load, join, commit, handshake, resolve, status, prune, erase, plus placeholder seal/open; no group creation, REINITIALIZE, History or Recovery | Crypto, Product |
| D11 | Application body construction: Content Key and History Key KDF, labels, AEAD, signature canonicalization | Crypto, Security, independent reviewer |
| D12 | Anti-rollback for local crypto state against whole-database restore | Client-core, Security |
| D13 | Source of `TrustedGroupStatus`, and the #193 M5 reseal policy as a prerequisite | Client-core, Contracts |

## 10. Successors

- **C1 interface and fake, 1 day.** Crypto: `crates/client-crypto/`. After this
  contract is reviewed, implement the Section 2-5 types, the port and a
  deterministic fake reproducing T1-T16. Seal/open are a labeled placeholder
  until D11. Entry point: `cargo test -p threadline-client-crypto --locked`,
  NOT RUN because the code does not exist yet.
- **C3 storage transaction contract.** Client-core: `docs/contracts/`, after C1.
  It implements Section 5 on `EncryptedDatabase` through a separately reserved
  migration.
- **C2 dependency admission.** Stays subject to an explicit Security and
  Architecture candidate decision. No lockfile change or library approval
  happens here.

## This task's verification

Run `git diff --check`, resolve every local Markdown link in this file, and run
`node proto/tools/verify-contracts.mjs` and
`node proto/tools/verify-crypto-contracts.mjs`. They show that no wire,
generated or fixture surface changed; they do not test this proposal.

# Audit, Retention, and Recovery metadata contract

Status: P03-07A v1 contract

This contract defines the durable metadata boundary consumed by later Core
Audit storage, Retention, Recovery, and approved operator-command tasks. It is
an append and integrity interface, not an Audit Viewer, a generic logging
format, or authority to retain protected content.

## 1. Boundary

An Audit Event answers only who performed which allowlisted control-plane
action, against which exact tenant-bound target, when the database recorded
it, with which stable result and policy/evidence references. It never embeds a
business payload or diagnostic message.

Core owns Audit writes. Authenticated Principal facts come from the trusted
request context, authorization/approval facts come from current Core reads,
and sequence, time, and chain-head facts come from PostgreSQL. A command may
propose an action and target, but it cannot supply or override those trusted
facts.

The v1 persisted event is a documented internal interface. P03-07A does not
add a public Protobuf/RPC surface, a migration, or a Go store.

## 2. Durable event projection

Every persisted `domain.audit_events` row and every public v1 fixture projects
to exactly these fields. Nullable identifiers are JSON `null`, never empty
strings or omitted properties.

| Property | Type | Owner and rule |
| --- | --- | --- |
| `contractVersion` | unsigned decimal string | Contract; exactly `"1"` |
| `auditEventId` | identifier | Core idempotency key; unique with Tenant |
| `tenantId` | identifier | Authenticated context; never command evidence |
| `tenantSequence` | positive signed-`bigint` decimal string | Database, contiguous within one Tenant |
| `recordedAt` | UTC timestamp | Database time with exactly nine fractional digits |
| `principal` | Actor object | Authenticated Actor; exact `actorId` and decimal-string `actorType` |
| `action` | registry string | Exact v1 action vocabulary below |
| `outcome` | registry string | Exact v1 outcome vocabulary below |
| `reason` | registry string | Stable minimized reason; never diagnostic text |
| `target` | Target object | Exact `targetType`, `targetId`, and nullable `targetVersion` |
| `policyVersion` | identifier | Trusted policy snapshot used by the decision |
| `requestId` | identifier | Trusted request/correlation identity; not globally unique |
| `approvalId` | identifier or null | Current tenant/target-bound Approval reference when required |
| `recoveryCaseId` | identifier or null | Current tenant-bound Recovery Case reference when applicable |
| `evidenceDigestHex` | 64 lowercase hex or null | SHA-256 of a separately durable, versioned minimized evidence projection |
| `previousEventHashHex` | 64 lowercase hex | Previous retained Event hash in this Tenant chain; zero for genesis |
| `eventHashHex` | 64 lowercase hex | SHA-256 of the canonical transcript defined below |

An identifier is non-empty, valid UTF-8, already trimmed, contains no Unicode
control character, `*`, or `?`, and is bounded by the later migration. An
Actor type is one of Protocol Human `1`, Agent `2`, or Service `3` encoded as a
decimal string. A Target version is either `null` or a positive signed-`bigint`
decimal string; it binds generation/version-sensitive commands such as Outbox
replay.

### 2.1 Initial v1 registries

The initial action registry is deliberately small:

- `channel.archive`
- `capability_grant.issue`
- `capability_grant.revoke`
- `retention.expire`
- `retention.legal_hold.apply`
- `retention.legal_hold.release`
- `recovery.request`
- `recovery.decision`
- `recovery.commit`
- `outbox.replay.request`

The outcome registry is `succeeded`, `denied`, and `failed`. The reason
registry is `authorized`, `authorization_denied`, `evidence_invalid`,
`policy_denied`, `retention_expired`, `state_conflict`, `invalid_input`, and
`internal_failure`.

The initial Target type registry is `channel`, `capability_grant`,
`retention_subject`, `recovery_case`, and `outbox_entry`. Adding or renaming a
registry value is a Contract change. Code does not synthesize values from RPC
or database names, and unknown values fail closed.

## 3. Canonical integrity chain

The v1 Event hash is SHA-256 over these exact bytes:

```text
UTF8("threadline.audit.event/v1\n" + JCS(integrity_projection))
```

`integrity_projection` contains every field in section 2 except
`eventHashHex`. Decimal values remain JSON strings. JCS is RFC 8785 JSON
Canonicalization Scheme. Producers reject invalid UTF-8, unknown or duplicate
properties, noncanonical timestamps/numbers, and registry values before
constructing a transcript. They do not repair or drop input.

The first Event for a Tenant uses 32 zero bytes as `previousEventHashHex` and
sequence `"1"`. Every later Event uses the immediately preceding retained
Event's `eventHashHex` and increments the signed-`bigint` sequence exactly once.
Database recording time is nondecreasing in sequence order.
`domain.audit_tenant_heads` stores the same Tenant, last sequence, last Event
ID, and last hash. Event and head update commit in one transaction while the
Tenant head is locked.

The chain makes mutation, insertion, deletion from the retained chain,
reordering, and cross-Tenant substitution evident to a verifier that has a
trusted head/checkpoint. Database privileges also reject update/delete through
runtime roles. This is not a transparency-log claim: a fully compromised
database owner can rewrite rows and the local head, and suffix truncation is
not independently detectable until a later external checkpoint is anchored.

## 4. Append and idempotency contract

The future store exposes one deep operation conceptually equivalent to:

```text
Append(authenticated principal, trusted decision/evidence, action target,
       audit_event_id, request_id) -> immutable Audit Event
```

Inside one transaction it:

1. validates canonical identifiers, registries, target shape, required
   evidence, and data minimization;
2. locks or creates the exact Tenant head using authenticated Tenant identity;
3. resolves every Approval, Recovery Case, and target by a Tenant-composite
   key, without revealing whether a mismatched row exists;
4. assigns database time, next sequence, and previous hash;
5. hashes and inserts the immutable Event; and
6. advances the Tenant head only if the insert succeeds.

`(tenant_id, audit_event_id)` is the idempotency key. Repeating an append with
the same normalized Principal/decision/target/evidence facts returns the
existing Event; it does not allocate a new time or sequence. Reusing the key
with any different non-database-owned fact returns `idempotency-conflict`; it
never overwrites. A request may create several Events, so `request_id` is
indexed but not unique.

The migration owner grants runtime roles only reviewed insert/append and read
functions. They receive no generic `UPDATE`, `DELETE`, sequence assignment, or
head mutation authority. Migration/backup owners remain a separate trust
boundary and their operations require platform audit evidence.

## 5. Data minimization and Retention

Audit metadata is C2 unless a separately classified identifier raises it. The
following are forbidden anywhere in an Event, evidence projection, error,
log, metric, or fixture:

- message/file plaintext, decrypted Context, Prompt, model response, or
  arbitrary justification/comment/diagnostic text;
- session/access/refresh Token, reusable Capability Grant bytes, claim token,
  nonce, signature, private/public key bytes, recovery-wrapped key material, or
  encrypted-field ciphertext;
- raw request/response, SQL, stack trace, broker payload, workspace path, or
  file path.

An irreversible SHA-256 digest may be stored only for a separately durable,
versioned, minimized evidence projection named by this or a later Contract. A
digest is not permission to hash arbitrary protected content or low-entropy
secrets.

Message, file, Prompt, Grant, cache, index, or key Retention never deletes the
corresponding required Audit metadata. `retention.expire` records only the
allowlisted target identity/version, policy version, outcome, and optional
minimized evidence digest. It does not assert physical erasure from a fully
compromised or permanently offline Device. Legal hold apply/release is a
separate audited action; its workflow and purge implementation remain
follow-ups.

## 6. Recovery metadata

Recovery Audit Events may reference `recoveryCaseId`, the target Recovery Case,
policy version, Actor decision, outcome, and a digest of the versioned minimized
decision/evidence projection. They never contain a recovery reason/note,
recipient key material, Recovery Envelope, approval signature, HSM output, or
private key.

Recovery approval validity remains owned by the current Recovery/Core control
plane. An Audit reference preserves explainability; it is not authorization to
execute recovery and cannot replace the two-person approval or target digest
checks defined by the Recovery contract.

## 7. Approved Outbox replay evidence seam

P03-08D may create and bind replay Audit evidence only inside the same Core
transaction that resolves all of these trusted facts and applies the replay:

| Fact | Required binding |
| --- | --- |
| Tenant | authenticated Principal Tenant equals Audit, Approval, and Outbox Entry Tenant |
| Principal | exact authenticated Actor equals `principal` |
| Action/result | `outbox.replay.request` and `succeeded` |
| Target | `outbox_entry`, exact Entry ID, and exact old replay generation in `targetVersion` |
| Approval | non-null current Approval bound to the same Tenant, Actor authority, action, target, and policy |
| Policy | current replay policy accepts the recorded `policyVersion` |
| Audit | append succeeds in the replay transaction; Event, replay evidence, and generation change commit atomically |

The Audit seam returns a narrow immutable staged evidence result; P03-08D
never accepts an Audit Event, Principal, Approval, Tenant, generation, policy
version, or preassembled evidence copied from request fields. Missing, stale,
revoked, cross-Tenant, target-mismatched, or chain-invalid evidence returns the
same `evidence-invalid` category without row-existence detail. The Event
targets the old generation; its versioned minimized evidence digest binds the
future replay row containing old/new generation, Approval, Actor, policy,
reason, and outcome. Audit append or replay failure rolls back both.

For the P03-07A seam, `evidenceDigestHex` is SHA-256 over:

```text
UTF8("threadline.audit.outbox-replay-evidence/v1\n" + JCS({
  auditEventId,
  tenantId,
  principal,
  outboxEntryId,
  oldReplayGeneration,
  policyVersion,
  approval: {
    approvalId,
    tenantId,
    authorizedPrincipal,
    state,
    action,
    target,
    policyVersion
  }
}))
```

All properties above are present. Actor and Target shapes and decimal-string
rules are the same as the Audit Event projection. `state` is exactly
`approved`; Approval Tenant, authorized Principal, action, target ID/old
generation, and policy must each independently equal the current facts and the
Event. A digest mismatch is `evidence-invalid`. P03-08D may extend a separately
versioned replay-row evidence projection with the assigned new generation and
final result; it cannot reinterpret or weaken this v1 Approval binding.

## 8. Stable failure categories

Contract fixtures use these secret-safe categories: `invalid-shape`,
`invalid-value`, `registry-unknown`, `minimized-data-violation`,
`tenant-mismatch`, `chain-mismatch`, `hash-mismatch`, and
`replay-evidence-invalid`. Implementations may map them to internal typed
errors, but errors and telemetry never include Event bytes, identifiers,
digests, protected content, or evidence diagnostics.

## 9. Follow-up ownership

- P03-07B owns the forward/rollback migration, Tenant head, append function,
  least-privilege database authority, and sqlc integration.
- P03-07C owns the Core store/service, retention/recovery adapters, concurrent
  append/idempotency tests, and checkpoint/export hook boundary.
- P03-08D owns the replay relation and command only after durable Approval and
  Audit reads exist.
- P11 owns Audit Viewer/export, legal-hold administration, and high-impact
  management UX.
- Platform/SRE owns external checkpoints, backup/PITR evidence, database-owner
  audit, and production retention jobs.

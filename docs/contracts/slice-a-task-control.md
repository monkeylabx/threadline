# Slice A Core Task control profile

Status: proposed business contract for [#347](https://github.com/monkeylabx/threadline/issues/347), under [#350](https://github.com/monkeylabx/threadline/issues/350). No implementation, schema, signed-Grant profile or production admission changes.

## 1. Responsibility and authority

The Core Task control module owns the durable business transitions of one Task and its Runs and Approvals. Its interface includes the authorization, ordering, retry and failure rules below, not merely the RPC types in [TaskService](../../proto/threadline/task/v1/task_service.proto) and [RuntimeGatewayService](../../proto/threadline/runtime/v1/runtime_service.proto). Transport, current-fact resolution, persistence and worker supervision are separate adapters.

Core is the only business writer. Gateway authenticates and relays through a reviewed Core interface; it has no domain database access or independent state. Desktop requests business operations; agentd reports bounded evidence. A renderer boolean, local activity frame, worker result, transport ACK or signed Grant alone cannot establish current authority or a durable Run outcome.

Inputs are an independently authenticated principal, validated command, current policy/resource facts and database time. Outputs are authorized current resource views plus stable committed identities. State, command receipt, audit, Run metadata and initial durable control work commit together or all roll back. Core returns success only after confirmed durable commit; an unconfirmed commit may already have succeeded and must be reconciled with the same key. The test seam is the same business command/read interface used by the RPC adapters, with deterministic fact, clock and persistence adapters; later PostgreSQL tests exercise the actual transaction.

This profile is independently reversible before implementation. Persisted/wire implementation requires separate compatibility and migration tasks. [ADR-0008](../adr/0008-plaintext-first-slice.md) remains a dev-stack delivery-order decision; production Device identity, E2EE, connectors and containment gates remain unchanged.

## 2. Bounded supported surface

Names below omit their package prefix. All calls authenticate before disclosing object existence. These are planned supported semantics, not claims that the current empty/stub entry points implement them.

| Need | Existing interface | Profile behavior |
| --- | --- | --- |
| Convert one message | `TaskService.CreateTask` | One active Channel, one authorized trigger event, immutable reference-only Context Manifest and a Task Thread inheriting the Channel's visibility; returns `OPEN`. Does not start a Run. |
| Read Task | `TaskService.GetTask` | Current parent authorization; returns opaque authored fields and metadata. |
| Start/retry work | `RunService.StartRun` | Explicit target Runtime bound to an authorized Device and selected Agent; creates a fresh attempt in `QUEUED`, pins the manifest and trusted route policy. No silent device fallback. |
| Read/observe durable work | `GetRun`, `ListRuns`, `StreamRunEvents` | Core metadata only; authorized cursor replay. Local C3 activity remains C1 `WatchRunActivity`. |
| Read pending action | `ApprovalService.GetApproval`, `ListApprovals` | Current Channel/Task visibility; list is restricted to this Task and the caller's own approver queue. No caller-selected foreign approver. |
| Raise pending action | Gateway `RequestApproval` → Core command | Current execution owner requests one exact prepared action; Core computes eligible approvers and returns one `PENDING` Approval. |
| Approve/reject | `ApprovalService.DecideApproval` | Human decision on that exact pending action; Slice A supports `ONCE` only. No local decision proxy. |
| Withdraw approval | `ApprovalService.RevokeApproval` | Current eligible approver revokes an existing approval; next protected use fails. |
| Cancel | `RunService.CancelRun` | Durable stop intent, then the confirmation rules in §6; response success alone does not mean `CANCELLED`. |
| Report execution | Gateway `SubmitRunEvents`, `CompleteRun` → Core commands | Current authenticated owner/fence, producer deduplication and legal transitions; never arbitrary state assignment. |
| Maintain authority | Gateway enrollment/heartbeat/lease methods | #348 defines their trusted binding, admission and current-fact ports; wire declarations alone authorize nothing. |

Slice A rejects DM parents, additional event/file/workspace references, Task editing/participant changes/manifest replacement, cross-Task listing, transfers and Artifact operations with `UNIMPLEMENTED`. Unlisted RPCs, including `ListTasks`, `UpdateTask`, `AddParticipants`, `UpdateContextManifest`, `TransferRun`, `ListArtifacts` and Runtime admin methods, are unsupported. Artifact publication and the Agent half of #260 remain successors. Unsupported fields/modes also fail explicitly; never silently drop them or fall back to a wider operation. Method authentication precedes this response.

One Task has one live Run at most (`QUEUED`, `RUNNING`, `WAITING_APPROVAL` or `BLOCKED`). One Run has one active prepared action at most, including an approved action whose consumption/outcome is not reconciled; a new request cannot evade an uncertain prior action. A retry after `FAILED` or `CANCELLED` is an explicit new `StartRun` command and a new Run; previous evidence is immutable. A completed Task does not reopen in this profile.

Runtime producer submissions support `RUN_STARTED` only in this subset. Other producer event types return `UNIMPLEMENTED`; approval/terminal metadata comes from the corresponding Core business commands, not client-authored canonical events. Run steps/artifacts remain empty. Local C1 activity can still show bounded turn output. Demonstration approvals authorize only trusted policy's synthetic no-effect action; real file/tool effects require their separate Connector/containment admission and stay denied here.

Identifiers and references are exact tenant-composite keys, not authority. Initial limits: 128 UTF-8 bytes per identifier, 65,536 encoded bytes per authored `EncryptedField`, 16 participants, 128 events per submission/stream batch and 100 rows per list page (default 20). Zero/unknown enum values, invalid UTF-8, duplicate participant identities, untrimmed/control-character IDs and over-limit inputs are rejected, not repaired. Stored sequences/versions are positive signed-bigint values with checked increments; JSON fixtures encode them as strings.

## 3. Visibility and current permission

Use the [Authenticated Principal and authorization language](../../CONTEXT.md), existing [current-fact resolver](../../services/core/internal/authorization/current/current.go) and [Grant signature contract](../architecture/capability-grant-signature-contract.md). Every read, duplicate receipt read, stream emission and mutation rechecks active Session/Device, Organization/Member, Channel Membership, Role ceilings, current ACL/policy and operation-specific facts. Authorize and mutate under the same reviewed transaction/locking discipline; a persisted allow decision is never a permission cache.

| Operation | Required current authority in addition to authentication |
| --- | --- |
| Task creation | Human with parent `channel.publish`; trigger belongs to that Channel and is individually readable in the current membership/history interval; target Runtime/Device and Agent assignment allowed by trusted policy. |
| Task/Run/Approval metadata read | Parent `channel.read`; participation does not grant private visibility. An authorized Channel reader can observe shared Task metadata without becoming execution owner or approver. |
| StartRun | Human Task `OWNER`, parent publish permission, current policy permitting that Agent/Runtime/Device and requested capability intersection. |
| CancelRun | Human Task participant with current policy control permission and parent read permission. The `OBSERVER` role may interrupt when policy allows, but cannot thereby approve. |
| Decide/revoke | Human in both the stored eligible-approver set and its currently recomputed policy/Task-role intersection; parent read and authority over every requested resource. |
| Runtime writes | Independently authenticated Runtime/Device acting for the Run's assigned Agent, exact current execution owner, lease/fence and current authority. Cleanup reports and terminal receipt reads use the restricted reconciliation authority in §6, not an expired/revoked execution Grant. A Human Task owner is not the execution owner. |

Core derives participants from trusted policy: creator `OWNER`, one permitted Agent `ASSIGNEE`, configured Humans `APPROVER` or `OBSERVER`; request participants must match this allowed set, and timestamps are Core-assigned. If dev policy permits the creator to approve, keep one `OWNER` participant and explicitly include that role in approval eligibility; do not add a duplicate participant. Later participant changes are unsupported. Requesting an approver role, owning a Channel or having an Organization admin role cannot manufacture approval/resource access. Empty eligible-approver intersection rejects the request without creating a pending action; unavailable facts return `UNAVAILABLE`. The owner must not perform the protected action.

Approval eligibility is snapshotted for explainability and intersected with current policy at decision/use time. Policy changes can remove eligibility, never silently add a new approver to an old request. Self-approval is permitted only by explicit trusted dev policy, visibly `NOT PRODUCTION`; absent such policy it is denied. This does not change recovery's separate no-self-approval rules.

The existing evaluator knows only Space/Channel actions; it does not already implement Task authority. The planned business actions are `task.create`, `task.read`, `run.start`, `run.read`, `run.control`, `approval.read`, `approval.request`, `approval.decide`, `approval.revoke` and `run.report`. G2 below must register their parent checks and Task-role rules before they can be allowed. Unknown action/policy/resource facts fail closed.

## 4. Content and exact action

Core treats title, goal, justification and decision note as opaque `EncryptedField` values; it never decrypts, indexes, logs or copies them into events/audit. The current type promises ciphertext and a real Crypto Profile: do not insert plaintext into `ciphertext`, forge a group/epoch, or treat an unknown profile as unencrypted content. ADR-0008's opaque message bytes do not silently redefine `EncryptedField`. A separately reviewed Slice A authored-field encoding is G4; until integrated, any incompatible dev input is rejected.

The manifest stores the one authorized trigger reference, parent identity, creator and policy version. A Run pins that manifest identity/version. Permission is rechecked when locald resolves the reference; creation-time access cannot preserve access after revocation. Only the authorized bounded local context adapter supplies initial goal/content to agentd. Core/Gateway carry references, not decrypted message history, file bytes, Prompt or model output. No physical path or user-file access is granted by this profile.

One Approval binds `(tenant, Task, Run, Agent, execution Device/Runtime, owner generation/fence, prepared-action identity/version, exact-action binding, requested capabilities/scope, policy version, expiry)`. Core derives identity fields from trusted facts. The runtime must hold that same prepared action immutable. Changed arguments, destination, action version or scope require a new request; an old decision cannot authorize them. #348 freezes the versioned action-binding encoding and use-time check; raw arguments and unreviewed hashes of sensitive/low-entropy values cannot enter C2 audit/events.

`approved=true` accepts or narrows the request only. Empty `granted_capabilities` means exactly the requested capabilities, per existing Proto; an empty ResourceScope grants nothing and is never a wildcard/default. Granted scope/capabilities must fit the request, initiator, approver, Agent delegation and current resource policy. Slice A `ONCE` grant expiry is the minimum of request expiry, current parent authority and a trusted policy ceiling; no in-place renewal. Task/time-bounded approval modes are unsupported. A denial issues no Grant. The fixed v1 signed projection is unchanged; the current Core Approval/issued-Grant relation and use-time exact-action check are also required.

## 5. State transitions

State names abbreviate existing enums. Transitions not listed are rejected. Core serializes all transitions on the Task/Run, including cancellation, authority loss, expiry and owner reports; expiry is evaluated with trusted time under that same serialization, not a delayed background job. Expiry reconciliation may commit its own audit/event/control fact before rejecting a late client command; the rejected command receives no success receipt or partial requested writes.

| Business fact committed by Core | Run | Task | Approval / condition |
| --- | --- | --- | --- |
| CreateTask | none | `OPEN` | No automatic Run. |
| StartRun | new `QUEUED` | `IN_PROGRESS` | Attempt increments once; target remains fixed even offline. |
| Owner admission/start | `QUEUED → RUNNING` | `IN_PROGRESS` | Current Core admission/lease/grant/route; not a dispatch receipt. |
| RequestApproval | `RUNNING → WAITING_APPROVAL` | `WAITING_APPROVAL` | New `PENDING`, no execution permission. |
| Approve | `WAITING_APPROVAL → RUNNING` | `IN_PROGRESS` | `PENDING → APPROVED`; no cancel intent, unexpired/current authority; effect still needs trusted delivery/use check. |
| Reject | remains `WAITING_APPROVAL` until cleanup | `BLOCKED` | `PENDING → DENIED`, atomically records cancellation intent; no Grant. |
| Pending Approval expires | `WAITING_APPROVAL → BLOCKED` | `BLOCKED` | `PENDING → EXPIRED`; no automatic retry. |
| RevokeApproval / authority lost | live Run → `BLOCKED` | `BLOCKED` | `APPROVED → REVOKED` where applicable; invalidate uses, stop new effects. Cancellation instead follows the next row. |
| Cancel admitted/live work | state unchanged until report | `BLOCKED` | Durable cancellation intent outranks the visible activity state. |
| Verified successful owner completion | live Run → `COMPLETED` | `COMPLETED` | No pending action/cancel intent; owner cleanup verified. |
| Verified owner failure | live Run → `FAILED` | `BLOCKED` | Cleanup verified, or explicit `cleanup_failed` evidence; stable minimized failure reason, no cancellation claim. |
| Verified stop confirmation | live Run → `CANCELLED` | `BLOCKED` | Existing cancel/denial intent; owner cleanup verified. |
| Explicit Task retry | old terminal Run unchanged; new `QUEUED` | `IN_PROGRESS` | Allowed only after `FAILED`/`CANCELLED`, with a new command identity. |

A cancelled Run does not cancel the collaborative Task: Task stays `BLOCKED` so a Human can explicitly retry. `Task.CANCELLED` and direct Task-state editing are outside this subset. Denial and user cancellation converge on `Run.CANCELLED` after the same stop confirmation, even though the Approval remains `DENIED` after denial.

An expired/revoked approval cannot be restored to `PENDING`/`APPROVED`. A `BLOCKED` Run never resumes automatically after policy/lease recovery; cancel/reconcile it, then explicitly start a fresh Run. Terminal Runs are immutable; exact duplicate terminal reports are receipts, conflicting reports fail. All unconsumed Grants are revoked when the Run terminates. Pending Approvals become `EXPIRED` on terminalization unless already `DENIED`; an approved unused Grant is revoked with its Approval. Task retry does not reuse any previous Approval, engine Session or uncertain local input.

## 6. Cancellation, races and use

Cancellation intent is durable control metadata, not a new overloaded Run enum. It has a stable identity, cause (`user_cancel` or `approval_denied`), Core order/version and recorded time. The first intent wins; later exact/control-equivalent cancel requests share that intent. A future typed response projection must show the pending intent (§9 G1); until that exists, `RUNNING` or an RPC success must never be rendered as confirmed cancellation.

For queued work, Core may confirm `CANCELLED` immediately only in the same transaction that proves no execution authority was ever admitted and invalidates all outstanding dispatch/claim authority. A disconnected Gateway, missing ACK, `QUEUED` label or empty process list is not that proof. If authority might have been admitted, keep the intent pending and require reconciliation/verified owner confirmation. This never-admitted exception needs an explicit service-comment/protocol clarification in G1; it is not implemented by today's CancelRun declaration.

For admitted work, only a trusted owner/supervisor report identifying the intent and current owner generation/fence can confirm cleanup and commit `CANCELLED`. An expired execution lease denies new effects; it must not make safe shutdown impossible: #348 must define narrowly scoped authenticated reconciliation authority for cleanup reports, never authority to resume work. The old/stale owner cannot confirm a replacement owner's cleanup. A failed cleanup reports `FAILED` with `cleanup_failed`, not `CANCELLED`; unavailable Core or unacknowledged failure remains visibly unconfirmed. ACP cancellation and a local stop receipt do not prove descendant cleanup.

| Serialized race | Required outcome |
| --- | --- |
| Cancel/deny commits before approve | Late approval fails; no Grant issuance/use or revival. Pending request becomes `EXPIRED` for user cancellation, `DENIED` for explicit rejection. |
| Approve commits before cancel | Cancel revokes the approval/Grant and records one stop intent. A delivered old approval cannot begin another action. |
| Action admission commits before cancel/revoke | An already admitted bounded effect may have happened; no rollback promise. Recheck at each subsequent protected effect; supervisor stops remaining work. |
| Cancel intent commits before completion report | Reject `COMPLETED`; require verified stop/failure report. A terminal `FAILED` cleanup report may win over cancellation. |
| Verified completion commits before cancel | Return the existing terminal Run without new intent, effect, event or state rewrite. |
| Expiry races with approval/use | At `now >= expires_at`, deny regardless of an unprocessed expiry notification. |

Protected use has a Core-serialized consume/admit decision for the exact prepared action and `ONCE` approval. This is a required internal Core port for #348 to specify, not an existing public RPC. It rechecks current membership/policy, approver authority, Grant state/expiry/signature binding, Run/cancel state, Device, owner and lease/fence. One consume identity may be retried to recover its receipt, never to re-execute an uncertain external effect. End-to-end exactly-once external effects are not promised; uncertain worker outcomes require reconciliation. If current Core cannot be reached, effects fail closed; ordinary IM and authorized local supervisor stop remain available.

## 7. Durable retry identity

Every mutating command needs a stable operation identity in a future additive wire contract (G1), including CreateTask, StartRun, RequestApproval, DecideApproval, RevokeApproval, CancelRun and CompleteRun. A bearer credential, HTTP request/correlation ID, local input ID or Run ID alone is not that identity. An ID is 1–128 printable non-space ASCII bytes and is generated/persisted before first send. Retries preserve it and the exact authored/encrypted bytes; a new encryption of the same text is a different command.

Human-command uniqueness is `(tenant, Actor type/id, authenticated Device, RPC, operation_id)`. Session is reauthenticated on every attempt but is not in the namespace, allowing authorized same-Device recovery after login. Runtime uniqueness additionally binds authenticated Runtime, Run and owner generation. A client changing target identifiers/fields does not gain a new namespace for the same key. No request field chooses the trusted identity.

Compare a versioned canonical typed request projection: every caller-set semantic field, oneof/presence, ordered repeated value, enum and opaque byte value is included; transport framing, bearer credentials and Core-assigned timestamps/IDs are excluded. Sets have a single registered sorted/unique representation; unknown fields/modes are rejected, not discarded. G1 must freeze the field-by-field encoding and cross-language vectors before implementation. Store protected exact request identity or its reviewed integrity commitment only in the scoped receipt store; never in audit/telemetry. No request plaintext/secret hash shortcut is allowed.

| Command/retry | Durable rule |
| --- | --- |
| CreateTask exact repeat | Same Task/Thread/Manifest identities; no second creation. |
| StartRun exact repeat | Same Run/attempt, even after completion; does not count as a Task retry. |
| RequestApproval exact repeat | Same Approval/action; no second pending request. A new key cannot evade the one-pending-action rule. |
| Decision/revoke/cancel repeat | Same issuance/revocation/intent identities; never renew, extend, reissue or execute. Repeated equivalent cancellation with a new key still creates no second stop intent. |
| CompleteRun exact repeat | Same terminal fact; altered state, usage, cleanup proof or reason conflicts, even with a fresh key. |
| Same key, changed request | `IDEMPOTENCY_CONFLICT`; first committed facts remain. |
| Lost ACK / Core restart | Reauthenticate and resend the same key; return the committed identities or commit once if absent. Never infer rollback from a timeout. |
| Authorization removed before retry | Deny receipt/resource disclosure; do not create anything or return cached permission/Grant authority. |

Receipts return immutable outcome/resource identities plus the current authorized resource view, not a stale serialized allow response. After authentication/shape/current disclosure checks, resolve an existing command binding before applying new-transition preconditions: a duplicate StartRun must not fail merely because its Run is now terminal. Receipt reads require current visibility/receipt-read role, not a new execution lease or Grant issuance. An old Approval receipt cannot restore expired/revoked authority. Successful no-ops also bind their operation key/result atomically, without another business event or intent. Failed preconditions do not reserve a success receipt; same-key retries re-evaluate current facts. Unique command bindings and business constraints serialize competing requests; a fresh key cannot evade terminal-state, active-Run, pending-action or Grant-consumption rules.

Retain command bindings/tombstones with the resource's reviewed retention policy; never evict one merely to admit a new request. After content removal an authorized same-key request receives `FAILED_PRECONDITION / RETENTION_EXPIRED`, never a recreated Task/Run. Minimal uniqueness metadata must remain, or the entire command namespace must be permanently closed before its tombstones are removed; retirement cannot reopen old keys. G5 must freeze this lifetime/erasure policy before any receipt purge; it is not permission to retain C3 forever. Quota/overflow returns a bounded failure with no partial mutation.

## 8. Persistence, events and errors

Minimum separate storage seams are Task/Manifest/participant identity; Run attempt/current owner/authority and cancellation intent; Approval/exact-action/eligible-snapshot/issued-Grant relation; command and producer receipts; Core Run metadata head/events; existing audit and Domain Event/Outbox stores. Every child key/FK/unique constraint includes Tenant. Prevent two live Runs per Task, two pending actions per Run, duplicate attempts, reused command identities and conflicting terminal facts in storage, not only handler memory.

Core allocates canonical positive RunEvent sequences/identities under the Run lock; clients cannot allocate or overwrite that ordering. Runtime submissions need a separate producer identity/position and receipt mapping to canonical Core sequence (G1/#348), including gap/conflict rejection. Existing `SubmitRunEvents.events` is not already such a mapping. Core-generated approvals/cancellation and runtime reports share one contiguous committed Run history. Duplicate receipt reads allocate no new event/time/sequence. Checked overflow or failed audit/outbox insert rolls the entire command back.

`StreamRunEvents(after_sequence)` emits only authorized committed sequences strictly greater than the cursor, in bounded batches. Cursor greater than the head is `CURSOR_INVALID`; a missing required retained segment is `SEQUENCE_GAP`, never silently skipped. Clients advance only after applying a batch; reauthorization failure ends the stream. Local C1 sequence/incarnation and Channel sync cursors are distinct and cannot substitute for this cursor.

Each successful business change appends minimized audit and metadata Domain Event(s) plus initial delivery records in its transaction, following the existing [audit](../architecture/audit-retention-contract.md) and [outbox](../architecture/transactional-outbox-contract.md) contracts. Delivery records contain identities/versions and approved metadata, not authored fields, raw action arguments or reusable Grant bytes. A notification wakes a current authorized Core read; it is never a Grant. No message-Event encoder, broker payload or gateway cache becomes Task authority. Broker outage delays delivery, not a durable Core commit.

G3 must extend the closed registries before writes: audit actions `task.create`, `run.start`, `run.cancel.request`, `approval.request`, `approval.decide`, `approval.revoke`, `approval.consume`, `run.report`; target types `task`, `run`, `approval`. Metadata event types are `task.created`, `run.queued`, `run.started`, `run.approval_required`, `approval.decided`, `approval.revoked`, `approval.consumed`, `run.cancel_requested`, `run.blocked`, `run.completed`, `run.failed`, `run.cancelled`, each with a versioned Task/Run/Approval aggregate descriptor and logical destination. Existing generic storage/Message encoder does not authorize these names. Denied/failed command audit, when policy requires it, is a separately durable minimized attempt record and cannot convert a rollback into a success receipt or invent an unknown target.

| Failure | Connect/gRPC status; existing detail code | Retry |
| --- | --- | --- |
| Missing/invalid authentication | `UNAUTHENTICATED`; no object detail | Authenticate; no object disclosure. |
| Unauthorized member/action/approver | `PERMISSION_DENIED`; `PERMISSION_DENIED` / `NOT_A_MEMBER` | No blind retry or cached receipt; omit unauthorized subject. |
| Unknown or foreign-tenant lookup | `NOT_FOUND`; no subject/detail identifying a foreign object | Tenant-derived lookup; indistinguishable from absence. |
| Invalid IDs/enums/shape | `INVALID_ARGUMENT`; no new invented domain code | Correct input. |
| Unsupported method/mode | `UNIMPLEMENTED`; no parallel error envelope | Wait for reviewed support; no fallback. |
| Current facts unavailable | `UNAVAILABLE`; no fabricated allow/detail | Same key only; effects remain denied. |
| Request too large / quota | `RESOURCE_EXHAUSTED`; `PAYLOAD_TOO_LARGE` / `RATE_LIMITED` | No partial write. |
| Reused key or producer ID, changed facts | `ALREADY_EXISTS`; `IDEMPOTENCY_CONFLICT` | Reconcile original; never overwrite. |
| Terminal/race/expired Approval | `FAILED_PRECONDITION`; `INVALID_STATE_TRANSITION` | Read current state; do not revive. |
| Expanded scope / rejected approval use | `PERMISSION_DENIED`; `GRANT_OUT_OF_SCOPE` / `APPROVAL_DENIED` | New reviewed authority/action, not another flag. |
| Lost owner/lease/fence | `FAILED_PRECONDITION`; `NOT_EXECUTION_OWNER` / `LEASE_LOST` / `FENCING_TOKEN_STALE` | Stop effects, reconcile; no old-writer completion. |
| Expired/revoked Grant | `PERMISSION_DENIED`; `GRANT_EXPIRED` / `GRANT_REVOKED` | No cached or duplicate allow. |
| Unreviewed/unknown authored-field profile | `FAILED_PRECONDITION`; `CRYPTO_PROFILE_UNSUPPORTED` | No plaintext fallback or reinterpretation. |
| Future cursor / required sequence gap | `FAILED_PRECONDITION`; `CURSOR_INVALID` / `SEQUENCE_GAP` | Reconcile from an authorized checkpoint. |
| Durable commit not confirmed | `UNAVAILABLE`; `NOT_DURABLE` | Same command identity only; outcome may have committed. |

An offline target is a successfully queued Run, not a fabricated execution ACK or a reason to retarget. Error details follow the [shared error model](./error-model.md), never contain content, credentials, paths, raw requests, SQL or worker logs. `retry_after` is present only for a safe same-operation retry; its absence is not permission to generate a new key.

## 9. Integration prerequisites and acceptance trace

These gaps block implementation; this document allocates no wire field numbers or migration IDs and does not change the v1 signed Grant projection. Each must be tracked as a bounded successor under #350 before the final contract task closes.

| Gap / owner | Required successor |
| --- | --- |
| G1 — Contracts/Integration, #355 | Additive operation identity/typed canonical request vectors, cancellation-intent/read projection, exact-action/decision version/expiry fields, producer-vs-Core event mapping, explicit Run metadata vocabulary for cancellation intent/blocking/consumption, and never-admitted cancellation clarification. CompleteRun needs intent/owner-generation/cleanup evidence; current fields cannot prove it. |
| G2 — Contracts/Core, #354 | Registered business actions/current parent/participant/approver policy, target Runtime–Agent–Device binding, revocation and disclosure rules, with positive/negative decision fixtures. |
| G3 — Contracts/Integration, #356 | Minimized audit/action/target and Domain Event descriptors, control destinations and payload versions; neither initial closed registry accepts Task/Run/Approval implicitly. |
| G4 — Contracts/Client-core, #353 | Explicit authored-field encoding for the plaintext dev profile, preserving opaque server handling and production Crypto Profile rules; current EncryptedField cannot be silently redefined. |
| G5 — Integration migrations | Separately reserved Task/Manifest then Run/Approval/receipt/control schemas, current-fact locks, uniqueness/retention/tombstones and generated sqlc output. |
| G6 — Core | Separate Task, Run, Approval and Runtime ingress/control implementations plus RPC/wiring tasks within review budgets; business logic, persistence and transport stay at distinct seams. |
| G7 — #348 / Runtime | Authenticated control/admission, exact action/ONCE use, replay/ack, route/lease/grant freshness and cleanup reconciliation protocol; no effects until reviewed and implemented. |
| G8 — #349 / Client-core/Desktop/Runtime | Trusted per-window/Session OS peer bootstrap and bounded initial-context adapter; fixtures cannot supply real principal/permission facts. |

| #259 acceptance | Contract source / downstream evidence needed |
| --- | --- |
| Message→Task, exact pending action | §§2–4, G1–G6; real Core command/read and authorized context tests. |
| Approve only after trusted current decision | §§3–7, G7; Gateway/agentd use-time and changed-action/revocation tests. |
| Reject/cancel converge on confirmed terminal Run | §§5–7; cleanup failure, queued-never-admitted and late-approve/complete races. |
| Second window isolated, reauthentication invalidates watch | C1 and G8/#349; actual host/IPC tests, separately reported native-window evidence. |
| IM usable when agentd/Core authority path unavailable | §§1,6; independent existing IM path and denied-effect/local-stop tests. |
| No UI/ACP/local event becomes authority | §§1,3,8 and G7/G8; real adapters with forged/wrong-principal/stale-fence negative cases. |

## 10. Reference evidence and verification

[scenarios.json](../../test/fixtures/proto/slice-a-task-control/scenarios.json) is a synthetic expectation table, **not an executable state machine, wire message, runtime authority adapter or proof of implemented behavior**. `given` overlays declared trusted test facts on defaults; absent facts inherit defaults, not permissive runtime fallbacks. `then` is the expected result of the named operation/ordered race. All IDs/content are synthetic; time is fixed. A later independently implemented command interface must satisfy these cases, including denial and no-extra-write assertions.

For this document-only task run `node scripts/proto.mjs check`, `make -C db migration-static`, `python3 -m json.tool test/fixtures/proto/slice-a-task-control/scenarios.json` and `git diff --check`. Manually trace the permission, state, retry and race tables against the reference expectations. Existing tests and JSON parsing do not execute this new business profile. New executable conformance tests/registry wiring belong to a separately bounded task after #335; no legacy aggregate-verifier growth is authorized here.

Integration may evaluate #348 readiness after this profile lands. #350 and #259 stay blocked on their implementation and real-boundary acceptance. Do not close the Slice A map or grant protected-tool/macOS production admission from profile/fixture success.

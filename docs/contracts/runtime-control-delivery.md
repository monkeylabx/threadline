# Slice A trusted Core-to-Runtime control delivery

Status: proposed profile for [#348](https://github.com/monkeylabx/threadline/issues/348), under [#350](https://github.com/monkeylabx/threadline/issues/350). Baseline: `9d5767664bcc089b709bd9188e9bbb33f843182b` (#347/#357). This specifies interfaces, not implemented transport, executable conformance, or production admission.

## 1. Responsibility and invariants

The control-delivery module carries Core-committed Run control facts to their exact Runtime and returns authenticated receipts. Core owns ordering, authority, admission, action consumption, cancellation and terminal outcomes; Gateway is a stateless relay through a versioned Core interface, with no Core database access. Runtime owns bounded local supervision and evidence, never a second business state machine. The [Core business profile](slice-a-task-control.md) controls all Task/Run/Approval transitions.

Inputs: independently authenticated Runtime identity, committed control records, current Core facts, stable operation identities and trusted supervisor evidence. Outputs: bounded reference-only delivery, checkpoints, fresh authorized snapshots and durable command receipts. Failures: authentication/authorization loss, gap, overflow, uncertain commit/effect, unavailable facts or cleanup. Every failure denies new effects; local supervisor stop and ordinary IM remain available. The test seam is this same interface with deterministic identity, clock, transactional store and supervisor adapters; PostgreSQL and authenticated-stream tests must later exercise the real seams.

This document and its declarative fixtures are independently revertible. Wire, persistence, host bootstrap and worker changes are separate tasks. No Gateway cache, dispatch ID, stream receipt, heartbeat, renderer flag, ACP notification or signed Grant alone supplies current authority.

## 2. Identity and connection

Production retains client-initiated gRPC/mTLS and independently authorized Device proof; this profile does not implement or waive them. Core binds `(tenant, operator Human, authorized Device, runtime_id, runtime_generation, credential identity/version)` from trusted verification and enrollment state. The assigned Agent is a separate Run fact. Request fields are equality checks against this principal, never its source. A Runtime is one operator's Device, not a shared execution pool. Tool/connector advertisements describe capacity only.

The explicit `NOT PRODUCTION` dev deployment uses server-authenticated TLS and a separate high-entropy Runtime bearer credential supplied to the supervised host by operator-managed secret configuration. A trusted verifier maps that credential to a preauthorized dev Device/operator and allowed deployment; Core atomically assigns/reuses one Runtime identity for that binding. Human session credentials have the wrong audience. Credential IDs, generation, expiry and revocation are server facts; secrets never enter renderer configuration, URLs, control records, fixtures or logs. Disabled/unknown/mixed dev profiles fail closed; a request flag cannot enable dev mode or downgrade a production endpoint. Rotation revokes the old credential and fences its streams; replacement enrollment cannot bypass unresolved prior work.

Only Runtime initiates connections to configured trusted Gateway endpoints. Gateway authenticates its own restricted service identity to Core and passes Runtime evidence; Core re-verifies the Runtime credential or independently verifiable peer attestation, audience, binding and current enrollment on every operation. Gateway's service identity alone cannot assert a Runtime. External caller-supplied identity headers are stripped. TLS peer/certificate failure has no insecure fallback. Core is the sole writer of enrollment/authority state; Gateway may retain only bounded in-memory connection buffers.

One current control stream per Runtime generation has a Core-issued connection epoch. A new authenticated stream fences the previous epoch for delivery/ACK, but does not transfer Run ownership. Disconnect does not move queued work, extend a lease, or start a replacement. Missing control freshness pauses new effects; each effect requires a live Core check regardless of stream connectivity. Heartbeats cannot renew execution authority. Runtime revocation, Device/session-policy loss or unknown facts stop effects and clear local content under C1; cleanup uses only §7's restricted path.

Dev connection/TLS establishment and individual Core commands have trusted five-second deadlines; no partial output/allow escapes timeout. Control stream reconnection uses operator policy with exponential delay capped at 30 seconds and bounded jitter, preserves operation IDs/checkpoints, and never retries execution. A missed 30-second heartbeat marks connectivity offline only; it cannot renew authority, certify cleanup or retarget queued work. Production timing remains separately configurable with the same denial semantics.

## 3. Concrete interfaces and wire gaps

The names below specify planned semantic operations at version `runtime-control/1`; they are not a parallel executable protocol. Additive Protobuf work must place Runtime-facing operations on `RuntimeGatewayService`, preserve existing field meanings and generate bindings through the pinned toolchain. Gateway and Core use the same typed payloads on a separately authenticated internal interface. Unknown versions/kinds are refused, not translated to initial dispatch.

| Interface | Request → response; obligation |
| --- | --- |
| Existing `EnrollRuntime`, `Heartbeat` | Verified binding → Core enrollment/liveness receipt. IDs and capacity never grant execution. |
| Planned `ReceiveControl` | Runtime/generation, last Core checkpoint → server stream: ready header then ordered `ControlEnvelope` records. Ready binds connection epoch, floor/head and checkpoint; contains no execution authority. |
| Planned `AcknowledgeControl` | Epoch, exact delivery ID/sequence and stable receipt identity → Core-committed cumulative checkpoint. Means handled notification only (§4). |
| Planned `ReadRunAuthority` | Run ID and minimum observed control revision → current authorized `RunAuthoritySnapshot`. Linearizable Core read; stale replicas/caches cannot satisfy it. |
| Planned `AdmitRun` | Operation ID, dispatch ID, pinned Run/owner/manifest/route revisions → durable admission receipt. Core marks ever-admitted before any context/model/worker activity. |
| Existing `RequestApproval`, planned `AdmitAction` | Immutable prepared action (§5), operation ID and owner proof → Approval ID; then a separate Core-serialized ONCE-consumption receipt. No action starts from a decision notification. |
| Existing `SubmitRunEvents`, `CompleteRun` | Stable producer/report identity, owner generation/fence and typed evidence → canonical Core sequence mapping/terminal receipt. Existing fields are insufficient (§7). |
| Planned `ReportCleanup` | Restricted reconciliation identity, original owner/worker, intent/barrier identity and verified evidence → minimized durable report receipt. No execution or general read authority. |

`ReceiveDispatch`/`known_dispatch_ids` remain initial-work compatibility declarations, not control replay or admission. A version-1 participant uses the reviewed control stream only; running both consumers for one generation is rejected. Legacy clients that cannot honor this profile receive no runnable dispatch. `TaskDispatch` lacks explicit recipient Runtime/Device/owner generation and immutable manifest version; #355 plus D1 must add them. Expired never-admitted dispatches may receive a new dispatch identity for the same queued Run; prior admission or uncertainty forbids automatic redispatch.

Existing `RequestApproval` lacks operation identity, owner generation/fence, action identity/binding and request expiry. `CompleteRun` lacks report identity, intent, worker identity and cleanup evidence. `SubmitRunEvents.last_committed_sequence` is Core history, not producer position. Do not overload any of those fields, `failure_reason`, or `UsageLedger` to carry missing proof. Run/control/producer sequences and C1 local activity cursors are separate namespaces.

### Envelope and current snapshot

Every envelope binds profile, tenant, Runtime/generation, delivery ID, positive per-recipient sequence, Task/Run, Agent, Device, owner generation/run fence, Core control revision, kind and Core commit time. Kinds are `dispatch_available`, `approval_changed`, `authority_invalidated`, `cancel_requested`, `run_terminal`. Approval hints add Approval/action/decision versions and expiry; cancel hints add the immutable intent identity/version/cause. Hints contain only authorized C2 metadata and references, never authored fields, arguments, reusable Grant bytes, credentials, context or model output.

`RunAuthoritySnapshot` binds the same identities plus current Run state/control revision, manifest identity/version, owner lease identity/expiry, cancellation intent or explicit absence, safety barrier identity/status, Grant identity/state/expiry, Approval/action/decision/consumption state, current policy/permission versions, trusted route reference/expiry and remaining budget. Its checked projection feeds C1's `AuthoritySource`; cached booleans are not evidence. Per-window observer/controller facts still come from #349's independently authenticated local principal and a current Core authorization read. Runtime owner authority cannot manufacture a Human observer/controller.

Slice A has no Workspace/file access. It needs an explicit Core-owned Run execution lease/generation/fence, including for no-workspace Runs; do not invent a Workspace or reinterpret `WorkspaceLease` as that lease. Workspace fencing, when later supported, is additional. A snapshot is explanatory evidence, never a reusable authorization ticket. Any changed/missing fact denies the next use; Core atomically checks and admits the operation under the same serialization as cancel/revoke.

## 4. Ordering, ACK and replay

Core assigns a contiguous sequence per `(tenant, runtime_id, runtime_generation)`, independently of Run metadata history. The corresponding control record, aggregate revision, audit/outbox fact and command receipt commit atomically with the business mutation. Gateway/NATS wake delivery but are not the durable queue. Fan-out to multiple recipients creates distinct delivery records referencing the same fact. Duplicate notifications never allocate another business event or extend authority.

| Limit / checkpoint | Version-1 rule |
| --- | --- |
| IDs / numbers | Identifiers: 1–128 UTF-8 bytes, no controls or surrounding whitespace. Versions/sequences: checked positive signed-bigint range, JSON decimal strings. No wrap or float conversion. |
| Stream buffer | At most 32 unacknowledged envelopes and 256 KiB encoded bytes per connection; each envelope ≤16 KiB. Stop reading/sending under backpressure; never drop an unseen record and advance the cursor. |
| Durable queue | Reserve 128 control slots atomically at StartRun, including queued work; at most eight outstanding Run assignments per recipient generation (1,024 slots). Limit each Run to eight dispatch issuances and 16 prepared actions. Each action can notify pending/decision/consumption/revocation once; first invalidation/cancel/terminal and reconciliation fit the remaining reserve. After first invalidation, further narrowing changes require current reads, not repeated hints or restored execution. Duplicate facts allocate no slot. At capacity reject new StartRun/action/dispatch issuance before mutation; first cancel/revoke/terminal retain their reserve. Release an assignment only after terminal reconciliation and its checkpoint. Unexpected reserve exhaustion is a fail-closed operator fault, never discarded cancellation. |
| Ordering | Apply only next contiguous sequence. Buffer within the stream bound or terminate with a gap; never reorder approvals around invalidation. A stop hint may stop the supervisor immediately even during repair, but cannot advance a missing checkpoint. |
| ACK | After validating identities and recording the handling outcome, ACK exact ID/sequence. Core verifies it was delivered in this epoch and all predecessors handled, then persists a monotonic checkpoint. Future, cross-generation and altered ACKs fail; exact duplicate ACK returns its receipt. |
| Retention | Unacknowledged records cannot expire silently. Keep receipt/tombstone uniqueness until the resource namespace is permanently retired (§7 of Core profile). Purge only a verified checkpointed prefix under reviewed retention. A required missing segment is `SEQUENCE_GAP`; future/foreign cursor is `CURSOR_INVALID`. |

`handled` means a hint was checked and its current Core facts reconciled, possibly to a stale/no-op outcome; it never means admitted, executed, approved or terminal. Run admission, action consumption, local execution evidence and durable terminal commit each have separate identities/receipts. Lost ACK: resend identical receipt, do not repeat an effect. Gateway crash: reconnect to Core's checkpoint. Core commit timeout: same operation identity only; a lost response does not prove rollback.

A Runtime keeps only bounded non-content receipt metadata; C1 intake/activity and engine Session stay volatile. If no durable local checkpoint survives restart, replay from Core's checkpoint and re-read every affected Run. A new agentd incarnation cannot resume old intake, prepared-action secret or engine Session. Previously admitted work with missing outcome stays blocked behind the Core safety barrier, even if its dispatch was ACKed. Replayed initial work cannot launch another turn.

Version 1 has no snapshot-reset shortcut for a lost required segment. Gap/unknown kind stops admission, reconciles existing owners and requires operator/retention repair; never skip to head or silently enroll a fresh generation. Control streams expose only the recipient's authorized metadata. Revoked principals cannot use retained envelopes to read old content; narrow cleanup delivery reveals only identities already bound to their original cleanup assignment.

## 5. Exact action and current admission

Only trusted policy's synthetic `noop/1` action is executable in the demonstration profile; its argument encoding is an empty byte string and its scope contains only the configured synthetic tool ID. Unknown adapters, real tools, user files and other argument encodings return `UNIMPLEMENTED`. They require separate Connector/containment profiles. A synthetic success is not permission for a real Goose protected Run.

The trusted prepared-action adapter freezes `(action_id, action_version, adapter_id/version, operation, capability set, requested ResourceScope, argument_encoding, exact argument bytes)` before requesting Approval. Version-1 action binding is HMAC-SHA-256 with a fresh 32-byte local random key per prepared action. The key and arguments remain in protected volatile owner memory; losing them requires reconciliation, never regeneration under an old Approval. Core retains only the 32-byte commitment in its restricted action/receipt store, not audit/telemetry. An unsalted hash of low-entropy content is forbidden. This commitment checks equality; it is not authentication, a Grant or proof of a protected tool's correctness.

The HMAC transcript is the fixed ordered tuple: domain `threadline.action-binding/1`, tenant, Task, Run, Agent, Device, Runtime, Runtime generation, owner generation, run fence, action ID/version, adapter ID/version, operation, sorted capability numbers, requested scope, argument encoding, exact argument bytes. Strings/bytes are `u32be(length) || bytes`, integers are `u64be`, lists are `u32be(count)` followed by encoded entries. Scope uses the eight `ResourceScope` fields in declaration order, each an encoded list of UTF-8 values sorted by unsigned byte order; duplicates/unknown fields/invalid UTF-8 are rejected. No normalization, omitted default, unordered JSON or transport serialization can change the transcript. D1/#355 must add cross-language golden vectors before use; v1 grants retain their existing signed projection.

Core's `AdmitAction` transaction verifies authenticated recipient, current Device/operator/Agent delegation, Channel/Task permission, eligible approver still current, Approval APPROVED/ONCE/decision version/expiry, exact commitment/action binding, narrowing scope, live Run with no cancellation/barrier, current owner/lease/fence, route and budget. It atomically consumes once and commits a stable consumption identity plus audit/control fact. Duplicate receipt lookup is authorized separately and returns `already_consumed`, never another execution ticket; changed facts conflict. A fresh operation ID cannot consume the same action again. Runtime claims local execution once before handing the prepared bytes to the adapter; uncertain claim/response/result is reconciled, never re-executed. This does not promise exactly-once external effects.

`AdmitRun` applies the analogous current checks to initial execution and pins manifest/route revisions. Its receipt cannot be reused by a replacement incarnation. For a protected effect, checks occur at the final trusted adapter immediately before use, with an online Core decision; no cached decision, stream lease or signed credential creates offline permission. If cancel/revoke serializes after bounded effect admission, that effect may already have happened; no rollback is promised. All subsequent effects are denied and the supervisor stops remaining work. Expiry uses trusted Core time; `now >= expiry` denies even if its notification has not arrived.

## 6. Initial context and routing seam

The trusted context adapter calls `ResolveInitialGoal` locally through locald with the admitted Run, immutable manifest identity/version, one exact trigger reference, owner generation/fence, Grant reference and initial-input identity. Locald independently verifies the host/OS peer (#349), current Human/Device and Agent delegation, current Channel/history interval, exact manifest/Grant binding and revocation through Core. No direct IM database access is exposed to agentd. Foreign, additional, expired or changed references fail; a caller cannot widen the manifest. Core/Gateway never resolve/decrypt or carry conversation/file plaintext.

Output is one opaque authored goal decoded only under #353's negotiated profile plus the one selected trigger message, with provenance references; combined UTF-8 output ≤65,536 bytes. Overflow fails without truncation or partial model input. C1's 16 later-input receipts remain separate. Admission does not preclude reauthorization denial at resolution/use. Resolve failure pauses local execution and reports through Core's business path for authoritative blocking/reconciliation, never a second initial turn or local state rewrite. Output lives only in protected memory and is cleared under C1; no history dump, path, token or durable plaintext spool. Missing/lost intake after possible worker use is uncertain, not an invitation to resolve/replay under a new key.

Route selection comes from discoverable trusted routing policy pinned by Run identity/version/data boundary and current expiry/budget. The demonstration may use a configured synthetic endpoint without credentials/customer content; renderer input cannot choose URL, model, tool or executable. Endpoint credentials are locally resolved through their separate trusted grant adapter and never enter control/audit records. No implicit fallback or mid-Run drift. Production Model Control/Connector admission remains separate.

## 7. Cleanup, reports and races

Admission records the original supervisor/worker incarnation and a separate Core-issued reconciliation assignment. Its authenticated credential is audience-restricted to `ReportCleanup` for that exact tenant/Run/Device/Runtime/owner generation/fence/worker and intent/barrier. It permits no work, Grant renewal, lease revival, general reads, context or completion-as-success. Execution Grant/lease expiry or revocation does not turn this narrowly scoped reporting assignment into execution authority. Credential compromise/revocation or expiry denies reporting; a trusted operator/Security reconciliation adapter must renew only that assignment after verifying original ownership. The record/barrier stays pending meanwhile; no automatic re-enrollment clears it.

The assignment is at most 30 minutes, checked at Core time, rotated without broadening audience; secrets never appear in durable control envelopes or telemetry. Even when Device/Runtime execution authority is revoked, Core may accept only this pre-bound, independently authenticated cleanup evidence. An obsolete owner cannot report a successor's cleanup, consume an action or append general Run events. Transfers are unsupported. Trusted deployment owns verifier keys/peer configuration; a `sealed=true` request field, ordinary Runtime bearer or reused PID is never a supervisor proof.

`ReportCleanup` binds stable report ID, original worker/owner, intent or safety-barrier ID, containment-profile/evidence version, immutable evidence identity, verdict and minimized reason. A trusted supervisor verifier validates evidence from the exclusively owned containment instance: admission irreversibly fenced, descendants unable to escape, no live members and OS wait/resource release complete (`SealedAndEmpty`). Unknown ownership, timeout or lost observation is unconfirmed. Dev fake-engine evidence proves only a no-worker synthetic lifecycle; it cannot certify Goose/native cleanup. #258/#328 already disprove ACP/root/process-group-only cleanup and grant no macOS containment admission.

| Core-serialized report or race | Result |
| --- | --- |
| Cancel/deny before approve/use/start | Intent blocks admission; old decision/dispatch cannot revive. Denial keeps Approval DENIED; verified stop yields Run CANCELLED, Task BLOCKED. |
| Approve before cancel/revoke | Invalidate issued Grant/consumption eligibility; late hint causes no new effect. Already admitted bounded effects require reconciliation. |
| Cancel before `CompleteRun(COMPLETED)` | Reject completion; require verified stop/failure. A local stop, ACP `cancelled` or activity EOF is never terminal evidence. |
| Verified successful completion before cancel | Return existing terminal fact; no new intent or rewrite. Success also requires verified cleanup and no uncertain action. |
| Verified stop, but Core unavailable | Supervisor stays stopped; retain bounded evidence identity and same report ID; UI remains unconfirmed until Core commit is acknowledged. |
| Verified failed cleanup | Core may commit FAILED/`cleanup_failed`; durable safety barrier prevents fresh/replacement Run admission. Failure report is not cancellation. |
| Later trusted reconciliation | Separate immutable resolution of that exact barrier/effect set, with verifier and audit provenance; terminal Run history is unchanged. Only then may an explicit fresh Run be considered. |
| Duplicate/conflicting report | Exact receipt creates no new event; changed state/usage/evidence/reason conflicts even under a new report ID. No unsafe retry. |

Never-admitted queued cancellation is the sole owner-report exception: Core must atomically prove no admission ever committed, invalidate all dispatch/claim authority and commit cancellation. Missing ACK, offline Runtime or the visible QUEUED enum is insufficient. Owner admission and this proof serialize on the same Run. An admitted or uncertain attempt cannot take this shortcut.

Producer submissions use `(tenant, Runtime generation, Run, owner generation, producer_id, producer_position)` with stable payload identity. Only RUN_STARTED is supported; Core allocates canonical RunEvent IDs/order. Duplicates return original mapping, changed payload conflicts, missing predecessor rejects with a gap. A restarted producer cannot replay or reset unknown work by choosing a new ID. Receipt/tombstone retention follows the Core profile; expired namespace returns RETENTION_EXPIRED and never recreates work.

## 8. Error and disclosure matrix

Use shared [ErrorDetail](error-model.md), never a new error envelope. Authentication precedes Run lookup; current disclosure checks also apply to duplicates and stream emissions. Unknown/foreign-tenant objects are indistinguishable NOT_FOUND without a foreign subject. All error metadata is bounded C2; omit content, arguments/commitments, credential bytes, paths and worker logs.

| Condition | Connect/gRPC status; existing detail | Safe recovery |
| --- | --- | --- |
| Invalid credential/peer/audience | UNAUTHENTICATED; no subject | Reauthenticate through the trusted host; no insecure fallback. |
| Current principal/permission revoked | PERMISSION_DENIED; PERMISSION_DENIED / DEVICE_REVOKED | Stop effects; only separately authenticated restricted cleanup may remain. |
| Envelope/request names another recipient | PERMISSION_DENIED; PERMISSION_DENIED, no foreign subject | Deny before Run disclosure; do not retarget. |
| Unknown profile/version/kind | UNIMPLEMENTED; ENVELOPE_VERSION_UNSUPPORTED when applicable | Explicit compatibility task; no legacy dispatch fallback. |
| Malformed/unknown field or nonpositive version | INVALID_ARGUMENT; no fabricated domain code | Correct input without partial writes. |
| Wrong Run owner/lease/fence | FAILED_PRECONDITION; NOT_EXECUTION_OWNER / LEASE_LOST / FENCING_TOKEN_STALE | Stop and reconcile; no retarget or old-owner writes. |
| Grant expired/revoked/out of scope | PERMISSION_DENIED; GRANT_EXPIRED / GRANT_REVOKED / GRANT_OUT_OF_SCOPE | No cached allow, renewed ONCE or action substitution. |
| Approval absent/denied or illegal state | FAILED_PRECONDITION; APPROVAL_REQUIRED / APPROVAL_DENIED / INVALID_STATE_TRANSITION | Read current state; no revival. |
| Changed operation/ACK/report/commitment | ALREADY_EXISTS; IDEMPOTENCY_CONFLICT | Reconcile original identity; never overwrite/re-execute. |
| Future/foreign cursor; required missing segment | FAILED_PRECONDITION; CURSOR_INVALID / SEQUENCE_GAP | Reconcile/repair, never skip or reset effects. |
| Uncertain local input/effect | FAILED_PRECONDITION; RUN_INPUT_OUTCOME_UNCERTAIN | Preserve safety barrier; trusted reconciliation only. |
| Current facts/route/Core unavailable | UNAVAILABLE; no invented allow/detail | Same operation identity; no new effects. |
| Unconfirmed durable commit | UNAVAILABLE; NOT_DURABLE | Same identity; commit may have succeeded. |
| Queue/buffer/payload/budget bound | RESOURCE_EXHAUSTED; RATE_LIMITED / PAYLOAD_TOO_LARGE when applicable | Backpressure or explicit denial; reserve stop capacity. |

## 9. Sequence traces and successors

1. Start: Human→Core StartRun commits QUEUED + control record; Runtime→Gateway→Core ReceiveControl; receive/ACK changes no Run state; ReadRunAuthority then AdmitRun commits once; locald resolves current bounded goal; trusted adapter starts one turn; SubmitRunEvents maps producer evidence to Core order.
2. Approval: owner freezes action→RequestApproval; Core commits WAITING_APPROVAL; Human decides through Core ApprovalService; approval_changed wakes current read; AdmitAction consumes exact ONCE action; trusted adapter claims it once. UI/local activity never delivers a decision authority.
3. Cancel/reconnect: Core cancel intent + reserved control record commits; disconnect loses no intent; supervisor may stop locally at once; replayed cancel leads to verified cleanup report; only acknowledged Core commit confirms CANCELLED. Failed/uncertain cleanup blocks a new attempt, including after restart.

All missing executable surfaces below require independently claimed tasks under #350. #355 owns common operation/action/owner/report wire additions; #356 owns audit/control descriptors; #354 owns current authorization; #353 owns authored encoding; #349 owns local peer/window identity. None is implemented by this profile.

| Gap / owner | Bounded successor |
| --- | --- |
| D1 — Contracts/Integration, #360/#361 | Separate control stream/ACK wire (#360), then current-authority/admission/reconciliation wire (#361) and cross-language golden vectors; reuse #355's business types and generate SDKs with provenance. Shared runtime_service.proto edits are serialized by native dependencies. |
| D2 — Core/Security, #362 | Runtime dev credential verification/enrollment binding and restricted reconciliation identity, without product process wiring or production Device proof. |
| D3 — Integration migrations, #363 | Reserved control sequence/checkpoint/receipt/reservation and owner/barrier/evidence storage; tenant isolation/constraints/rollback, no handler implementation. |
| D4 — Core, #364 | Durable control replay/ACK port and repository adapter; no Gateway database access. |
| D5 — Core, #365 | Current Run authority plus serialized Run/action admission/consumption; exact action/decision, online fact checks and receipt behavior. |
| D6 — Core, #366 | Cleanup report verification, terminal reconciliation and immutable safety-barrier clearance; no resumed execution authority. |
| D7 — Runtime, #367 | Authenticated outbound Gateway relay with bounded streaming/backpressure; Core interface only, no business cache/store. |
| D8 — Runtime, #368 | agentd control replay/admission/authority adapter with uncertain-restart behavior; preserve the three C1 local RPCs. |
| D9 — Runtime/Security, #369 | Trusted supervisor evidence/reconciliation adapter; synthetic evidence kept distinct from native containment; macOS protected execution remains gated. |
| D10 — Client-core, #370 | Authorized bounded locald initial-goal resolver; references/current authority only, no raw DB/file access from agentd. |

Core Task/Run/Approval storage/commands and process wiring remain separate packages in #350; each dependency must be concretely split before implementation claim. New schemas/verifier registration await #335. No migration ID is reserved here. #259 stays blocked until real authenticated Core/Gateway/agentd/context/IPC implementations and business-chain acceptance pass.

## 10. Reference evidence

[scenarios.json](../../test/fixtures/proto/runtime-control-delivery/scenarios.json) contains synthetic reference expectations only. Each `given` overlays declared trusted defaults; these are not wire booleans or a policy evaluator. Ordered traces use each operation's proper principal. `newEffects` counts executable dispatches, never hints/ACKs; `unchanged` means no requested durable mutation. No parsing/existing suite success proves this new behavior or coverage.

Run `node scripts/proto.mjs check`, parse the fixture with `python3 -m json.tool`, and run `git diff --check`. Manually review identity, replay/receipt, exact-action, race, expiry and cleanup matrices against both positive and denial rows. Record existing C1 suite results separately. Executable conformance and real transport tests belong to the named successors after #335; this task modifies neither the legacy aggregate verifier nor Proto/SDKs, manifests, lockfiles or product code.

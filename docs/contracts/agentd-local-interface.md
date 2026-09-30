# Desktop to agentd local contract

Status: **Draft contract** for [ADR-0007](../adr/0007-agentd-task-interface.md) and [issue #216](https://github.com/monkeylabx/threadline/issues/216). The Proto and synthetic fixtures define an interface; no Tauri bridge, Rust agentd, trusted bootstrap, worker containment or production authorization is implemented here.

## One small interface

The typed Tauri host calls `threadline.local_agent.v1.LocalAgentService` over the planned local gRPC/Protobuf IPC (UDS on macOS/Linux, Named Pipe on Windows). Only agentd connects to Goose through ACP stdio. The service has three methods: `SubmitRunInput`, `WatchRunActivity`, and `RequestRunStop`. None exposes a Goose Session, arbitrary engine method, executable, file path, MCP configuration, model endpoint or credential. No TCP listener is introduced.

| UI need | Existing owner | New local behavior |
| --- | --- | --- |
| Create/start Task or Run | Core `TaskService` / `RunService` | None. The initial Task goal enters through authorized dispatch; `SubmitRunInput` is only for a later turn in the same live Run. |
| Submit a later turn | No existing device-local content method | `SubmitRunInput` admits one bounded text input for an authorized live Run. |
| Watch text/tool activity | Core `StreamRunEvents` only carries C2 metadata | `WatchRunActivity` streams filtered C3 device-local views. Core's `RunEvent` stays metadata-only. |
| Cancel | Core `RunService.CancelRun` records the durable intent | `RequestRunStop` is an immediate local interrupt request. Its receipt is never a durable CANCELLED result. |
| Decide/revoke Approval | Core `ApprovalService` | No local decision RPC. The owner may act only after receiving a trusted Core decision and rechecking the exact action. |

The module's responsibility is local Run interaction; authorization, durable state, Connector access and engine orchestration remain behind the agentd interface. Deleting agentd would distribute those responsibilities across the Tauri host and Goose. The interface is independently reversible because it only adds a new package and one additive shared error code; no existing wire field changes meaning.

## Trusted binding

The Tauri Rust host must bind each window to a trusted Tenant/Actor/Device/Session principal and authenticate its OS IPC peer. Agentd checks that binding, the requested Run, current observer/controller role, execution owner, lease/fencing, grants and routing before an effect. A request's `run_id`, `input_id`, stream ID, process path or possession of the socket cannot assert identity or authority. A different window cannot inherit a prior window's stream. The Proto deliberately has no principal, window, token or server-credential field.

The synthetic fixture supplies trusted binding facts to exercise the decision table. Its `authorizedActor`/`authorizedDevice` describe a checked role for this caller, not the Execution Owner; an authorized observer can watch without owning execution. It is not a bootstrap mechanism. Device proof in [#200](https://github.com/monkeylabx/threadline/issues/200) and exact IPC peer ACL, window binding and revocation propagation need Contracts/Security review before any protected production operation. Missing, expired, unavailable or mismatched evidence fails closed. Agentd independently stops new effects when its owner lease or grant is lost; cancelling its own contained worker is always permitted to the supervisor.

| Fact needed by agentd | Planned trusted source | Missing/stale result |
| --- | --- | --- |
| Tenant, Actor, Device, Session and window | Authenticated Tauri host bootstrap plus OS IPC peer binding; exact mechanism remains with Client/Security | Deny local call before Run lookup. |
| Run state, observer/controller role and Execution Owner | Core Run/authorization services, delivered over authenticated business paths | Deny input/watch/control as appropriate; never infer ownership from Run ID. |
| Workspace lease and fencing token | RuntimeGateway renewal and current Core Run authority | Deny new effects; stop an old worker. |
| Grant, budget and model route | Core Capability Grant and approved routing policy | Deny context/model/tool effects. |
| Workspace file consent | User-authorized `connectord` capability | Deny file/tool effect; no path from local RPC can grant access. |

Production blockers: Client/Security own peer ACL, window binding and revocation design; Contracts/Core/Runtime own trusted Approval decision delivery; Runtime/Security own protected intake lifetime and OS worker containment. These are separate admission decisions. The C1 fixtures substitute synthetic facts and do not resolve them.

## Input and activity semantics

- `SubmitRunInput` accepts UTF-8 text of 1–65,536 bytes and an opaque 1–128-byte printable non-space ASCII `input_id` unique within one Run. Additional context is selected through the existing Task/Context Manifest and authorized local connectors, not arbitrary request paths. A live Run allows one active turn at a time; terminal Runs never reopen.
- Success means the input was validated and recorded in protected local intake before Goose dispatch. It does **not** mean a model turn completed, a Core RunEvent committed or an Artifact was published. The local intake's encryption, lifetime and crash recovery require Runtime/Security admission before implementation. The same `(run_id, input_id, exact UTF-8 bytes)` returns `DUPLICATE` without another turn; altered bytes return `ERROR_CODE_IDEMPOTENCY_CONFLICT`.
- If an earlier input may already have reached Goose or a tool but the local outcome cannot be reconciled, return `ERROR_CODE_RUN_INPUT_OUTCOME_UNCERTAIN`. Neither a new `input_id` nor an automatic replay is safe until the owner reconciles effects. Parallel new turns are refused; a duplicate of a known accepted turn may return its prior receipt.
- `WatchRunActivity` sends a ready frame first (`sequence=0`) with an agentd-incarnation stream ID, earliest retained sequence and current head. Updates have positive per-Run sequences, distinct from Core `RunEvent.sequence`. An empty stream ID and cursor 0 start a first watch; a reconnect supplies the issued stream ID and last applied sequence. A stream ID from another incarnation/window is invalid. If the next needed sequence precedes the bounded buffer's floor, report `ERROR_CODE_SEQUENCE_GAP`; the UI marks missing output and does not reconstruct plaintext from Core events.
- Agent text is capped at 8 KiB UTF-8 per frame; tool display labels at 128 bytes. Agentd maps/filter engine updates before emission. Raw tool arguments/results, internal reasoning, diagnostic logs, paths and unapproved source content never appear in the local stream. Authorization is rechecked during a long stream; loss of authority ends it. Backpressure may discard old buffered frames only with an explicit subsequent gap; the buffer and retained content are bounded and cleared by reviewed Run data-lifecycle policy.

## Stop and Approval convergence

`RequestRunStop` requires current control authority and immediately asks agentd to interrupt the local worker. The UI also uses Core `CancelRun` for the durable cancellation intent; the two paths converge on that Run, not on a local copy of its state machine. A local response acknowledges a stop request, not descendant cleanup or Core cancellation. If Core is temporarily unavailable, an authorized local stop can still prevent further workstation effects, while the Run stays unconfirmed until owner reconciliation. A late Approval cannot revive the action. Once containment cleanup is verified, the Execution Owner acknowledges terminal state through RuntimeGateway/Core. [ADR-0006](../adr/0006-rust-agentd-goose-runtime.md) still requires descendant cleanup evidence; ACP `cancelled` is insufficient.

The UI reads/decides Approvals with Core's existing service. RuntimeGateway currently has `RequestApproval` but no reviewed delivery of the subsequent decision or cancellation to agentd. Contracts/Runtime must specify that trusted delivery and exact action/expiry/revocation recheck before R1 can execute a protected effect. A client-supplied decision, cached allow or local stream update is never a Capability Grant. With delivery unavailable, protected tools fail closed; ordinary IM remains available.

## Error and evidence table

Use the shared `threadline.type.v1.ErrorDetail` in gRPC details. An unauthenticated peer receives gRPC `UNAUTHENTICATED` without disclosing a Run identifier. A denied observer/controller receives `PERMISSION_DENIED`; wrong-tenant requests receive `TENANT_MISMATCH` only after a trusted principal is established. Do not include input, response, credential, paths or raw worker output in error details or logs.

| Condition | Stable error or response | Retry rule |
| --- | --- | --- |
| Terminal Run / parallel new turn | `INVALID_STATE_TRANSITION` | No replay of the same turn without checking current Run state. |
| Old owner or lost lease | `NOT_EXECUTION_OWNER` / `LEASE_LOST` | No effect until current owner/lease is established. |
| Stale fencing token | `FENCING_TOKEN_STALE` | No effect until current fencing is established. |
| Revoked/expired grant | `GRANT_REVOKED` / `GRANT_EXPIRED` | No protected effect; get new authority through business path. |
| Changed text for a known input ID | `IDEMPOTENCY_CONFLICT` | Never treat as the original input. |
| Uncertain prior input effect | `RUN_INPUT_OUTCOME_UNCERTAIN` | Reconcile; do not retry or mint a fresh ID blindly. |
| Oversized input or activity | `PAYLOAD_TOO_LARGE` or bounded stream termination | Reduce or reject; no partial protected effect. |
| Old stream incarnation / future cursor | `CURSOR_INVALID` | Start a new watch and show that past plaintext may be lost. |
| Retained-stream gap | `SEQUENCE_GAP` | Show an explicit gap; never claim a complete transcript. |
| Local stop receipt | `RequestRunStopResponse` | Wait for verified cleanup and durable owner acknowledgement. |

The independent [synthetic scenarios](../../test/fixtures/proto/agentd-local/scenarios.json) and [wire frames](../../test/fixtures/proto/agentd-local/wire.json) cover the decision matrix and canonical binary/JSON encoding. Run `node proto/tools/verify-agentd-local-contracts.mjs` to check outcomes, fixture digests, encoding, method/field surface and error-code additions; the repository's contract check also invokes it. Buf build/lint/breaking check the schema itself. Fixture success proves the stated contract only; auth bootstrap, OS IPC, model execution, cleanup and five-platform generated SDK compatibility remain **NOT RUN**.

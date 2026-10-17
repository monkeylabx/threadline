# Threadline agentd task interface

Status: **Draft implementation profile** for accepted [ADR-0007](../adr/0007-agentd-task-interface.md). No local Agent RPC, Rust supervisor or worker lifecycle is implemented or admitted by this document.

## One engine boundary

React uses a typed, window-scoped Tauri bridge. The Rust host calls `agentd` through the planned local business gRPC/Protobuf IPC over UDS/Named Pipe. `agentd` is the sole ACP Client for a supervised, pinned `goose acp` subprocess over stdio. The UI receives Threadline Run references and activity views, not Goose Session IDs, a raw ACP transport or arbitrary engine commands. There is no client-facing ACP server, relay, new workstation TCP listener or third-party ACP compatibility claim.

Goose owns model/tool turns and context compaction. `agentd` owns local execution supervision and translates engine results to Threadline interaction and Run events; it does not implement a second Agent loop. IM/locald commands, Connector authorization, Mobile FFI and server business APIs retain their roles. Web/Mobile do not execute a local worker.

## Small client surface

These are responsibilities, not new RPC names or wire schemas. Reuse Threadline domain types and the existing business contracts; the contract task must define only the missing local operations.

| Interaction | Required behavior |
| --- | --- |
| Submit Run input | Accept only an existing, live, authorized Run. Check principal, current owner/lease, route, grants, budget and admitted input/context references before invoking Goose. Do not create authority or silently revive a terminal Run. |
| Subscribe to activity | Verify observer and window access. Return bounded, scoped text and tool activity for that Run; distinguish transient progress from confirmed durable state. No raw engine logs, internal reasoning or unauthorized tool content. |
| Request cancellation | Use the existing Run cancellation business path. The owner revokes queued effects/tool authority, interrupts Goose, stops contained execution and verifies descendants; confirmed cancellation requires durable owner acknowledgement. |
| Approval interaction | Show the bounded pending action and submit the user's decision through the existing Approval contract. Recheck exact arguments, principal, scope, expiry and revocation before releasing execution. A local UI answer alone is not a Grant or Approval. |

The existing [Task/Run/Approval services](../../proto/threadline/task/v1/task_service.proto) in Core own creation and durable transitions. [RuntimeGatewayService](../../proto/threadline/runtime/v1/runtime_service.proto) supplies dispatch, grants, lease/fencing and event submission. The local interface consumes those facts. Observing or requesting control of a Run does not confer execution ownership. Local and server cancellation requests must converge on the same durable cancellation intent; disconnected handling requires the contract review rather than a second local state machine.

## Authorization and resource boundary

Trusted authentication supplies the Tenant/Actor/Device/Session principal and window binding. Local peer access, a Run ID, a process path or UI fields cannot construct that identity. Bootstrap, OS IPC ACLs and current control/observer binding must be reviewed before enabling the interface; absent, expired, revoked or unavailable authority denies new effects. Cross-window Run access is checked independently.

`agentd` maps the Run to an internal Goose Session; neither is a Channel or shared Task Thread. Follow-up input uses the live Run's current grants/context. A retry uses a fresh Run and internal Session; optional engine Session loading stays disabled until same-principal recovery and retained-context access are proven.

Context reads use scoped `locald` Context API access. Workspace effects use `connectord` and current user-authorized grants. Worker configuration, workspace roots and MCP descriptors come from trusted Run policy, not UI-provided executables, environment, network destinations or engine extensions. Do not bridge Goose filesystem/terminal requests into unrestricted UI capabilities. Built-in host-capable tools, dynamic installation, cross-session recall and global Memory remain disabled pending ADR-0006 admission.

Model endpoints and short-lived credentials come from approved routing. The worker has no direct IM database/key access or general host filesystem authority; OS sandbox and egress enforcement remain required. Permission modes and prompt instructions do not supply containment. Protocol output is separate from redacted diagnostics; prompts, message/file/tool content, credentials and keys are not logged.

Only admitted activity reaches its authorized observer. Shared Artifact access follows existing Task/Run/Artifact contracts. Private Work publication needs the separately reviewed contract and scope decision in proposed [ADR-0005](../adr/0005-private-work-publication-boundary.md); streaming a result never publishes it to a Channel or Task Thread.

## Failure and cancellation

Core remains the durable shared Run authority. ACP notifications and `end_turn` are engine interaction results, not Run completion, Approval or publication records. UI reconnect reads committed Run events and reconciles uncertain effects; it must not blindly repeat input or a tool effect. The contract task defines input idempotency, sequence/cursor bounds, parallel-input handling and restart behavior.

A Goose `cancelled` result alone does not prove cleanup: [PR #211](https://github.com/monkeylabx/threadline/pull/211) observed a surviving background child. Stop revokes tool authority, cancels pending approvals, interrupts the engine and terminates/verifies contained descendants. A late approval cannot revive cancelled work. Cleanup failure is reported through the Run failure contract and never presented as successful cancellation.

UI disconnection cancels pending UI approval interactions but does not imply worker termination or permission to continue forever. A reviewed Run lifecycle determines whether authorized background work can continue. If identity, lease, recovery or containment evidence is unavailable, new effects stop. Agent/bridge/worker failures never block IM, Outbox or Sync.

## Bounded successors and evidence

Each row is a separate task of at most two agent days. OS-specific production containment/signing follow separately; root manifests/lockfiles remain Integration-owned. No implementation scenario below has run.

| Task | Owner and output | Acceptance |
| --- | --- | --- |
| C1 Local Run interface and fixtures | Contracts: reuse Task/Run/Approval types; specify trusted bootstrap/window binding, minimal local commands/activity, errors, bounds, idempotency, sequencing and cancellation convergence. | Reviewed contract and independent fixtures; no new ACP server or parallel business state machine. **NOT RUN** |
| R1 Rust supervisor with fake worker | Runtime: implement the admitted local interface and one internal ACP Client in `crates/agentd/`; use trusted binding fixtures and a fake engine, without credentials or user files. | Planned `cargo test -p threadline-agentd --locked` plus independent local-interface/stdio scenarios created with the task. Requires C1 and separate workspace integration. **NOT RUN** |
| R2 Pinned Goose probe | Runtime: substitute the pinned Goose process with a synthetic model endpoint on one specified Desktop OS; enforce restrictive tools and verify descendant cleanup. | Binary hash, launch/sandbox policy, scoped storage and reproducible probe; remaining OS evidence follows separately. Requires R1 and reviewed containment inputs. **NOT RUN** |
| D1 Desktop task bridge | Desktop: render the same Run input/activity/approval/cancellation fixtures through typed, window-scoped commands. No ACP parser or engine API in the UI. | Exact bridge test command defined before claim; multi-window, denial, reconnect and IM isolation cases. Requires C1/R1. **NOT RUN** |

| Scenario | Required result | Evidence |
| --- | --- | --- |
| Authorized live Run and compatible pinned worker | Local input, scoped streaming and business Run reconciliation work through one ACP connection. | NOT RUN |
| Missing/revoked principal, wrong Tenant/Device/Run/window or stale owner/lease | Denial before context/model/tool access; no identity or ownership inferred from request fields. | NOT RUN |
| UI attempts engine commands, hostile MCP configuration, ungranted files or unrestricted terminal callbacks | No raw engine entry, arbitrary process, unapproved file access or egress. | NOT RUN |
| Changed/expired/revoked approval, wrong principal or late response | Protected effect is denied and cancelled work stays stopped. | NOT RUN |
| Unauthorized worker output or Artifact publication attempt | Only scoped activity; no implicit Channel/Task publication. | NOT RUN |
| Duplicate/parallel input, reconnect after uncertain effect or worker restart | Idempotency/sequencing and reconciliation prevent blind replay; terminal Run stays terminal. | NOT RUN |
| Cancel during inference, approval, foreground tool or detached child | Pending approvals cancelled, grants/effects stopped and descendants independently verified before confirmed cancellation. | NOT RUN |
| Unsupported/malformed/oversized engine stream or bridge/worker crash | Bounded failure, redacted diagnostics, no unsafe fallback and IM stays usable. | NOT RUN |

Official sources for the internal engine boundary: [Goose ACP subprocess](https://goose-docs.ai/docs/gdk/acp/), [ACP initialization](https://agentclientprotocol.com/protocol/v1/initialization), and [prompt/permission/cancellation](https://agentclientprotocol.com/protocol/v1/prompt-turn). Threadline's local business interface and controls above are project design requirements, not capabilities provided by ACP.

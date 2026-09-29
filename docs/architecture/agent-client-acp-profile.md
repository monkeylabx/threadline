# Threadline Agent client ACP profile

Status: **Draft implementation profile** for accepted [ADR-0007](../adr/0007-acp-agent-client-interface.md), reviewed against primary documentation on 2026-09-29. No facade, authentication bootstrap, or two-connection runtime has been implemented or admitted by this document.

## Roles and transport

The Tauri Rust host is the ACP Client toward the signed Rust `agentd` sidecar. `agentd` is an ACP Agent toward that client and an ACP Client toward the supervised Goose process from [ADR-0006](../adr/0006-rust-agentd-goose-runtime.md). Each leg has its own initialization, capabilities, request IDs, Session IDs, timeouts, and failure handling. Request IDs are correlated internally; clients cannot address the Goose connection or invoke its private methods directly.

The first Desktop transport uses child stdio with the ACP UTF-8 JSON-RPC framing rules. Protocol output is isolated from redacted diagnostics; no prompt, message body, credential, key, or tool content is logged. A typed Tauri bridge restricts windows to the admitted ACP interaction subset and binds each window to its trusted principal. Creating a new window does not confer another window's Session access. Existing locald/Connector RPC and server business contracts remain separate. No remote listener is introduced.

## Authority and identity

Initialization negotiates protocol features, not authority. Before creating/loading a Session, sending a prompt, requesting cancellation, or answering an approval, `agentd` requires a trusted Authenticated Principal and a current Run binding. Trusted authentication supplies Tenant/Actor/Device/Session identity; it is never constructed from `_meta`, a process path, `cwd`, a Session ID, or the fact that the peer is on the same computer. A protected host bootstrap or an advertised ACP authentication flow must be reviewed before enabling this path. Missing, expired, revoked, or unavailable evidence fails closed.

The Desktop business path creates/starts the Task/Run through the existing [RunService](../../proto/threadline/task/v1/task_service.proto); Runtime dispatch, grants, lease/fencing, and event submission use [RuntimeGatewayService](../../proto/threadline/runtime/v1/runtime_service.proto). The facade consumes that authority rather than minting it. Ownership to execute and permission to observe/control are checked separately. A valid Run reference does not authorize every observer or permit a new execution owner.

The mapping is client Session → authorized Run → internal Goose Session. A Channel is never any of these Sessions. Within a live Run, admitted follow-up turns use its current context and grants. A terminal Run cannot be revived with another prompt; retry creates a fresh Run and internal worker Session. Session loading remains unadvertised until same-principal access, retained context, and recovery are proven. Unsupported features are not advertised on either leg.

## Standard interaction and Threadline mediation

| ACP interaction | Required Threadline behavior |
| --- | --- |
| `initialize` / authentication | Pin a reviewed protocol schema and negotiate each leg independently. Expose only tested capabilities; authenticate before protected operations. |
| `session/new` | Resolve an authorized Run binding and validate all supplied workspace/MCP inputs before creating a Session. No implicit shared Task, Channel, or grant creation. |
| `session/prompt` | Recheck current principal, Run/owner, lease, grants, route, and budget. Combine admitted user input with bounded context from locald; reject unauthorized source references and unsupported content. |
| `session/update` | Emit authorized text/activity views, mapped to the client's Session. Reconcile durable Run facts separately; do not relay raw logs, internal reasoning, or arbitrary tool results. |
| `session/request_permission` | Record the exact protected action through Threadline's Approval contract, show the authorized principal a bounded request, and validate the resulting decision before allowing execution. |
| `session/cancel` | Check control authority, revoke queued effects/tool authority, interrupt Goose, stop contained execution, and verify descendants are gone. Resolve the prompt as cancelled only after successful abort; durable Run cancellation also requires the owner acknowledgement. |
| Optional load/configuration/extension methods | Enable only after a reviewed contract and tests. A mode change, model selection, or extension request cannot widen authority or bypass approved routing. |

An ACP permission choice expresses user intent; it is not an enterprise Approval or Capability Grant. Tool arguments, Actor/Device, scope, expiry, revocation, and the exact pending action are checked again before execution. Persistent ACP approval choices cannot silently become standing permission. Cancellation resolves pending permission requests as cancelled and prevents a late allow response from reviving the action.

## Resource and output policy

- `cwd`, additional directories, embedded resources, and `mcpServers` are untrusted inputs. Only granted workspace roots and Threadline-approved tool descriptors are admitted. Client-supplied MCP commands, arguments, environment, or network destinations cannot cause arbitrary process launch or credential access.
- The first facade does not expose general client filesystem/terminal callbacks. `agentd` does not forward worker requests to an unrestricted UI/editor filesystem or terminal, even if a client advertises them. Goose receives only allowlisted Threadline tools backed by current locald/Connector grants; direct host-capable tools and dynamic installation stay disabled pending ADR-0006 admission.
- Every context read and tool operation rechecks current authorization. Model endpoints and credentials come from approved routing, not from a client extension. No IM database or key is exposed to the client or worker through ACP.
- Output is scoped to its current observer and authorized sources. Private execution material is never broadcast to a Channel or Task Thread merely because ACP produced it. Shared Artifact visibility follows the existing Task/Run/Artifact contracts and their authorized audience. Publishing Private Work requires the separately reviewed publication contract and scope decision in proposed [ADR-0005](../adr/0005-private-work-publication-boundary.md); this interface decision does not approve that scope.

## Disconnect, retry, and durable state

ACP notifications and a prompt stop reason are transient interaction results. Core remains the durable shared Task/Run authority; the facade reconciles against Threadline Run events and sequences. An `end_turn` result does not itself commit Run completion or Artifact publication. The UI distinguishes streamed activity from confirmed business state.

JSON-RPC request IDs are not durable idempotency keys. After a disconnect or crash, clients do not blindly repeat a prompt or tool effect. The supervisor reconciles committed events and uncertain external effects before accepting a retry. Duplicate/parallel prompts, Session loading, and recovered approvals need explicit sequencing rules in the reviewed binding contract. If identity, lease, or reconciliation is unavailable, new effects stop; ordinary IM, Outbox, and Sync continue.

A Goose `cancelled` response is insufficient to prove process cleanup: [PR #211](https://github.com/monkeylabx/threadline/pull/211) observed a background child surviving worker cancellation. Cleanup failure is recorded through the Run/failure contract and is not presented as confirmed cancellation. Lost UI transport cancels any outstanding UI permission requests; continued background execution requires a reviewed Run lifecycle policy, never an assumption from the ACP connection disappearing.

## Extension and interoperability limits

Standard [ACP interaction](https://agentclientprotocol.com/protocol/v1/overview) is preferred. Threadline-only correlation or binding data may use supported `_meta` locations and negotiated underscore-prefixed methods as defined by [ACP extensibility](https://agentclientprotocol.com/protocol/v1/extensibility). This profile defines no new private method or root-level schema field. Client metadata can carry references, never trusted identity or approval evidence.

A compatible third-party client still needs an admitted authentication flow and a Run obtained through the Threadline business path. A preauthorized test Run can exercise the basic ACP subset without product-specific extensions; that does not prove general Task selection, enterprise approvals, or the full IM interface works in that client. The prototype must reveal the minimum required extensions and unsupported cases before claiming interoperability.

## Successors and acceptance evidence

Each item is a separate task of at most two agent days; production OS-specific containment and signing are subsequent platform tasks. Root workspace/lockfile changes remain Integration-owned.

| Task | Owner and output | Gate |
| --- | --- | --- |
| C1 ACP binding interface and fixtures | Contracts: review trusted bootstrap evidence, Run selection, observer/control identity, action correlation, reconnect/idempotency, bounds and errors; pin the ACP schema and define minimal extensions only where fixtures require them. | Reviewed documented interface plus independent JSON fixtures; no invented production authentication or wire-field changes. **NOT RUN** |
| R1 Rust facade lifecycle slice | Runtime: implement the admitted interface in `crates/agentd/` against a fake worker and trusted binding fixture. Exercise both ACP roles with an independent client, streaming, approval denial, disconnect and cancellation; use no real credentials or user files. | Planned `cargo test -p threadline-agentd --locked` and an independent stdio scenario runner, created with the task. Requires C1 and a separate workspace integration task. **NOT RUN** |
| R2 Pinned Goose worker probe | Runtime: replace the fake with the pinned Goose process and a synthetic model endpoint; test restrictive tools and cancellation/descendant containment on one specified Desktop OS. | Reproducible binary hash, launch policy, scoped storage and probe evidence; remaining OS matrices tracked separately. Requires R1 and reviewed containment inputs. **NOT RUN** |
| D1 Desktop bridge slice | Desktop: implement typed window-scoped ACP transport and interaction rendering against the same R1 fixtures. Preserve IM when Agent transport fails. | Exact bridge test command defined before claiming; multi-window, denied operations, redaction and reconnect cases. Requires C1/R1. **NOT RUN** |

The independently executable fixture/scenario suite must cover:

| Scenario | Required result | Current evidence |
| --- | --- | --- |
| Compatible protocol, authenticated principal, preauthorized Run | Session and streamed interaction work on both legs without sharing their IDs. | NOT RUN |
| Unsupported version or required capability | Explicit incompatibility; no execution or automatic wider fallback. | NOT RUN |
| Missing/revoked principal, wrong Tenant/Device/Run or another window's Session | Denial before context/model/tool access; no identity taken from request metadata. | NOT RUN |
| Forged binding metadata, out-of-scope cwd or hostile MCP descriptor | No grant creation, process launch, host file read, or unapproved egress. | NOT RUN |
| A client advertises unrestricted filesystem/terminal capabilities | No worker-to-client bypass of Connector checks. | NOT RUN |
| Approval allow with altered arguments, wrong principal, expired/revoked grant or a late response | No protected action executes; authoritative Approval remains bound to the exact action. | NOT RUN |
| Worker sends raw logs, internal reasoning or unauthorized tool content | Content is withheld; only admitted activity/output reaches the observer. | NOT RUN |
| Duplicate/parallel prompt or disconnect after an uncertain effect | No blind replay; sequence/side-effect reconciliation is explicit. | NOT RUN |
| Cancel during inference, approval, foreground tool or detached child | Pending approvals are cancelled; grants/effects stop and cleanup is independently verified before reporting success. | NOT RUN |
| Worker/facade/bridge crash or malformed/oversized stream | Bounded failure and reconciliation; IM continues; no secret-bearing diagnostics. | NOT RUN |
| Independent ACP client with no Threadline extensions | Only the admitted prebound subset is claimed; missing product capabilities are explicit. | NOT RUN |

Official references: [ACP transport](https://agentclientprotocol.com/protocol/v1/transports), [session setup](https://agentclientprotocol.com/protocol/v1/session-setup), [prompt/permission/cancellation flow](https://agentclientprotocol.com/protocol/v1/prompt-turn), and [Goose ACP subprocess](https://goose-docs.ai/docs/gdk/acp/). These support protocol facts; the Threadline controls and gates above are this project's design requirements.

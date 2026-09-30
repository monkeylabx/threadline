---
status: accepted
date: 2026-09-29
---

# ADR-0007: Task-oriented client interface with internal ACP

Threadline Desktop will use its planned local business IPC for the Agent UI to call Rust `agentd`. Only `agentd` speaks ACP to the pinned, headless Goose worker selected in [ADR-0006](./0006-rust-agentd-goose-runtime.md). This keeps one engine connection and a small Run-oriented client interface, avoiding a second ACP lifecycle and a partial protocol relay.

```text
Threadline Agent UI
       | typed, window-scoped Tauri bridge
       v
Tauri Rust host
       | versioned local business IPC
       v
Rust agentd: Run supervisor and ACP Client
       | supervised ACP / JSON-RPC over child stdio
       v
Pinned Goose worker: ACP Agent
```

## Interface and ownership

The client surface covers authorized Run input, activity subscription, cancellation and approval interaction using Threadline domain types. Task/Run creation, durable transitions, Capability Grants, exact-action Approval, Audit, Artifact provenance and Publication retain their existing business owners. Goose Session IDs, ACP capabilities and engine-specific methods stay inside `agentd`; the UI has no raw ACP endpoint or direct Goose connection.

Use the planned local gRPC/Protobuf IPC over UDS on macOS/Linux and Named Pipe on Windows from the [system architecture](../architecture/system-architecture.md). React calls a typed Tauri bridge, and the Rust host authenticates local IPC and binds each window to its trusted principal. The [Draft interface profile](../architecture/agentd-task-interface.md) defines behavior for a contract task; concrete bootstrap, local RPC schemas and recovery fixtures still require review before implementation. This ADR adds no new listener or parallel Task/Run/Approval model.

`agentd` checks current principal, Run/control authority, execution lease, grants, route and budgets; maps one authorized Run to its internal Goose Session; and exposes only scoped activity. It obtains bounded context through `locald` and approved workspace tools through `connectord`. High-impact effects require the existing auditable Approval contract. Client input, ACP permission choices, paths and MCP configuration never create authority. Cancellation includes worker/descendant cleanup and durable reconciliation, not only sending ACP cancellation.

## Trade-off and consequences

This revises the client-facing ACP proposal in the still-unmerged interface task. A two-role ACP facade would require independent negotiation, Session mapping, approval translation and recovery on both connections, while Threadline-specific authorization would still prevent full transparency. A raw relay or direct UI-to-Goose connection would distribute engine and permission handling into clients. The selected business interface keeps those responsibilities in the Run supervisor. It does not build another model/tool loop; Goose continues to own that loop and its working-context compaction.

Third-party ACP clients are not a Threadline compatibility target in this decision. Mobile/Web continue to control and observe Tasks through existing business contracts without a local worker. [ADR-0001](./0001-client-platform.md), Rust/Goose selection, process isolation and every ADR-0006 admission gate remain in force. Ordinary IM, Outbox and Sync remain usable when `agentd` or Goose fails.

The decision is accepted, but the local interface, worker lifecycle and OS security evidence are **NOT RUN**. This documentation changes no executable, Proto schema, dependency, estimate, scope or Gate status. Goose exposes the chosen internal [ACP subprocess interface](https://goose-docs.ai/docs/gdk/acp/); its protocol support does not establish Threadline authority or production admission.

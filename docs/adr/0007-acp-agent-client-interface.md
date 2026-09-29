---
status: accepted
date: 2026-09-29
---

# ADR-0007: ACP between Agent clients and Rust agentd

Threadline will expose an ACP Agent interface from Rust `agentd` for its Desktop Agent interaction surface. `agentd` remains an ACP Client toward the pinned, headless Goose worker selected in [ADR-0006](./0006-rust-agentd-goose-runtime.md). This reuses standard Agent interaction while preserving Threadline's business and authorization contracts. The decision is accepted; the [implementation profile](../architecture/agent-client-acp-profile.md) and runtime admission evidence are still pending.

```text
Threadline Agent UI
       | Tauri bridge (window identity and permission checks)
       v
Tauri Rust host: ACP Client
       | ACP / JSON-RPC over child stdio
       v
Rust agentd: ACP Agent toward the UI; ACP Client toward Goose
       | separate supervised ACP / stdio connection
       v
Pinned Goose worker
```

## Interface scope

This decision replaces the generic versioned local RPC choice for the **Desktop Agent interaction path** shown in ADR-0006. It does not replace Desktop IM commands/queries to `locald`, `locald` Context access, Connector authorization, Mobile FFI, server Connect/Protobuf, or Runtime Gateway mTLS/Protobuf. [ADR-0001](./0001-client-platform.md) continues to govern process, storage, signing, and failure isolation.

ACP carries session interaction, prompts, filtered streaming updates, permission requests, and cancellation. Its [standard methods](https://agentclientprotocol.com/protocol/v1/overview) and [extension mechanism](https://agentclientprotocol.com/protocol/v1/extensibility) are reused before adding private methods. Task creation, durable Run transitions, Capability Grant issuance, exact-action Approval, Audit, Artifact provenance, and Publication remain Threadline-owned business contracts. A protocol Session is not a Channel, shared Task Thread, or new source of durable facts.

The first transport is local: the Tauri Rust host manages the signed `agentd` sidecar and its [ACP stdio connection](https://agentclientprotocol.com/protocol/v1/transports). React uses a typed, window-scoped Tauri bridge; it receives no arbitrary executable, transport, filesystem, or terminal access. The `agentd`–Goose connection is separate and retains the containment gates in ADR-0006. This decision does not add a workstation TCP listener, a remote ACP deployment, or local Agent execution to Web/Mobile.

## Responsibilities and compatibility

`agentd` binds each client Session to an authenticated connection and one currently authorized Run, with a separate internal Goose Session. It validates context and route policy, mediates approvals, enforces budgets, filters output, and supervises worker lifecycle. Raw client fields, advertised capabilities, permission choices, and Goose extension calls cannot bypass these checks. A protocol cancellation result is not evidence that a worker's descendants have exited.

A third-party ACP client may reuse the admitted standard interaction subset after the same authentication and Run-binding checks. ACP support alone does not supply Threadline grants, enterprise approvals, or complete IM/Task/Publication UI. Required extensions must be negotiated and versioned; unsupported clients fail explicitly rather than receiving a wider scope. Exact bootstrap, Run-selection, reconnect, and idempotency schemas require a reviewed interface task and the bounded spike in the profile before an implementation is considered compatible. Credentials never travel as client-supplied `_meta` identity claims.

## Alternatives and consequences

A Threadline-only Agent RPC would align directly with business Protobuf types, but would duplicate standard session, streaming, and permission interaction and require a separate integration for other Agent clients. Direct UI-to-Goose ACP would leave engine-specific session and safety decisions in clients. The selected `agentd` interface concentrates those decisions in the supervisor and keeps clients independent of Goose internals.

The cost is two independently negotiated ACP connections plus explicit translation to Threadline Run/Approval facts; this is a policy implementation, not a transparent relay. The profile must prove that standard UX semantics survive filtering, supervision, and durable state reconciliation. No runtime code, wire schema, dependency, estimate, Gate, or production acceptance result changes in this documentation task. If compatibility or containment cannot be established, Agent execution remains unavailable while IM works; another interface choice requires a new ADR rather than an unsafe bypass.

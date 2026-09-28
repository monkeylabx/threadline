---
status: accepted
date: 2026-09-25
---

# ADR-0006: Rust agentd with a headless Goose runtime

Threadline Desktop will implement its local `agentd` supervisor in Rust and select Goose as the first Agent execution engine. The initial integration is a separately packaged, pinned Goose CLI binary running `goose acp` over stdio; Goose Desktop and its UI are not part of Threadline. This choice preserves the process and authorization boundaries in [ADR-0001](./0001-client-platform.md) while reusing an existing multi-model Agent loop. **Accepted architecture does not mean Goose has passed production security admission.**

## Context and decision

`locald`, `connectord`, and the Tauri host are already Rust-based, whereas the current Go `services/agentd/main.go` is only an empty process target. The delivery plan's Go `agentd` entry is superseded by this ADR; Go remains the language for the server-side Runtime Gateway and other planned server workloads. Mobile and Web remain Task controllers/observers and do not run a local Agent.

```text
Tauri UI ── versioned IPC ──> Rust agentd (signed Desktop sidecar)
                               ├─ Run/Lease/Fencing, budget, approval, audit events
                               ├─ authorized Context API ──> locald
                               ├─ capability-scoped tools ──> connectord
                               └─ supervised stdio ACP ──> Goose CLI worker
                                                         └─ approved model endpoint
```

The diagram shows ownership, not direct authority for Goose. Threadline owns Task and Run identity, durable state, current grants, exact-action approvals, model route policy, retention, and publication. A Goose session is an internal execution detail mapped to one authorized Run; a Channel is never a Goose session. Goose owns the model/tool loop and its working-context compaction. `agentd` turns ACP events into Threadline's versioned Run events and never treats model text or a compacted summary as authorization evidence.

Goose receives only a bounded, Run-scoped context bundle and explicitly allowlisted MCP tools. `locald` decides which message/file references may be decrypted and returned; `connectord` rechecks each workspace operation against current path, action, expiry, and revocation constraints. The worker must have no direct IM database/key access or general host filesystem permission. Model Control supplies an approved endpoint and short-lived route grant; the Goose worker may send only that Run's authorized prompt to that endpoint under an enforced egress policy. Model names are resolved through routing policy, not hard-coded into workflows.

The selected Goose binary is built or sourced from an exact reviewed release, signed with the Desktop app, and launched with minimal environment, isolated storage, and OS-level process/network/filesystem constraints. Neither a prompt instruction nor Goose's tool permission mode is an OS sandbox. Built-in host-capable tools, dynamic extension installation, cross-session recall, and global Memory are disabled unless a later security review admits a specific capability. Threadline provides its own permissioned knowledge and memory tools; Goose's local session history is not an IM fact source or an enterprise knowledge store. Its on-disk session/log data must be scoped, protected, retained, and erased according to the Run's policy before shipping.

## Why this shape

Goose officially exposes its full Agent as an [ACP subprocess over stdio](https://goose-docs.ai/docs/gdk/acp/). Its [Rust GDK](https://goose-docs.ai/docs/gdk/) now publishes components including [`goose-agent`](https://docs.rs/goose-agent/latest/goose_agent/) and [context management](https://docs.rs/goose-context-management/latest/goose_context_management/), but the SDK surface is alpha and its [documented in-process SDK](https://goose-docs.ai/docs/gdk/sdk/) is primarily provider-level. Starting with ACP uses the complete CLI behavior without embedding Goose's app internals into a privileged Threadline process. A later in-process GDK adoption requires its own compatibility and security decision; it must not silently remove the worker isolation boundary.

Keeping Go for `agentd` was viable with ACP, but would add another language and toolchain to the Desktop local service set while the present Go target has no implementation to preserve. Writing a new Agent loop would give more control but would also make Threadline responsible immediately for model adapters, tool turns, compaction, streaming, and recovery. Rust is chosen for local ownership and GDK compatibility, **not** because Rust alone enforces file, network, or approval policy.

## Admission and recovery gates

1. **Protocol and lifecycle:** Pin the Goose version; prove initialize, session creation/load, streaming, interruption, restart recovery, budget limits, and model-route changes against Threadline's Run contract on macOS, Windows, and Linux. Any Goose-specific unstable ACP methods used need compatibility tests.
2. **Capability boundary:** Start from a restrictive configuration with no default Developer/shell or dynamically installable extensions. Register only Threadline-owned tools; test unauthorized message and file reads, revoked grants, changed approval arguments, symlink escapes, and denied network destinations. Unavailable containment fails closed; ordinary IM remains usable.
3. **Cancellation and descendants:** On Stop, revoke tool capabilities, send ACP cancellation, terminate the contained worker and all descendants, and verify cleanup. A prior bounded [Goose ACP investigation in PR #211](https://github.com/monkeylabx/threadline/pull/211) observed a background shell child surviving `session/cancel`; the ACP response alone is not proof of termination. An external side effect with an uncertain outcome must be reconciled before retry, never blindly replayed.
4. **Data lifecycle:** Inventory Goose's session SQLite, logs, prompts, tool results, Memory, context files, and extension configuration. Prove per-Run/actor isolation, at-rest protection or ephemeral handling, retention/deletion, crash recovery, and no cross-Channel or private-to-team disclosure. Goose [stores session messages and tool results locally](https://goose-docs.ai/docs/guides/logs/), and its optional [Memory extension loads saved memories into prompts](https://goose-docs.ai/docs/mcp/memory-mcp/); neither behavior is accepted as Threadline's data policy by default.
5. **Release evidence:** Package and sign `agentd` and the pinned Goose worker with the Desktop release. Record license/dependency review, exact binary hash, minimum OS targets, resource limits, and a three-OS fault and security matrix. These are implementation gates, not checks passed by this documentation change.

If Goose cannot satisfy these gates, the Run feature remains unavailable while IM continues. Architecture/Security may separately evaluate a constrained GDK worker or another runtime; there is no automatic fallback to unrestricted Goose tools or an uncontained host shell. The Rust `agentd` and Goose integration are future P09 implementation work, including removal of the unused Go stub. P07/P09 estimates must be reviewed after the integration spike. This ADR changes no executable, protocol, or acceptance result today.

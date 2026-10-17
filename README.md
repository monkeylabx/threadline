# Threadline

**A team IM and Agent runtime: communicate with people, work privately with AI, and share results your team can review and continue.**

Threadline is an open-source, agent-native enterprise messaging project. Humans, agents, and services are first-class actors with explicit identities, responsibilities, permissions, and audit records—not a chatbot bolted onto team chat with standing access.

The product brings everyday channels and DMs together with a personal AI work surface. Its value hypothesis is less friction between making something with AI and having colleagues review, decide on, and reuse it—not more agents, or visibility into every employee's private AI process. Private work and team tasks both remain available.

Three boundaries shape the product:

- **Messaging and agent runtimes remain independent.** Teams can keep communicating when models or runtimes are offline.
- **Context and authority are granted per task.** An agent receives only the messages, files, and tools needed for its task; high-impact actions require visible, auditable approval.
- **Private work is not a shared Task Thread.** Referencing a channel does not publish personal prompts, drafts, files, or tool history. The author explicitly selects an Artifact version and destination when sharing; destination permissions still apply.

## First product slice

```text
E2EE Message
    → Local Agent Task
    → Capability-scoped Context
    → Human Approval
    → Verifiable Artifact
```

This is the frozen engineering slice for a **shared team task**, not a rule that all AI work must be public. A local runtime works inside an authorized workspace with explicit read scope, model egress, tool access, and budget. It requests approval before high-impact actions and returns authorized results and evidence to the original conversation. The complementary private-work-to-explicit-publication path is a product design with implementation contracts still pending; see [ADR-0005](./docs/adr/0005-private-work-publication-boundary.md).

Threadline is not a general-purpose AI control panel. Agent activity belongs in the collaboration context, but a Channel is not an Agent Session.

## Explore the interactive prototype

[Open the interactive HTML prototype](./docs/prototype/index.html), or serve it locally for desktop and mobile viewport testing:

```bash
python3 -m http.server 4173 --bind 127.0.0.1 --directory docs/prototype
```

Then visit [the default demo](http://127.0.0.1:4173/): desktop opens the confirmed V34 two-pane layout. To preview desktop in a narrow window, use [the desktop demo](http://127.0.0.1:4173/?viewport=desktop). See the [prototype guide](./docs/prototype/README.md) for the default and historical review routes.

The prototype covers channel collaboration, agent tasks, risk approvals, artifact delivery, files, search, runtime health, and enterprise administration. Narrow viewports route to the mobile renderer.

> [!IMPORTANT]
> This is a static product-review prototype. It does not start production services, connect a real agent, or execute local commands. The repository does not yet provide a one-command end-user installation.

## Project status

Threadline is building its foundational contracts and engineering substrate. It is not production-ready.

| Status | What it includes |
| --- | --- |
| Available now | Unified HTML product prototype, PRD, frozen scope, system architecture, threat model, and delivery plan |
| Engineering foundations | Multi-language workspace, Protobuf contracts and compatibility checks, selected authorization/capability/outbox storage modules, Rust FFI and E2EE technical validation, and development infrastructure |
| In progress | Identity and authorization facts, reliable event delivery, encrypted client core, service assembly, and reproducible builds |
| Planned | Usable desktop and mobile clients, the real local-agent vertical slice, complete E2EE messaging, enterprise installers, and formal security review |

Do not use the current repository for production messages, confidential data, or high-impact agent actions.

## Why Threadline

The intended benefit is a shorter handoff from personal AI work to team collaboration: preview the actual result, discuss its version, make a decision, and continue the work without joining the author's private AI conversation. Messaging remains useful even when AI is unavailable. This is a product hypothesis, not evidence of customer willingness to switch or pay; compare it against existing IM and AI workflows using handoff time, repeated context explanation, and colleague reuse of results.

Chatbots usually answer questions. Agents can read files, run tools, and change external systems. That requires clearer governance boundaries:

- Every agent has a distinct identity, owner, responsibility, and participation mode.
- Every execution has its own task, run, budget, and cancellation semantics.
- Message, file, workspace, and tool access are authorization decisions.
- Capability Grants are short-lived, scoped, and revocable; they are not Channel Memberships.
- Deletion, publishing, merging, and external sending require approval when high impact.
- Prompts are assembled in an authorized runtime and sent directly to an enterprise-approved model endpoint; IM services and Model Control do not proxy prompt content.
- Message bodies and attachments are end-to-end encrypted by default; enterprise recovery is isolated from daily application services and requires multi-party approval.

See [`CONTEXT.md`](./CONTEXT.md) for the domain language.

## Repository guide

| Path | Purpose |
| --- | --- |
| [`docs/prototype/index.html`](./docs/prototype/index.html) | Product-design entry point and interactive prototype |
| [`docs/product-requirements.md`](./docs/product-requirements.md) | Product requirements, boundaries, and end-to-end behavior |
| [`docs/acceptance/scope.md`](./docs/acceptance/scope.md) | Frozen scope and acceptance baseline |
| [`docs/architecture/`](./docs/architecture/) | System, service, data, and security architecture |
| [`proto/`](./proto/) | Cross-platform Protobuf contracts, golden frames, and generation configuration |
| [`services/`](./services/) | Go server workloads, including control-plane, realtime, worker, and Runtime Gateway |
| [`crates/`](./crates/) | Rust client core, crypto, FFI, locald, agentd, and connector boundaries |
| [`apps/`](./apps/) | Desktop, iOS, Android, and admin client skeletons |
| [`deploy/`](./deploy/) | Local development stack and private-deployment foundations |

## Architecture principles

- Keep IM, control plane, agent runtime, and local connector boundaries explicit.
- Coordinate through events and capability contracts rather than shared databases.
- Keep model names out of workflow code; select models through discoverable routing policy.
- Server, realtime, worker, and Model Control services handle only ciphertext envelopes and necessary metadata.
- Local connectors access files and tools only through user-authorized capabilities.
- Desktop, web, and mobile share protocol semantics while retaining native implementations.

The selected local runtime is **Rust `agentd` supervising a pinned, headless Goose worker over ACP stdio**.
The Desktop UI uses Threadline's task interface through the Tauri host; ACP stays internal to `agentd` and Goose.
Threadline owns Task/Run authority, grants, approvals, Connector effects and publication; Goose supplies the model/tool loop.
The early self-owned core and Codex research are historical alternatives. Existing [agentd primitives](./crates/agentd/README.md)
do not establish a complete Goose integration or production admission.

Authoritative references:

- [System architecture and communication protocol](./docs/architecture/system-architecture.md)
- [Service catalog](./docs/architecture/service-catalog.md)
- [ADR-0006: Rust agentd with a headless Goose runtime](./docs/adr/0006-rust-agentd-goose-runtime.md)
- [ADR-0007: Task-oriented client interface with internal ACP](./docs/adr/0007-agentd-task-interface.md)
- [Private Enterprise v1.0 delivery plan](./docs/delivery-plan.md)
- [Asynchronous agent workflow](./docs/agent-workstreams.md)

## Contributing

Read [`CONTRIBUTING.md`](./CONTRIBUTING.md) and the repository-level [`AGENTS.md`](./AGENTS.md) before starting. GitHub Issues are the source of truth for tasks and product requirements. Engineering tasks are marked `ready-for-agent` only after dependencies, owned paths, and acceptance checks are clear.

Useful ways to contribute include:

- Test the prototype and report specific workflow or usability feedback.
- Discuss real-world agent authorization, approval, and audit scenarios.
- Claim an Issue marked `ready-for-agent`.
- Improve reproducible builds, cross-platform validation, and security testing.

Do not disclose secrets, message bodies, prompts, tokens, or enterprise data in public Issues. Until a dedicated `SECURITY.md` and private reporting channel exist, use a maintainer's public profile to request a private security contact.

## Commercial use and support

Threadline is licensed under the Apache License 2.0 and may be studied, modified, integrated, and used commercially subject to the terms in [`LICENSE`](./LICENSE).

Private deployment, enterprise identity and model integration, security reviews, custom connectors, upgrade assistance, and commercial support may be offered in the future. No paid plan or SLA is currently available; this design does not introduce Enterprise feature gates or paid privacy controls. Teams interested in design collaboration or a private pilot can contact the maintainers through a GitHub Issue without posting sensitive environment details.

## License

Licensed under the [Apache License 2.0](./LICENSE).

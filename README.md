# Threadline

**Delegate work to local AI agents from team conversations while keeping access, activity, and human approval visible.**

Threadline is an open-source, agent-native enterprise messaging project. Humans, agents, and services are first-class actors with explicit identities, responsibilities, permissions, and audit records—not a chatbot bolted onto team chat with standing access.

Two boundaries shape the product:

- **Messaging and agent runtimes remain independent.** Teams can keep communicating when models or runtimes are offline.
- **Context and authority are granted per task.** An agent receives only the messages, files, and tools needed for its task; high-impact actions require visible, auditable approval.

## First product slice

```text
E2EE Message
    → Local Agent Task
    → Capability-scoped Context
    → Human Approval
    → Verifiable Artifact
```

The target experience starts when a team turns part of a conversation into a task. A local runtime works inside an authorized workspace with explicit read scope, model egress, tool access, and budget. It requests approval before high-impact actions and returns results and evidence to the original conversation.

Threadline is not a general-purpose AI control panel. Agent activity belongs in the collaboration context, but a Channel is not an Agent Session.

## Explore the interactive prototype

[Open the interactive HTML prototype](./docs/prototype/index.html), or serve it locally for desktop and mobile viewport testing:

```bash
python3 -m http.server 4173 --directory docs/prototype
```

Then visit <http://localhost:4173>.

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
| [`services/`](./services/) | Go control-plane, realtime, worker, and runtime services |
| [`crates/`](./crates/) | Rust client core, crypto, FFI, locald, and connector boundaries |
| [`apps/`](./apps/) | Desktop, iOS, Android, and admin client skeletons |
| [`deploy/`](./deploy/) | Local development stack and private-deployment foundations |

## Architecture principles

- Keep IM, control plane, agent runtime, and local connector boundaries explicit.
- Coordinate through events and capability contracts rather than shared databases.
- Keep model names out of workflow code; select models through discoverable routing policy.
- Server, realtime, worker, and Model Control services handle only ciphertext envelopes and necessary metadata.
- Local connectors access files and tools only through user-authorized capabilities.
- Desktop, web, and mobile share protocol semantics while retaining native implementations.

Authoritative references:

- [System architecture and communication protocol](./docs/architecture/system-architecture.md)
- [Service catalog](./docs/architecture/service-catalog.md)
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

Private deployment, enterprise identity and model integration, security reviews, custom connectors, upgrade assistance, and commercial support may be offered in the future. No paid plan or SLA is currently available. Teams interested in design collaboration or a private pilot can contact the maintainers through a GitHub Issue without posting sensitive environment details.

## License

Licensed under the [Apache License 2.0](./LICENSE).

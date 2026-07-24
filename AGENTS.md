# Threadline Agent Instructions

## Product boundary

- Threadline is an enterprise IM with first-class, permissioned Agent actors.
- The IM must remain usable when models and Agent runtimes are offline.
- A Channel is not an Agent session. Runtime context must be explicit and scoped.
- Local files are accessed only through a user-authorized local connector.
- High-impact actions require visible, auditable approval.

## Engineering defaults

- Keep IM, control plane, Agent runtime, and local connector boundaries explicit.
- Prefer event and capability contracts over shared database access.
- Do not place model names directly in workflow code; use discoverable routing policy.
- Treat all message, file, workspace, and tool access as authorization decisions.
- Preserve desktop, web, and mobile compatibility in product and protocol choices.

## Asynchronous agent workflow

- Read `docs/delivery-plan.md` and `docs/agent-workstreams.md` before starting implementation.
- Work from one claimed GitHub issue in a dedicated branch and Git worktree. Never implement directly on `main`.
- Keep one task to one primary ownership area. Do not edit another workstream's paths without an explicit contract task.
- Treat `proto/`, database migrations, generated SDKs, workspace manifests, and lockfiles as integration-owned surfaces.
- Define cross-workstream behavior in Protobuf or a documented interface before depending on another agent's implementation.
- Keep agent-sized tasks between half a day and two days. Delivery-plan items are work packages and must be split before coding.
- Finish with the handoff format in `docs/templates/agent-handoff.md`, including commit, tests, contract changes, risks, and unblock notes.
- A task is not complete until its acceptance checks pass and the integration owner can merge it without relying on uncommitted local state.

## Frontend direction

- Treat `docs/prototype/index.html` as the single product-design entry and `docs/prototype/` as the source of truth.
- Desktop/web behavior lives in the root prototype; narrow viewports are routed to the internal renderer in `docs/prototype/mobile/`.
- Maintain and review the product design directly in the HTML prototype. Figma exports are not part of the active workflow.
- Daily IM surfaces should feel native, quiet, and work-focused.
- Agent activity belongs in the conversation context through sheets and activity layers.
- Avoid oversized typography, heavy black rules, decorative dashboards, and card nesting.

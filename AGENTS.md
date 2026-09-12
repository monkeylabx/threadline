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

## AI-auditable code generation

- Before changing handwritten code, read `docs/agents/ai-auditable-coding.md` and treat every `MUST` and `MUST NOT` as an acceptance criterion.
- Design before generating: state the unit's single responsibility, invariants, inputs, outputs, failure modes, and test seam; split mixed responsibilities before writing code.
- New or materially changed handwritten functions must stay within 60 non-comment lines, 40 statements, cyclomatic complexity 10, cognitive complexity 15, nesting depth 3, and 5 parameters.
- New or materially changed types and modules must have one primary responsibility and one cohesive set of invariants. Split disconnected state or behavior clusters; 250 lines per type, 300 lines per handwritten file, or 11 methods triggers mandatory redesign. Crossing 350 type lines, 500 file lines, or 15 methods requires a documented exception.
- Behavioral changes must include tests in the same task. Bug fixes require a regression test; unit tests must be deterministic, independent, and hermetic; critical authorization, capability, crypto, recovery, audit, and idempotency branches require positive and negative cases.
- Keep human-authored diffs reviewable: target at most 400 changed lines and split changes above 800 lines. Separate behavior changes from mechanical refactors and generated output.
- Never silently suppress a quality rule. Any exception must identify the reason, scope, owner, and removal condition in the Issue and handoff. Generated code, vendored code, declarative schemas, migrations, fixtures, and protocol output use the exception rules in the detailed standard.

## Agent skills

### Issue tracker

Tasks and PRDs are maintained in GitHub Issues for `monkeylabx/threadline`. See `docs/agents/issue-tracker.md`.

### Triage labels

Use the five canonical triage labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, and `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Use a single-context layout: `CONTEXT.md` at the repo root for domain language and `docs/adr/` for architecture decisions. See `docs/agents/domain.md`.

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

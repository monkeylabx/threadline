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

## Frontend direction

- Use the `docs/prototype/visual-v2/` benchmark as the current visual reference.
- Daily IM surfaces should feel native, quiet, and work-focused.
- Agent activity belongs in the conversation context through sheets and activity layers.
- Avoid oversized typography, heavy black rules, decorative dashboards, and card nesting.

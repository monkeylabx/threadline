# V34 Two-pane Prototype Handoff

## Identity

- Issue: https://github.com/monkeylabx/threadline/issues/236
- Workstream: product prototype
- Branch: `codex/236-two-pane-prototype`
- Base commit: `b045d402ebf45537b1ac24bfbe4413465114d235`
- Implementation head commit: `f5954c5974ad67a8d1b54e86114dd760e9e57b7c` (this handoff is a subsequent documentation commit)

## Outcome

- Completed: one sidebar with enterprise identity, Messages/Work/Attention switch, contextual lists and profile; main conversation; resizable/collapsible sidebar; dismissible native-dialog approval/result sheets; in-memory decisions and pending indicators; channel request summary reflects decisions.
- Completed: V34 review instructions and screenshots in `screenshots/v34/`.
- Not completed: production IM/runtime/file operations, new mobile design, Existing placeholder actions outside this layout task remain placeholders.
- Acceptance status: scoped visual and interaction checks passed. User accepted V34 on 2026-10-17 and requested squash merge to main. Default entry and product baseline documentation updated; PR integration follows.

## Changed Surfaces

- Owned paths: `docs/prototype/`, root `README.md`, `docs/product-requirements.md`, `docs/design/prototype-state-matrix.md`. Expanded documentation ownership explicitly follows the user request to accept V34 and update the default demo.
- Contract changes: none.
- Migration changes: none.
- Generated or lockfile changes required: none.

## Verification

```text
node --check docs/prototype/app.js  # passed
node --check docs/prototype/entry-route.js  # passed
node --test docs/prototype/entry-route.test.mjs  # 4 passed
git diff --check                  # passed
python3 -m http.server 4173 --bind 127.0.0.1 --directory docs/prototype
```

Manual checks through the Codex in-app browser:

- Desktop CSS viewport widths 1200px, 750px and native preview approximately 718px: two grid columns, no horizontal overflow; private-work view visually checked.
- Sidebar collapse/restore and keyboard Right resize (240 -> 250) passed; resize key did not change prototype variants.
- Search `入职` -> private work -> back to channel passed.
- Open approval from channel, approve/reject in separate fresh-page checks, updated status/pending count, no repeated decision actions passed.
- Close button, native Escape and backdrop click dismiss; focus returns to opener.
- Result confirmation -> pending count zero -> return to private work passed; no auto-publication.
- Narrow viewport routed through the root entry to `mobile/index.html?view=channel`; mobile channel and composer visually checked. Mobile files unchanged.
- Final preview browser error log empty.

## Security And Data

- Permissions or trust-boundary impact: none. All state is in memory, refresh resets it. Sample file names are illustrative; no connector or runtime is invoked.
- Message, file, prompt, token or key handling impact: none.
- Logging and telemetry review: no new logging or telemetry.

## Risks And Decisions

- User accepted V34 as the current desktop product baseline on 2026-10-17. V33 is retained only as design history.
- Approval sheets use native modal dialogs; background is inert while open. Sidebar is inert when collapsed.
- Historical B/C routes remain accessible by URL. Current A hides the floating variant switcher and disables its global arrow switching to prevent interference with daily interaction.
- Failed approach worth preserving: inherited historical three-column CSS required scoped V34 overrides; a zero-basis list flex value initially collapsed rows, corrected before delivery.
- No ADR or protocol change required. Root README, prototype guide, current PRD baseline and navigation matrix now agree on V34.
- Entry routing design: single responsibility is selecting the default desktop route. Inputs are query text; output is URLSearchParams; explicit prototype, non-channel screen, modal and mobile view parameters take precedence. No I/O or authorization effects. Pure VM-loaded function is the hermetic test seam. The helper and call site can be independently reverted.
- Scoped legacy-size exception under `docs/agents/ai-auditable-coding.md` sections 7–8: `docs/prototype/app.js` is 860 nonblank, non-`//` lines versus 811 on the latest main prototype (manual line count), exceeding the 500-line ceiling. Most V34 layout handlers predate the policy's arrival in this task's baseline. Splitting all shared historical-prototype state during an accepted-layout integration would expand scope and add cross-variant regressions; the new default-entry policy is instead isolated in a 12-line helper with deterministic positive/negative tests. Owner/reviewer: repository maintainer `monkey-lab-x`. Exception expires at the next substantial prototype behavior change or when the prototype is replaced by the desktop implementation; that task must extract the A shell into its own module. Browser workflow evidence and four routing tests compensate for this bounded integration.

## Next

- Issues unblocked: none.
- Blocking condition: required GitHub PR checks must pass before squash merge.
- Recommended next task: integrate the CI-verified PR and serve the main checkout at the default demo entry.

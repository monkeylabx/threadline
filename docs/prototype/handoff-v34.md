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
- Not completed: production IM/runtime/file operations, new mobile design, or formal product approval. Existing placeholder actions outside this layout task remain placeholders.
- Acceptance status: scoped visual and interaction checks passed. Ready for product review, not merged into main.

## Changed Surfaces

- Owned paths: `docs/prototype/index.html`, `app.js`, `styles.css`, `README.md`, `screenshots/v34/`, this handoff.
- Contract changes: none.
- Migration changes: none.
- Generated or lockfile changes required: none.

## Verification

```text
node --check docs/prototype/app.js  # passed
git diff --check                  # passed
python3 -m http.server 4174 --bind 127.0.0.1 --directory docs/prototype
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

- User agreed to try the suggested two-pane layout; V34 still awaits visual product review. V33 confirmed design history remains documented.
- Approval sheets use native modal dialogs; background is inert while open. Sidebar is inert when collapsed.
- Historical B/C routes remain accessible by URL. Current A hides the floating variant switcher and disables its global arrow switching to prevent interference with daily interaction.
- Failed approach worth preserving: inherited historical three-column CSS required scoped V34 overrides; a zero-basis list flex value initially collapsed rows, corrected before delivery.
- No ADR or protocol change required. Root README still names the previously confirmed V33; update it together with product docs if V34 is accepted for integration.

## Next

- Issues unblocked: none.
- Blocking condition: product/integration owner reviews V34 before merging.
- Recommended next task: review the two-pane layout and decide whether it replaces the confirmed V33 baseline.

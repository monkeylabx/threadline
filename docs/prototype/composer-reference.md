# Toolbar-first composer reference draft

- Issue: #430; branch `codex/430-reference-composer-layout`.
- Question: does the right-hand message editor feel clearer with tools above the writing area, matching the supplied reference?
- N=1: reference-led draft on the existing V34 channel route. Palette, sidebar and macOS toolbar stay as approved.
- Preview: `/?viewport=desktop&screen=channel&prototype=im-agent-fusion&variant=A&ui=v34&review=430`.
- Run: `python3 -m http.server 58266 --bind 127.0.0.1 --directory docs/prototype` (the existing preview server already serves this worktree).

## Draft layout

The composer is a continuous part of the white conversation panel, with one quiet upper boundary. Tools precede the editor; the send affordance and keyboard hint sit beneath it. There is no nested rounded input card. At a 1280×800 CSS viewport, the draft measures 201px overall, with a 52px tools row and a 96px empty editor. Text growth still follows the existing prototype sizing handler and scrolls at its cap. Viewport-relative limits leave conversation history usable at larger text sizes.

Toolbar icons use existing global neutral/disabled policy; the send affordance uses the shared primary policy. All palette values come from the existing generated tokens. Emoji, attachments, capture, formatting and more remain unavailable; no new working capabilities or real send integration are claimed. The candidate stylesheet is isolated in `composer-reference.css`; production geometry tokens and Desktop code are untouched.

## Verification and verdict

- Existing entry routing: 4/4 PASS; existing app script syntax PASS.
- Browser QA at 1280×800 and 760×640, each at 100%/200% text: 4/4 PASS. The iframe follows actual available viewport height, not a fixed large test canvas.
- Checks: tools above editor, actions below it, history area at least 96px, panel flush sides/bottom, no rounded inner card, matching panel background, no native resize grip, roomy minimum, draft growth/cap and clear reset.
- Old-card mutation: FAIL as intended; restoring draft styles: PASS.
- No backend, native UI, shared token, schema, dependency, lockfile or data changes.

Verdict: pending user review. This is a prototype-only branch; do not merge or port it to Desktop until the user chooses this direction. A production change must define the accepted shared geometry contract and retain current input/IME/send behavior.

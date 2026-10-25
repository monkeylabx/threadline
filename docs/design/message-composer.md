# Message composer presentation contract

The accepted V34 composer is the reference for this component. Authoritative
portable values live in packages/design-tokens/src/tokens.json. The fixed
TypeScript, Swift, Kotlin and CSS adapters publish the same roles; product pages
consume these roles instead of adding near-duplicate dimensions or state colors.

| Role | At16px root | Use |
| --- | --- | --- |
| space.composerOuterInset |20px| Side and bottom breathing room |
| space.composerInputInset |13px| Textarea padding |
| space.composerFieldMin | 43px | Resting input height: one caption line, scalable |
| space.composerFieldMax | 162px | Largest input height: eight caption lines, then scroll |
| space.composerFieldGap |3px| Explicit gap between field and toolbar |
| space.composerToolbarInset |10px| Horizontal toolbar padding |
| space.composerControl |26px| Attachment and send control box |
| space.composerIconWidth / composerIconHeight |13px /18px| Reference icon geometry |
| radius.composerTool / composerSend |5px /9px| Tool and send control corners |

The gap replaces the prototype's accidental inline-textarea baseline gap.
Both consumers use an explicit grid, block textarea and the shared gap. With
the existing 6px bottom padding and 26px controls, the toolbar is 32px. The
field is content-driven: it rests at one caption line (2 × 13px inset + 16.8px
line, rounded up to 43px), grows with the draft line by line, and starts to
scroll only after eight full lines (162px). The composer therefore totals 78px
at rest and at most 197px at a 16px root, doubling at 200% text. Dimensions are
rem, so these bounds scale with text size. The textarea has no native resize
handle. Input and toolbar use the existing caption role12px and
its line height. The old prototype10px toolbar is migration inventory, below the
already-reviewed12px schema minimum; that minimum must not be relaxed.
The toolbar's text slot carries the muted keyboard hint
"Enter 发送 · Shift+Enter 换行" and truncates with an ellipsis; it is not a control.

Shared CSS custom properties use kebab-case for colors, spaces, radii and type
roles. The existing radius.full value remains pixels rather than rem. Theme
colors and all interaction states remain governed by interaction-states.md;
this contract changes no palette, hover/pressed/selected/disabled precedence.

The CSS adapter is pure and deterministic; generation and byte verification
have no product side effects. It emits role data only, no selectors for product
layout and no events. Schema/contrast validation stays in the existing verifier.
The legacy generator orchestrator only renames the adapter call; its size and
control flow do not grow, so the existing#400 exception does not expand.

Consumer acceptance must exercise the real Composer with the real stylesheet:
compare the reference textarea, toolbar/control radii, insets and total geometry;
cover100%/200% text and short/long/offline scenes. Check the resting field,
growth to the eight-line cap and scrolling past it at both text sizes. Preserve
Enter/Shift+Enter, IME composition, empty/disabled sends, shared focus and state feedback. No
Repository, outbox, authorization or native-session behavior changes. A browser
visual fixture does not establish real native network delivery.

Revert consumers before reverting these additive presentation roles. No stored
state, migration, dependency or lockfile changes are required.

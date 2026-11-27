# Message composer presentation contract

The desktop V34 channel in [the HTML prototype](../prototype/index.html) is the
visual reference. Inside one inset frame, the tool row precedes the textarea.
The muted keyboard hint and send action share a bottom row. Layout uses
existing spacing, type, radius and semantic color roles published by the
TypeScript and CSS token adapters.

| Element | Geometry at a 16px root |
| --- | --- |
| Frame | 20px side and bottom insets, 20px horizontal inner padding, 12px corners, 1px border |
| Top tool row | 32px square controls, 5px corners, 20px glyphs and 12px gaps |
| Textarea | Body-compact type, 8px vertical padding, no native resize handle |
| Editor height | At least the smaller of 96px and 18vh, at most the smaller of 162px and 24vh |
| Bottom actions | 12px caption, 32px send height, 64px minimum width, 9px send corners |

Spacing and radii follow the root text size. Tool controls and glyphs cap at
40px and 24px to leave space for the editor at 200%. In windows up to 700px
high, tighter vertical padding, tool/send heights capped at 32px/40px and
14vh/18vh editor bounds leave room for history and the connection banner. The draft grows up to the editor height cap and scrolls within the textarea after reaching it. Clearing
the draft restores the resting height. The timeline scrolls independently.
The hint truncates in narrow windows.

The frame uses the shared surface and border roles. Focus within the composer
uses the shared focus ring. Tool and send states follow the
[interaction policy](interaction-states.md). The six toolbar tools remain
disabled until their capabilities are connected. Desktop keeps its existing
Enter, Shift+Enter, IME, empty-draft and unavailable-send behavior.

The real browser fixtures check the frame, row order, draft growth and cap,
and independent history scrolling at 100% and 200% in regular and compact
windows. Desktop unit tests cover the form behavior. The fixtures exercise
rendering and input locally. Repository, outbox, authorization, native
session and persisted data contracts are unchanged by this layout.

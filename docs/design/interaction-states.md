# Shared control interaction states

The palette is authoritative in `packages/design-tokens/src/tokens.json`. The
state policy is authoritative in `packages/design-tokens/src/interaction.css`.
The fixed generator publishes CSS colors plus a byte-verified copy of the policy
under `docs/prototype/generated/`; Desktop consumes the same source policy.
Product selectors bind background/color to `--tl-control-background` and
`--tl-control-foreground`. They never assign a component-specific state color.

| State | Light background | Foreground | Meaning |
| --- | --- | --- | --- |
| Neutral rest | transparent | textSecondary | ordinary row or quiet button |
| Tinted rest | navigationControlSurface `#DCE6F7` | textSecondary | individual navigation button or menu trigger |
| Hover | controlHoverSurface `#D7E4F8` | textPrimary | pointer feedback, no persistent selection |
| Pressed | controlPressedSurface `#C9DAF3` | textPrimary | only while pointer or keyboard is held |
| Selected/open rest | controlSelectedSurface `#DCE7FF` | controlSelectedText `#204FA8` | stays after pointer exit |
| Selected hover | controlSelectedHoverSurface `#D0DFF8` | controlSelectedText | hover preserves selection |
| Selected pressed | controlSelectedPressedSurface `#C2D5F4` | controlSelectedText | pressed feedback preserves selection |
| Disabled | controlDisabledSurface `#E9EDF3` | textMuted | overrides hover, pressed and selected |

Primary actions share accent/accentHover/actionPressed (`#4467B8`, `#365DA9`,
`#2C4F91`) and onAccent. Danger actions use danger for resting text, dangerSubtle
on hover, and danger/onAccent while pressed. Status badges and decorative avatars
are not interactive controls and keep their semantic status/identity roles.

## Consumer interface

Mark a container with `data-tl-interaction-scope`. Native buttons and
`role="button"` controls inside receive state custom properties. An individual
control may choose `data-tl-control="tinted"`, `primary`, or `danger`; the default
is neutral. Persistent selection comes from `.is-current`, `.is-active`,
`aria-current="true"/"page"`, `aria-selected="true"`, `aria-pressed="true"`, or
`aria-expanded="true"`. Disabled wins last. Do not add click feedback to a static
current-device card or claim that disabled future features are implemented.

All four modes expose explicit identical role names; consumers neither derive
nor invert colors. Existing contrast gates remain. Additional pairs cover normal
hover/press text, all selected states, disabled text, primary press and danger
press. Lowest new light pair is disabled text at 4.66:1. Keyboard focus keeps the
shared focusRing; no animation is needed to communicate state. Native mobile uses
the same generated state roles with native pointer/touch/selection semantics.

Generation has no product side effects. The pure CSS adapter is deterministic,
hermetic and covered at 100% lines/branches/functions. Revert consumers before
reverting this additive contract; no migration or persisted-state change exists.

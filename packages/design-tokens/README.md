# Threadline design tokens

One versioned JSON source generates platform-native value adapters for
TypeScript, Swift, and Kotlin. The package shares design decisions, not UI
component code.

```text
src/tokens.json                        authoritative values
schema/design-tokens.schema.json       machine-readable shape
generated/typescript/tokens.ts         Desktop/Admin Web adapter
generated/swift/ThreadlineTokens.swift iOS adapter
generated/kotlin/ThreadlineTokens.kt   Android adapter
examples/                              platform consumption examples
scripts/design-tokens.mjs              fixed generate/verify commands
scripts/validate-tokens.mjs            schema and contrast invariants
scripts/render-*.mjs                   one deterministic renderer per output
scripts/outputs.mjs                    which inputs feed which generated files
```

From the repository root:

```sh
node packages/design-tokens/scripts/design-tokens.mjs generate
node packages/design-tokens/scripts/design-tokens.mjs verify
```

The package-local `npm run generate` and `npm run verify` aliases run the same
dependency-free commands.

`verify` validates schema invariants, WCAG contrast pairs, Dynamic Type
mappings, the 200% scaling contract, reduced-motion behavior, and byte-for-byte
generated output. Generated files are never edited directly.

See [the token contract](../../docs/design/tokens.md) for platform rules and
change policy.

## Approved V34 palette (2026-10-23)

The light palette follows the accepted source-of-truth HTML prototype (#358 / #359): neutral surfaces and blue emphasis. `chrome` is the window toolbar, `surfaceHover` is the navigation hover surface, `selection` is the current navigation background, and `unread` is its notification foreground. Use `accent` on `selection`; use `radius.control` for the search field and `radius.xl` for rounded navigation. All roles are available in every mode. Existing dark and high-contrast values are preserved; new roles in those modes reuse their corresponding semantic surfaces and notification colors.

Declared contrast pairs cover selected navigation, toolbar labels, muted sidebar text and unread indicators in addition to the existing content/action pairs. Consumers use these shared names rather than copying prototype hex values. Native window controls remain platform-owned.

## V34 navigation refinement (#393)

`navigationSurface` separates the rounded sidebar from the neutral window surface and white conversation. Light navigation uses the prototype's pale gray-blue; light accent/hover use its softer blue. Desktop consumes these roles directly. Other modes retain their existing palette, with navigation mapped to `surface`. Primary, secondary, muted and unread navigation foregrounds are verified in every mode.

`navigationAccent` keeps the sidebar's selected controls at the user's previous vivid blue (#245BDB in light mode), independent of the softer conversation accent. Other modes reuse their existing accent. Both navigation and selection contrast pairs are verified at the unchanged4.5 threshold.

The selected light navigation background is the softer gray-blue #EAF0F8. Conversation surfaces and emphasis remain unchanged. Existing muted navigation text contrast remains above 4.5.

`navigationControlSurface` gives each navigation logo and the profile area a visible background separate from the softer sidebar. Light mode uses #DCE6F7; other modes reuse their existing elevated surface. Use primary/secondary text or navigationAccent on this surface; all three pairs keep the 4.5 contrast gate. This role does not change hover or selection colors.

## Shared control states (#399)

Use the [interaction contract](../../docs/design/interaction-states.md) and
`src/interaction.css` for all hover, pressed, selected/open and disabled colors.
`generated/css/colors.css` and the prototype's generated CSS are deterministic
outputs of the same command. Colors stay in tokens.json; no consumer derives a
nearby shade. A selected resting control remains selected after pointer exit.

Disabled controls keep their variant's resting surface and mute only the
foreground; filled primary controls fall back to `controlDisabledSurface` (#405).

## V34 dark, high-contrast and identity roles (#404)

The V34 palette now covers every mode. Dark mode replaces the earlier green
surfaces with neutral graphite (`surface` window, a lifted `canvas`
conversation) and the same blue emphasis family as light mode; high-contrast
light and dark keep their black/white structure with blue accents. Statements
above that other modes "retain their existing palette" describe the state before
this change. Light values are unchanged.

`identitySlate`, `identityPlum`, `identityClay` and `identityAmber` are the
prototype's person avatar fills; `onIdentity` is their initial. Consumers map a
stable author key to one role and never derive a nearby shade; Agent avatars
keep `accentSubtle` with `accentHover`. `test/palette.test.mjs` keeps every
mode's emphasis roles in the blue family and requires a contrast pair for each
identity role.

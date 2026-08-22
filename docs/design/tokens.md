# Cross-platform design tokens

Threadline uses one machine-readable token source for web/desktop, iOS, and Android. The authoritative source is [`packages/design-tokens/src/tokens.json`](../../packages/design-tokens/src/tokens.json); generated files are checked in so each client can consume the same reviewed values without a runtime generator.

## Contract

[`design-tokens.schema.json`](../../packages/design-tokens/schema/design-tokens.schema.json) publishes the portable shape. The dependency-free verifier enforces the same contract and additional executable invariants. It defines:

- semantic colors for light, dark, high-contrast light, and high-contrast dark modes;
- typography roles with web `rem`, iOS Dynamic Type style, and Android `sp` mappings;
- logical spacing, radius, and layer scales;
- motion durations, easing, and a fail-closed reduced-motion policy; and
- accessibility thresholds and foreground/background contrast pairs.

Product code must consume semantic names such as `textPrimary`, `surfaceElevated`, and `danger`. A page must not introduce an approximate color, spacing, type size, radius, layer, or animation value when an equivalent token exists. Generated files are outputs, not editing surfaces.

## Generate and verify

From `packages/design-tokens`:

```sh
npm run generate
npm run verify
```

Generation is deterministic and writes the source SHA-256 into every output:

- TypeScript: `generated/typescript/tokens.ts`
- Swift: `generated/swift/ThreadlineTokens.swift`
- Kotlin: `generated/kotlin/ThreadlineTokens.kt`

Verification checks schema-level invariants, identical semantic color keys across modes, WCAG contrast for every declared pair, scalable typography, monotonically increasing size scales, the reduced-motion contract, byte-for-byte generated output, and the absence of raw color/size values in examples.

## Theme and accessibility behavior

The operating-system theme is the default selector. An explicit user preference may select light or dark, while a platform high-contrast setting selects the matching high-contrast mode. Product code must not infer a color from another color or invert a mode locally.

Web and desktop typography uses `rem`. Layouts must reflow without clipped content at 200% browser zoom and must not use fixed-height text containers. iOS maps `swiftTextStyle` to Dynamic Type and applies font metrics rather than fixed point sizes. Android uses `sp`, honors the current `fontScale`, and reflows rather than truncating a control at larger accessibility sizes. All platforms must remain usable at the declared `maximumTextScale` of 2×.

When the platform requests reduced motion, nonessential spatial movement and duration are multiplied by zero. Essential state communication must remain available without relying on animation. Opacity changes are allowed only when they do not hide state or delay interaction, and the platform preference always wins.

## Platform consumption

The examples under [`packages/design-tokens/examples`](../../packages/design-tokens/examples) show direct consumption for TypeScript, SwiftUI, and Jetpack Compose. Native adapters may translate the generated string colors into platform color objects, but they must not substitute different values or rename the semantic roles.

Existing prototype hard-coded values are migration inventory, not additional token authority. Page implementation tasks should replace them with these semantic tokens within their own owned paths; T013 deliberately does not edit product-page surfaces.

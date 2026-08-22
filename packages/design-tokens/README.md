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

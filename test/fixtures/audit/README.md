# P03-07A Audit/Retention fixtures

These fixtures are synthetic public contract evidence. They contain no
message/file plaintext, Prompt, Token, Grant, key, nonce, signature, recovery
material, workspace path, or diagnostic text.

`scenarios.json` freezes one three-Event tenant chain, its trusted head, the
Outbox replay evidence context, and required fail-closed mutations.
`manifest.json` pins the source digest and required case registries.

Verify with:

```text
node proto/tools/verify-audit-retention-contracts.mjs
```

The fixture demonstrates local chain integrity with a trusted head. It does
not claim database-owner resistance or suffix-truncation detection without a
future external checkpoint.

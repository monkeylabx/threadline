# Domain Docs

Threadline uses a single-context domain documentation layout.

## Before Exploring Code

- Read `CONTEXT.md` at the repository root when it exists.
- Read ADRs in `docs/adr/` that affect the area being changed.
- If these files do not exist, proceed silently instead of asking for empty placeholders.
- Add domain language or architecture decisions with the domain-modeling skill after the terms or decisions are resolved.

## File Structure

```text
/
|-- CONTEXT.md
|-- docs/
|   |-- agents/
|   `-- adr/
`-- src/
```

## Domain Language

Use terms defined in `CONTEXT.md` in Issues, implementation plans, test names, and code. When a needed term is missing, determine whether it exposes a domain-model gap instead of introducing an unreviewed synonym.

## ADR Conflicts

Explicitly flag proposals that contradict an existing ADR. Do not silently override a recorded decision.

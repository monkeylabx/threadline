# Contributing to Threadline

## Before coding

1. Read `AGENTS.md`, `docs/delivery-plan.md`, and `docs/agent-workstreams.md`.
2. Claim one ready GitHub Issue and record its base commit, branch, worktree, owned paths, and verification commands.
3. Work in a dedicated `agent/<task>-<slug>` branch and worktree. Do not implement on `main`.
4. Define cross-workstream behavior in Protobuf or a documented interface before depending on another implementation.

## Workspace commands

Run commands from the repository root:

```text
npm run doctor
npm run build
npm run test
npm run lint
npm run verify
```

`make doctor|build|test|lint` is an equivalent convenience entry on systems with Make. The root Node script invokes each language tool without a shell so the command semantics remain portable.

T008 intentionally has no external package dependency and does not pin the final toolchain. T009 owns exact Node/pnpm, Rust, Go, Swift/Xcode, Java/Gradle/Kotlin versions, Gradle Wrapper files, platform CI images, and the first reproducible lockfiles.

## Ownership rules

- Only Integration changes root Workspace manifests and lockfiles.
- Only Contracts changes `proto/` and generated SDKs.
- Feature work declares dependencies in its own manifest and calls out required root integration in the handoff.
- Do not hand-edit generated sources or copy another worktree's uncommitted output.

Finish with `docs/templates/agent-handoff.md`, including exact commands, results, contract changes, security impact, risks, and issues unblocked.

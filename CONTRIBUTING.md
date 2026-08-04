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
npm run doctor:workspace
npm run doctor:desktop
npm run doctor:apple
npm run doctor:android
npm run build
npm run test
npm run lint
npm run verify
npm run toolchain:verify
```

`make doctor|build|test|lint|verify` is an equivalent convenience entry on systems with Make. The root Node script invokes each language tool without a shell so the command semantics remain portable.

T009 freezes every toolchain in `toolchains.json` and its native version file. Run `npm run toolchain:verify` after changing any version, lockfile, Gradle Wrapper, platform runner, or GitHub Action. Upgrades must update all repeated pins and checksums in one Integration-owned change; never replace a pin with `latest`, a moving major tag, `^`, or `~`.

Platform setup and unsigned build commands are documented in `docs/build/reproducible-builds.md`. Research sources and compatibility reasoning live in `docs/build/toolchain-research.md`.

## Ownership rules

- Only Integration changes root Workspace manifests and lockfiles.
- Only Contracts changes `proto/` and generated SDKs.
- Feature work declares dependencies in its own manifest and calls out required root integration in the handoff.
- Do not hand-edit generated sources or copy another worktree's uncommitted output.

Finish with `docs/templates/agent-handoff.md`, including exact commands, results, contract changes, security impact, risks, and issues unblocked.

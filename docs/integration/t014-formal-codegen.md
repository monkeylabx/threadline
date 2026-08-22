# T014 authenticated formal codegen handoff

## Scope

Issue #66 owns the two-stage protected-runner workflow in `.github/workflows/proto-formal.yml`. It prepares and verifies the exact T014 PR #44 commit without installing generated SDKs into the repository. It does not run physical devices; Issue #41 remains `NOT RUN`.

## Trust boundary

The `prepare` dispatch must target the exact current PR #44 head. It downloads pinned upstream artifacts, builds source-only generators on the standard `macos-26` runner, calls the T014 schema-5 bundle packager, and uploads an immutable candidate artifact. That run does not claim formal PASS.

An Integration reviewer downloads the candidate, checks `manifest.json`, source URLs/digests, provenance, runner image, and target SHA, then supplies the separately reviewed manifest SHA-256 to a `verify` dispatch together with the prepare run ID. The verify job downloads that same artifact, checks both identities, authenticates Xcode/Gatekeeper again, and runs `verify-codegen.mjs --mode=verify-only` with the bundle's absolute Node executable and an empty environment.

## Evidence

The successful verify artifact records the exact target SHA, prepare and verify run IDs, runner image version, reviewed manifest SHA-256, output file counts/tree digests, and `physicalDevices: NOT RUN`. Workflow permissions are read-only, no repository-mode generation occurs, and no credentials or production data enter the artifact.

## Operator sequence

1. Dispatch `prepare` with the exact PR #44 head SHA.
2. Download `proto-formal-bundle`, verify its archive checksum, and review `manifest.json` plus `prepare-evidence.json`.
3. Dispatch `verify` with the same target SHA, the prepare run ID, and the reviewed manifest SHA-256.
4. Retain `proto-formal-verification` with the T014 handoff; only then may #28 leave Draft HOLD.

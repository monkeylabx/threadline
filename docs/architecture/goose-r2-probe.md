# R2: pinned Goose on macOS

Issue: [#258](https://github.com/monkeylabx/threadline/issues/258). Evidence date: 2026-10-20.
This is an opt-in synthetic experiment, not production worker admission.

## Result

Host: macOS 26.4, Darwin 25.4.0, arm64. Official Goose 1.52.0, Python 3.9.6,
Rust 1.97.1. The checked-in [receipt](goose-r2-macos-receipt.json) records this run.
The [pinned-source audit](goose-r2-source-audit.md) explains configuration and protocol behavior.

| Check | Observed result |
| --- | --- |
| Restricted session | `approve`; extension results empty; all model requests advertised zero tools. |
| Forced `shell` call | Rejected as unadvertised; no command marker created. |
| Sandbox controls | Local fixture HTTP succeeded. Outside-workspace canary read/write and non-loopback connect failed with permission denied. |
| Foreground ACP cancel | Child observed live before cancellation, absent afterward; completion marker absent after five seconds. |
| Background ACP cancel | Child remained live after `cancelled`; completion marker appeared within five seconds. The #210 leak reproduces. |
| Background owner stop | After `cancelled`, owner killed the retained worker's process group and waited for root exit (`-9`). Known fixture child was absent and its completion marker remained absent after five seconds. |
| Current strict agentd API | Initialize/session creation succeeded; text turn returned `UnexpectedMessage` on Goose setup updates. Real Goose compatibility remains incomplete. |

**Conclusion:** this configuration can omit builtin tools and reject a forced
unadvertised call. ACP cancellation alone does not clean up background work.
The owner-stop experiment covers an ordinary inherited process group only.
It does not establish `SealedAndEmpty`, detached/`setsid` containment, descendant
reaping, crash cleanup or protection against a hostile worker. PID observations
are corroboration for known fixtures, never recovery identities or global proof.
The [containment profile](worker-containment-profile.md) remains unsatisfied on macOS.
No Core terminal result, Connector access, real credential or user document was used.

## Reproduce

Run from the repository root on macOS arm64, with Cargo and Python 3 available.
The local port must be unused. No global Goose installation/config change is needed.
Use an environment that permits creating the nested Seatbelt sandbox; failure aborts
without an unrestricted fallback. Normal CI does not download or run Goose.

```sh
set -eu
probe_artifacts=$(mktemp -d /private/tmp/threadline-goose-r2.XXXXXX)
curl --fail --location --output "$probe_artifacts/goose.tar.gz" \
  https://github.com/aaif-goose/goose/releases/download/v1.52.0/goose-aarch64-apple-darwin.tar.gz
printf '%s  %s\n' 7674b0124aab685c71f8782fb7e65bac100c736ce3de0c9d3bf46ba07910e412 \
  "$probe_artifacts/goose.tar.gz" | shasum -a 256 -c -
tar -xzf "$probe_artifacts/goose.tar.gz" -C "$probe_artifacts" ./goose
cat > "$probe_artifacts/routing.json" <<'JSON'
{"synthetic":true,"endpoint":"http://127.0.0.1:48991","credential":"synthetic-probe-only","model":"threadline-synthetic-fixture"}
JSON
cargo build -p threadline-agentd --example goose_probe --locked
python3 crates/agentd/test_support/goose_probe.py \
  --binary "$probe_artifacts/goose" --policy "$probe_artifacts/routing.json" \
  --output "$probe_artifacts/result" --agentd-example target/debug/examples/goose_probe
```

Archive pin: `7674b0124aab685c71f8782fb7e65bac100c736ce3de0c9d3bf46ba07910e412`.
Extracted executable pin, checked before launch:
`b7a38f3ff1e2d48b34b6550f84cf4104f3f7d797429e1231d1255d0501d47702`.
Archive identity comes from the [official release API](https://api.github.com/repos/aaif-goose/goose/releases/tags/v1.52.0);
the executable digest was measured from that verified archive. Signing/notarization was not assessed.

The routing file supplies endpoint, model and fake credential; non-loopback origins
and real credentials are rejected. Each case gets fresh configuration, an empty cwd,
`GOOSE_PATH_ROOT`, cleared inherited environment and explicit loopback proxy bypass.
Eight default extensions are disabled. Developer is enabled in `auto` only for the
separate synthetic cancellation controls, whose commands write task-only markers.
The child profile denies default access: content reads are limited to system runtime
paths, the exact binary and its workspace; writes stay in that workspace plus `/dev/null`.
File metadata, root-directory reads, Mach lookup, process operations and child-only
signals support bootstrap/cancellation. The exact public SSL config is readable for
the curl control. These allowances are not an audited production sandbox.

The Rust example exercises the existing agentd ACP API. A separate bounded test-only
observer records Goose updates for the remaining probes; it rejects permission/tool
callbacks and adds no product protocol capability. Temporary config, synthetic-only
Goose logs and the redacted receipt stay under the chosen output directory.

## Verification and next boundary

Hermetic checks: `python3 -m unittest discover -s crates/agentd/test_support -p test_goose_probe.py -v`.
Rust: `cargo test -p threadline-agentd --all-targets --locked`, Clippy with `-D warnings`, and formatting.
The real probe is a separate explicit integration check. Next work must define the
permitted Goose setup-update contract and implement independently observed macOS
containment before treating a real worker as a trusted production Run participant.

# R2: pinned Goose on macOS

Issue: [#258](https://github.com/monkeylabx/threadline/issues/258). Full rerun: 2026-10-21 after #327.
This opt-in synthetic experiment does not grant production worker admission.
Host: macOS 26.4/Darwin 25.4.0 arm64; Goose 1.52.0, Python 3.9.6, Rust 1.97.1.
Baseline: latest `main` at rerun start, `8245be362c0414fad3a5f28877eb201e60ff3098`, merged into the #258 branch without rewriting delivered commits.
The [receipt](goose-r2-macos-receipt.json) records this run; the [pinned-source audit](goose-r2-source-audit.md) explains configuration and protocol behavior.

| Check | Observed result |
| --- | --- |
| Restricted session | `approve`; extension results empty; all model requests advertised zero tools. |
| Forced `shell` call | Rejected as unadvertised; no command marker created. |
| Sandbox controls | Local fixture HTTP succeeded. Outside-workspace canary read/write and non-loopback connect failed with permission denied. |
| Foreground ACP cancel | Child observed live before cancellation, absent afterward; completion marker absent after five seconds. |
| Background ACP cancel | Child remained live after `cancelled`; completion marker appeared within five seconds. The #210 leak reproduces. |
| Background owner stop | After `cancelled`, owner killed the retained worker's process group and waited for root exit (`-9`). Known fixture child was absent and its completion marker remained absent after five seconds. |
| Strict agentd API | Initialize/session creation succeeded; exact 24-byte `synthetic probe complete` and `end_turn` asserted; zero advertised model tools. |

All five scenarios actually ran. Recording the known background leak lets the investigation finish; it is a cleanup failure, not a containment pass.
Owner stop covers an ordinary inherited process group, without proving `SealedAndEmpty`, detached/`setsid` containment, descendant reaping, crash cleanup or hostile-worker protection.
PID observations corroborate known fixtures; they are never recovery identities or global proof. The [containment profile](worker-containment-profile.md) remains unsatisfied on macOS.
No Core terminal result, Connector access, real credential or user document was used.

## Reproduce

Run from the repository root on macOS arm64 with Cargo, Python 3 and an unused local port.
The environment must permit nested Seatbelt sandbox creation; failure aborts without unrestricted fallback. Normal CI does not download or run Goose.

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
cargo build -p threadline-agentd --example goose_passive_probe --locked
python3 crates/agentd/test_support/goose_probe.py \
  --binary "$probe_artifacts/goose" --policy "$probe_artifacts/routing.json" \
  --output "$probe_artifacts/result" --agentd-example target/debug/examples/goose_passive_probe
```

Extracted executable SHA-256, checked before launch: `b7a38f3ff1e2d48b34b6550f84cf4104f3f7d797429e1231d1255d0501d47702`.
Measured from the verified official archive; signing/notarization was not assessed. The routing file supplies endpoint, model and fake credential.
The isolated configuration/proxy boundary follows the source audit. Child content reads are limited to system runtime paths, the exact binary and workspace; writes stay in the workspace plus `/dev/null`.
Metadata/root-directory reads, Mach lookup, process operations and child-only signals support bootstrap/cancellation; curl may read the exact public SSL config. These allowances are not an audited production sandbox.
The #327 Rust example fails unless exact text and typed `EndTurn` match. The full runner rejects incorrect/missing evidence, nonzero exit, missing endpoint traffic or advertised tools.
A separate bounded test-only observer records restricted-tool/cleanup cases, rejects callbacks and adds no product protocol capability. Synthetic logs/config/markers stay under the output directory.

## Verification and next boundary

Hermetic: `python3 -m unittest discover -s crates/agentd/test_support -p test_goose_probe.py -v` (13 passed).
Rust: `cargo test -p threadline-agentd --all-targets --locked` (98 passed, including the Python wrapper); Clippy with `-D warnings`, formatting and diff checks passed.
The real probe is explicitly separate from hermetic CI. Its receipt records strict text, tool restrictions, shell denial, sandbox controls, foreground cancel and ordinary owner stop passing; ACP-only background cleanup fails.
#327's [bounded passive-update profile](goose-passive-acp-profile.md) resolves the former `UnexpectedMessage` text failure; its narrower allowlist remains in force.
Next work must prove macOS containment before protected Run admission. These results establish neither Core completion nor #259 business-loop acceptance.

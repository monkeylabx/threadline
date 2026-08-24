# Secret scan gate

Status: P00-06A activation validation; branch protection is enabled after this
documentation-only pull request records a green trusted `Secret Scan` check.

Every pull request runs the free, open-source Gitleaks CLI over the exact
base-to-head commit range. The `pull_request_target` workflow executes only the
trusted base commit's workflow, scripts, and configuration. It fetches the
server-owned PR head ref as Git objects, verifies the event's exact head SHA,
and never checks out or executes PR content. Scanning history, rather than only
the checked-out tree, means a credential added in one pull-request commit and
deleted in a later commit still blocks the pull request.

## Trust and installation boundary

The workflow installs Gitleaks `8.30.1` from the official GitHub release asset
`gitleaks_8.30.1_linux_x64.tar.gz`. The installer verifies SHA-256
`551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb`
before extracting the single executable into `RUNNER_TEMP`; it does not change
the runner's global tool directories.

The pull-request workflow has only `contents: read`, checks out the exact base
SHA with `persist-credentials: false`, uses no repository secret or protected
environment, and uploads no finding artifact. Base and head revisions must be
immutable lowercase 40-character commit SHAs present in the repository and
share history. The scan deliberately ignores inline `gitleaks:allow` markers.
Download, checksum, version, range, scanner, and finding failures all fail the
job closed.

The wrapper discards raw scanner output and temporary reports. CI emits only a
stable pass, finding, or scanner-failure category. It never prints the matched
secret, file content, scanner diagnostics, or a reusable credential.

## Local verification

The pinned installer targets the Linux x64 GitHub Actions runner. On Linux x64,
use a disposable directory and remove it after the run:

```sh
tool_directory="$(mktemp -d)"
bash scripts/security/install-gitleaks.sh "${tool_directory}"
bash -n scripts/security/fetch-pr-head.sh scripts/security/install-gitleaks.sh scripts/security/scan-secrets.sh scripts/security/secret-scan.test.sh
bash scripts/security/secret-scan.test.sh "${tool_directory}/gitleaks"
base_sha="$(git merge-base HEAD origin/main)"
head_sha="$(git rev-parse HEAD)"
bash scripts/security/scan-secrets.sh \
  "${tool_directory}/gitleaks" "${PWD}" "${base_sha}" "${head_sha}"
```

On macOS or another architecture, supply a separately downloaded and verified
Gitleaks `8.30.1` executable to the test and scan wrappers. The repository
installer intentionally refuses to install a different release asset.

The isolated test assembles its synthetic credential only at runtime. It proves
a safe repository passes, a finding fails without exposing the complete
canary, malformed or unavailable ranges fail, and a secret committed and then
removed remains detectable in an exact commit range.

Because GitHub loads a `pull_request_target` workflow only from the default
branch, the PR that first adds this workflow is a bootstrap change and cannot
produce its own trusted Secret Scan check. After bootstrap merges, a separate
activation PR must record a green trusted run before `Secret Scan` is added to
the protected branch's required checks.

## Remediation

Treat every finding as potentially compromised. Do not paste it into an Issue,
pull-request comment, chat, log, or artifact. Stop distributing the branch,
notify the Security Owner through the approved private channel, revoke or
rotate the credential at its authority, then remove it from every affected
commit before requesting another scan. Closing the finding without revocation
is not remediation.

False positives require a narrow reviewed `.gitleaks.toml` rule or allowlist
that identifies the exact non-secret shape. Document the reason in the pull
request. Blanket directory, extension, entropy, default-rule, or test-tree
disables are forbidden. Never commit a complete credential as a scanner
fixture.

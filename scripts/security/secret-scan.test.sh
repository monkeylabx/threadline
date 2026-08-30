#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 || ! -x "${1}" ]]; then
  echo "secret scan tests: one executable Gitleaks path is required" >&2
  exit 2
fi

readonly gitleaks="${1}"
readonly script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
readonly scan="${script_directory}/scan-secrets.sh"
readonly fetch_head="${script_directory}/fetch-pr-head.sh"
readonly temporary_directory="$(mktemp -d)"
trap 'rm -rf "${temporary_directory}"' EXIT
umask 077

initialize_repository() {
  local repository="${1}"
  mkdir -p "${repository}"
  git -C "${repository}" init --quiet
  git -C "${repository}" config user.email "secret-scan-test@invalid.example"
  git -C "${repository}" config user.name "Secret Scan Test"
}

commit_all() {
  local repository="${1}"
  local message="${2}"
  git -C "${repository}" add --all
  git -C "${repository}" commit --quiet --message "${message}"
}

run_expect_failure() {
  local output_path="${1}"
  shift
  set +e
  "$@" >"${output_path}" 2>&1
  local status=$?
  set -e
  if [[ ${status} -eq 0 ]]; then
    echo "secret scan tests: expected failure was reported as success" >&2
    exit 1
  fi
}

assert_output_contains() {
  local output_path="${1}"
  local expected="${2}"
  if ! grep -F --quiet "${expected}" "${output_path}"; then
    echo "secret scan tests: expected stable outcome was absent" >&2
    exit 1
  fi
}

assert_finding_output() {
  local output_path="${1}"
  if ! grep -F --quiet \
    "secret scan: finding detected; inspect locally with approved handling" \
    "${output_path}"; then
    echo "secret scan tests: expected finding was an operational failure" >&2
    exit 1
  fi
}

readonly safe_repository="${temporary_directory}/safe"
initialize_repository "${safe_repository}"
printf '%s\n' "ordinary fixture content" >"${safe_repository}/fixture.txt"
commit_all "${safe_repository}" "safe fixture"
"${scan}" "${gitleaks}" "${safe_repository}" >/dev/null

sqlite_api_prefix="sqlite3_api"
sqlite_member_operator="->"
sqlite_column_member="column_bytes16"
sqlite_soft_limit_member="soft_heap_limit64"
sqlite_hard_limit_member="hard_heap_limit64"
sqlite_key_name="iKey"
sqlite_averages_comparison="==FTS5_AVERAGES_ROWID"

readonly sqlite_allowlist_repository="${temporary_directory}/sqlite-allowlist"
initialize_repository "${sqlite_allowlist_repository}"
mkdir -p "${sqlite_allowlist_repository}/third_party/libsqlite3-sys/sqlcipher"
printf '#define sqlite3_column_bytes16 %s%s%s\n#define sqlite3_soft_heap_limit64 %s%s%s\n#define sqlite3_hard_heap_limit64 %s%s%s\nif( %s%s ){}\n' \
  "${sqlite_api_prefix}" "${sqlite_member_operator}" "${sqlite_column_member}" \
  "${sqlite_api_prefix}" "${sqlite_member_operator}" "${sqlite_soft_limit_member}" \
  "${sqlite_api_prefix}" "${sqlite_member_operator}" "${sqlite_hard_limit_member}" \
  "${sqlite_key_name}" "${sqlite_averages_comparison}" \
  >"${sqlite_allowlist_repository}/third_party/libsqlite3-sys/sqlcipher/sqlite3.c"
commit_all "${sqlite_allowlist_repository}" "audited SQLite identifiers"
"${scan}" "${gitleaks}" "${sqlite_allowlist_repository}" >/dev/null

readonly sqlite_near_miss_repository="${temporary_directory}/sqlite-near-miss"
initialize_repository "${sqlite_near_miss_repository}"
printf '#define sqlite3_column_bytes16 %s%s%s\n' \
  "${sqlite_api_prefix}" "${sqlite_member_operator}" "${sqlite_column_member}" \
  >"${sqlite_near_miss_repository}/fixture.c"
commit_all "${sqlite_near_miss_repository}" "SQLite identifier outside audited path"
readonly sqlite_near_miss_output="${temporary_directory}/sqlite-near-miss-output"
run_expect_failure "${sqlite_near_miss_output}" \
  "${scan}" "${gitleaks}" "${sqlite_near_miss_repository}"
assert_finding_output "${sqlite_near_miss_output}"

readonly finding_repository="${temporary_directory}/finding"
initialize_repository "${finding_repository}"
credential_prefix="TL_TEST_"
credential_middle="SECRET_"
credential_suffix="$(printf 'A%.0s' {1..32})"
credential="${credential_prefix}${credential_middle}${credential_suffix}"
printf '%s\n' "${credential}" >"${finding_repository}/fixture.txt"
commit_all "${finding_repository}" "runtime-assembled finding"

readonly finding_output="${temporary_directory}/finding-output"
run_expect_failure "${finding_output}" \
  "${scan}" "${gitleaks}" "${finding_repository}"
assert_finding_output "${finding_output}"
if grep -F --quiet "${credential}" "${finding_output}"; then
  echo "secret scan tests: finding output exposed the complete canary" >&2
  exit 1
fi

readonly direct_stdout="${temporary_directory}/direct-stdout"
readonly direct_stderr="${temporary_directory}/direct-stderr"
readonly direct_report="${temporary_directory}/direct-report.json"
set +e
"${gitleaks}" git \
  --verbose \
  --no-banner \
  --redact=100 \
  --ignore-gitleaks-allow \
  --exit-code=2 \
  --config "${script_directory}/../../.gitleaks.toml" \
  --report-format json \
  --report-path "${direct_report}" \
  "${finding_repository}" \
  >"${direct_stdout}" 2>"${direct_stderr}"
direct_status=$?
set -e
if [[ ${direct_status} -ne 2 ]]; then
  echo "secret scan tests: direct finding did not return the dedicated status" >&2
  exit 1
fi
if ! grep -F --quiet "REDACTED" "${direct_stdout}" "${direct_stderr}" "${direct_report}"; then
  echo "secret scan tests: direct finding did not contain redaction evidence" >&2
  exit 1
fi
if grep -F --quiet "${credential}" "${direct_stdout}" "${direct_stderr}" "${direct_report}"; then
  echo "secret scan tests: direct scanner output exposed the complete canary" >&2
  exit 1
fi

readonly range_repository="${temporary_directory}/range"
initialize_repository "${range_repository}"
printf '%s\n' "range fixture" >"${range_repository}/fixture.txt"
commit_all "${range_repository}" "range base"
base_sha="$(git -C "${range_repository}" rev-parse HEAD)"
printf '%s\n' "${credential}" >"${range_repository}/fixture.txt"
commit_all "${range_repository}" "add runtime-assembled finding"
printf '%s\n' "range fixture restored" >"${range_repository}/fixture.txt"
commit_all "${range_repository}" "remove runtime-assembled finding"
head_sha="$(git -C "${range_repository}" rev-parse HEAD)"

readonly range_output="${temporary_directory}/range-output"
run_expect_failure "${range_output}" \
  "${scan}" "${gitleaks}" "${range_repository}" "${base_sha}" "${head_sha}"
assert_finding_output "${range_output}"
if grep -F --quiet "${credential}" "${range_output}"; then
  echo "secret scan tests: range output exposed the complete canary" >&2
  exit 1
fi

git -C "${range_repository}" checkout --quiet --detach "${base_sha}"
printf '%s\n' "base branch advanced independently" >"${range_repository}/base-update.txt"
commit_all "${range_repository}" "advance base independently"
advanced_base_sha="$(git -C "${range_repository}" rev-parse HEAD)"
run_expect_failure "${range_output}" \
  "${scan}" "${gitleaks}" "${range_repository}" "${advanced_base_sha}" "${head_sha}"
assert_finding_output "${range_output}"

readonly malformed_output="${temporary_directory}/malformed-output"
run_expect_failure "${malformed_output}" \
  "${scan}" "${gitleaks}" "${range_repository}" "${base_sha%?}" "${head_sha}"
run_expect_failure "${malformed_output}" \
  "${scan}" "${gitleaks}" "${range_repository}" "$(printf 'f%.0s' {1..40})" "${head_sha}"

readonly unrelated_repository="${temporary_directory}/unrelated"
initialize_repository "${unrelated_repository}"
printf '%s\n' "unrelated fixture" >"${unrelated_repository}/fixture.txt"
commit_all "${unrelated_repository}" "unrelated commit"
unrelated_sha="$(git -C "${unrelated_repository}" rev-parse HEAD)"
git -C "${range_repository}" fetch --quiet "${unrelated_repository}" "${unrelated_sha}"
run_expect_failure "${malformed_output}" \
  "${scan}" "${gitleaks}" "${range_repository}" "${unrelated_sha}" "${head_sha}"

readonly fetch_remote="${temporary_directory}/fetch-remote.git"
readonly fetch_producer="${temporary_directory}/fetch-producer"
readonly fetch_consumer="${temporary_directory}/fetch-consumer"
git init --bare --quiet "${fetch_remote}"
initialize_repository "${fetch_producer}"
printf '%s\n' "trusted base" >"${fetch_producer}/fixture.txt"
commit_all "${fetch_producer}" "fetch base"
fetch_base_sha="$(git -C "${fetch_producer}" rev-parse HEAD)"
git -C "${fetch_producer}" remote add origin "${fetch_remote}"
git -C "${fetch_producer}" push --quiet origin "HEAD:refs/heads/main"
printf '%s\n' "untrusted head data" >"${fetch_producer}/fixture.txt"
commit_all "${fetch_producer}" "fetch head"
fetch_head_sha="$(git -C "${fetch_producer}" rev-parse HEAD)"
git -C "${fetch_producer}" push --quiet origin "HEAD:refs/pull/7/head"
git clone --quiet "${fetch_remote}" "${fetch_consumer}"
git -C "${fetch_consumer}" checkout --quiet "${fetch_base_sha}"
"${fetch_head}" "${fetch_consumer}" 7 "${fetch_head_sha}" >/dev/null
if [[ "$(git -C "${fetch_consumer}" rev-parse refs/threadline/secret-scan/7/head)" != "${fetch_head_sha}" ]]; then
  echo "secret scan tests: fetched head ref did not match" >&2
  exit 1
fi
run_expect_failure "${malformed_output}" \
  "${fetch_head}" "${fetch_consumer}" 7 "$(printf 'f%.0s' {1..40})"

readonly stub_scanner="${temporary_directory}/stub-gitleaks"
readonly stub_marker="${temporary_directory}/stub-marker"
cat >"${stub_scanner}" <<'STUB'
#!/usr/bin/env bash
if [[ "${1:-}" == "version" ]]; then
  printf '%s\n' "${STUB_VERSION:-8.30.1}"
  exit 0
fi
printf '%s\n' "invoked" >>"${STUB_MARKER}"
exit "${STUB_SCAN_STATUS:-0}"
STUB
chmod 0700 "${stub_scanner}"

STUB_MARKER="${stub_marker}" STUB_SCAN_STATUS=0 \
  "${scan}" "${stub_scanner}" "${safe_repository}" >/dev/null
for operational_status in 1 126 127 137; do
  run_expect_failure "${malformed_output}" env \
    STUB_MARKER="${stub_marker}" STUB_SCAN_STATUS="${operational_status}" \
    "${scan}" "${stub_scanner}" "${safe_repository}"
  assert_output_contains "${malformed_output}" "secret scan: scanner failed"
done
run_expect_failure "${malformed_output}" env \
  STUB_MARKER="${stub_marker}" STUB_SCAN_STATUS=2 \
  "${scan}" "${stub_scanner}" "${safe_repository}"
assert_output_contains "${malformed_output}" "secret scan: finding detected"
run_expect_failure "${malformed_output}" env \
  STUB_MARKER="${stub_marker}" STUB_VERSION=8.30.0 \
  "${scan}" "${stub_scanner}" "${safe_repository}"
assert_output_contains "${malformed_output}" "secret scan: scanner version verification failed"

uppercase_base="$(printf '%s' "${base_sha}" | tr '[:lower:]' '[:upper:]')"
for invalid_base in "" "${base_sha}f" "${uppercase_base}"; do
  rm -f "${stub_marker}"
  run_expect_failure "${malformed_output}" env \
    STUB_MARKER="${stub_marker}" STUB_SCAN_STATUS=0 \
    "${scan}" "${stub_scanner}" "${range_repository}" "${invalid_base}" "${head_sha}"
  if [[ -e "${stub_marker}" ]]; then
    echo "secret scan tests: invalid range reached the scanner" >&2
    exit 1
  fi
done
blob_sha="$(printf '%s' "not a commit" | git -C "${range_repository}" hash-object -w --stdin)"
rm -f "${stub_marker}"
run_expect_failure "${malformed_output}" env \
  STUB_MARKER="${stub_marker}" STUB_SCAN_STATUS=0 \
  "${scan}" "${stub_scanner}" "${range_repository}" "${blob_sha}" "${head_sha}"
if [[ -e "${stub_marker}" ]]; then
  echo "secret scan tests: non-commit range reached the scanner" >&2
  exit 1
fi

unset credential credential_prefix credential_middle credential_suffix
echo "secret scan tests: safe, finding, redaction, and range cases passed"

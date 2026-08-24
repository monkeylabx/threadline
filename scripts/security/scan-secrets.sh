#!/usr/bin/env bash
set -euo pipefail

readonly expected_version="8.30.1"
readonly script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
readonly configuration="${script_directory}/../../.gitleaks.toml"

usage() {
  echo "secret scan: expected GITLEAKS REPOSITORY [BASE_SHA HEAD_SHA]" >&2
}

if [[ $# -ne 2 && $# -ne 4 ]]; then
  usage
  exit 2
fi

readonly gitleaks="${1}"
readonly repository_input="${2}"

if [[ ! -x "${gitleaks}" || ! -f "${configuration}" ]]; then
  echo "secret scan: scanner or configuration is unavailable" >&2
  exit 2
fi

scanner_version="$("${gitleaks}" version 2>/dev/null)" || {
  echo "secret scan: scanner version verification failed" >&2
  exit 1
}
if [[ "${scanner_version}" != "${expected_version}" ]]; then
  echo "secret scan: scanner version verification failed" >&2
  exit 1
fi

repository="$(cd "${repository_input}" 2>/dev/null && pwd -P)" || {
  echo "secret scan: repository is unavailable" >&2
  exit 2
}
if ! git -C "${repository}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "secret scan: repository is unavailable" >&2
  exit 2
fi

range_log_option=""
if [[ $# -eq 4 ]]; then
  readonly base_sha="${3}"
  readonly head_sha="${4}"
  readonly sha_pattern='^[0-9a-f]{40}$'
  if [[ ! "${base_sha}" =~ ${sha_pattern} || ! "${head_sha}" =~ ${sha_pattern} ]]; then
    echo "secret scan: commit range is invalid" >&2
    exit 2
  fi
  if ! git -C "${repository}" cat-file -e "${base_sha}^{commit}" 2>/dev/null ||
    ! git -C "${repository}" cat-file -e "${head_sha}^{commit}" 2>/dev/null; then
    echo "secret scan: commit range is unavailable" >&2
    exit 2
  fi
  if ! git -C "${repository}" merge-base "${base_sha}" "${head_sha}" >/dev/null 2>&1; then
    echo "secret scan: commit range is invalid" >&2
    exit 2
  fi
  range_log_option="--log-opts=${base_sha}..${head_sha}"
fi

readonly temporary_directory="$(mktemp -d)"
trap 'rm -rf "${temporary_directory}"' EXIT
umask 077

scanner_arguments=(
  git
  --no-banner
  --redact=100
  --ignore-gitleaks-allow
  --exit-code=2
  --config "${configuration}"
  --report-format json
  --report-path "${temporary_directory}/report.json"
)
if [[ -n "${range_log_option}" ]]; then
  scanner_arguments+=("${range_log_option}")
fi
scanner_arguments+=("${repository}")

set +e
"${gitleaks}" "${scanner_arguments[@]}" \
  >"${temporary_directory}/stdout" \
  2>"${temporary_directory}/stderr"
scan_status=$?
set -e

case "${scan_status}" in
  0)
    echo "secret scan: no findings"
    ;;
  2)
    echo "secret scan: finding detected; inspect locally with approved handling" >&2
    exit 1
    ;;
  *)
    echo "secret scan: scanner failed" >&2
    exit 1
    ;;
esac

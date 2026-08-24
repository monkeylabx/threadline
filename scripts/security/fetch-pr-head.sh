#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "secret scan fetch: expected REPOSITORY PR_NUMBER HEAD_SHA" >&2
  exit 2
fi

repository="$(cd "${1}" 2>/dev/null && pwd -P)" || {
  echo "secret scan fetch: repository is unavailable" >&2
  exit 2
}
readonly pull_request_number="${2}"
readonly expected_head="${3}"
if [[ ! "${pull_request_number}" =~ ^[1-9][0-9]*$ || ! "${expected_head}" =~ ^[0-9a-f]{40}$ ]]; then
  echo "secret scan fetch: pull request identity is invalid" >&2
  exit 2
fi
if ! git -C "${repository}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "secret scan fetch: repository is unavailable" >&2
  exit 2
fi

readonly source_ref="refs/pull/${pull_request_number}/head"
readonly destination_ref="refs/threadline/secret-scan/${pull_request_number}/head"
if ! git -C "${repository}" fetch --no-tags --force origin "+${source_ref}:${destination_ref}" \
  >/dev/null 2>&1; then
  echo "secret scan fetch: pull request head is unavailable" >&2
  exit 1
fi
actual_head="$(git -C "${repository}" rev-parse --verify "${destination_ref}^{commit}" 2>/dev/null)" || {
  echo "secret scan fetch: pull request head is unavailable" >&2
  exit 1
}
if [[ "${actual_head}" != "${expected_head}" ]]; then
  echo "secret scan fetch: pull request head did not match the event" >&2
  exit 1
fi

echo "secret scan fetch: immutable pull request head verified"

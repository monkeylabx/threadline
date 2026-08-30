#!/usr/bin/env bash
set -euo pipefail

readonly gitleaks_version="8.30.1"
readonly archive_name="gitleaks_${gitleaks_version}_linux_x64.tar.gz"
readonly archive_sha256="551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb"
readonly archive_url="https://github.com/gitleaks/gitleaks/releases/download/v${gitleaks_version}/${archive_name}"

if [[ $# -ne 1 || -z "${1}" ]]; then
  echo "gitleaks installer: one destination directory is required" >&2
  exit 2
fi

case "${1}" in
  /*) ;;
  *)
    echo "gitleaks installer: destination must be absolute" >&2
    exit 2
    ;;
esac

readonly destination="${1}"
if [[ -e "${destination}" || -L "${destination}" ]]; then
  echo "gitleaks installer: destination must not already exist" >&2
  exit 2
fi

readonly temporary_directory="$(mktemp -d)"
trap 'rm -rf "${temporary_directory}"' EXIT
umask 077

curl \
  --proto '=https' \
  --proto-redir '=https' \
  --tlsv1.2 \
  --connect-timeout 15 \
  --max-time 120 \
  --retry 2 \
  --retry-all-errors \
  --fail \
  --location \
  --silent \
  --show-error \
  "${archive_url}" \
  --output "${temporary_directory}/${archive_name}"

printf '%s  %s\n' \
  "${archive_sha256}" \
  "${temporary_directory}/${archive_name}" \
  | sha256sum --check --status

mkdir -p "${temporary_directory}/extract"
tar -xzf "${temporary_directory}/${archive_name}" \
  -C "${temporary_directory}/extract" \
  gitleaks

if [[ ! -f "${temporary_directory}/extract/gitleaks" ]]; then
  echo "gitleaks installer: verified archive did not contain the scanner" >&2
  exit 1
fi

install -d -m 0700 "${destination}"
install -m 0755 "${temporary_directory}/extract/gitleaks" "${destination}/gitleaks"

installed_version="$("${destination}/gitleaks" version 2>/dev/null)"
if [[ "${installed_version}" != "${gitleaks_version}" ]]; then
  rm -f "${destination}/gitleaks"
  echo "gitleaks installer: installed version verification failed" >&2
  exit 1
fi

echo "gitleaks installer: verified scanner installed"

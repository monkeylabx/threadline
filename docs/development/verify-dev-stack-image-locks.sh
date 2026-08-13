#!/bin/sh

set -eu

usage() {
  echo "usage: $0 exact MANIFEST IMAGE... | common MANIFEST_A MANIFEST_B IMAGE..." >&2
  exit 2
}

manifest_images() {
  awk '$1 == "image:" { print $2 }' "$1" | LC_ALL=C sort -u
}

expected_images() {
  printf '%s\n' "$@" | LC_ALL=C sort -u
}

verify_exact() {
  manifest=$1
  shift
  actual=$(manifest_images "$manifest")
  expected=$(expected_images "$@")

  if [ "$actual" != "$expected" ]; then
    echo "image lock mismatch in $manifest" >&2
    echo "expected:" >&2
    printf '%s\n' "$expected" >&2
    echo "actual:" >&2
    printf '%s\n' "$actual" >&2
    exit 1
  fi

  for image in "$@"; do
    digest=${image##*@sha256:}
    [ "$digest" != "$image" ] && printf '%s\n' "$digest" | grep -Eq '^[0-9a-f]{64}$' || {
      echo "mutable or malformed image lock: $image" >&2
      exit 1
    }
  done
}

verify_common() {
  manifest_a=$1
  manifest_b=$2
  shift 2

  images_a=$(manifest_images "$manifest_a")
  images_b=$(manifest_images "$manifest_b")
  for image in "$@"; do
    printf '%s\n' "$images_a" | grep -Fxq "$image" || {
      echo "$manifest_a does not contain common core image $image" >&2
      exit 1
    }
    printf '%s\n' "$images_b" | grep -Fxq "$image" || {
      echo "$manifest_b does not contain common core image $image" >&2
      exit 1
    }
  done
}

case "${1:-}" in
  exact)
    [ "$#" -ge 3 ] || usage
    shift
    verify_exact "$@"
    ;;
  common)
    [ "$#" -ge 4 ] || usage
    shift
    verify_common "$@"
    ;;
  *) usage ;;
esac

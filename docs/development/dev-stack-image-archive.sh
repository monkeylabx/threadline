#!/bin/sh

set -eu

usage() {
  echo "usage: $0 tag IMAGE... | write-ids FILE IMAGE... | verify-ids FILE IMAGE... | verify-compose-tags FILE IMAGE... | verify-kustomize-tags FILE IMAGE..." >&2
  exit 2
}

tag_name() {
  printf '%s\n' "${1%@*}"
}

case "${1:-}" in
  tag)
    shift
    [ "$#" -gt 0 ] || usage
    for image in "$@"; do
      docker image tag "$image" "$(tag_name "$image")"
    done
    ;;
  write-ids)
    [ "$#" -gt 2 ] || usage
    output=$2
    shift 2
    temporary="${output}.tmp.$$"
    trap 'rm -f "$temporary"' EXIT HUP INT TERM
    : > "$temporary"
    chmod 600 "$temporary"
    for image in "$@"; do
      identifier=$(docker image inspect --format '{{.Id}}' "$image")
      printf '%s %s\n' "$image" "$identifier" >> "$temporary"
    done
    mv "$temporary" "$output"
    trap - EXIT HUP INT TERM
    ;;
  verify-ids)
    [ "$#" -gt 2 ] || usage
    input=$2
    shift 2
    [ -f "$input" ] || { echo "image ID manifest not found: $input" >&2; exit 1; }
    [ "$(wc -l < "$input" | tr -d ' ')" = "$#" ] || {
      echo "image ID manifest entry count mismatch: $input" >&2
      exit 1
    }
    for image in "$@"; do
      matches=$(awk -v image="$image" '$1 == image { count += 1; value = $2 } END { if (count == 1) print value }' "$input")
      [ -n "$matches" ] || { echo "missing or duplicate image ID entry: $image" >&2; exit 1; }
      actual=$(docker image inspect --format '{{.Id}}' "$(tag_name "$image")" 2>/dev/null || true)
      [ "$actual" = "$matches" ] || { echo "offline image ID mismatch: $image" >&2; exit 1; }
    done
    ;;
  verify-compose-tags)
    [ "$#" -gt 2 ] || usage
    input=$2
    shift 2
    actual=$(awk '$1 == "image:" { print $2 }' "$input" | LC_ALL=C sort -u)
    expected=$(printf '%s\n' "$@" | LC_ALL=C sort -u)
    [ "$actual" = "$expected" ] || { echo "Compose offline tag set mismatch: $input" >&2; exit 1; }
    ;;
  verify-kustomize-tags)
    [ "$#" -gt 2 ] || usage
    input=$2
    shift 2
    actual=$(awk '
      $1 == "-" && $2 == "name:" { source = $3; next }
      $1 == "newName:" { name = $2; next }
      $1 == "newTag:" && source != "" && name != "" { print name ":" $2; source = ""; name = "" }
    ' "$input" | LC_ALL=C sort -u)
    expected=$(printf '%s\n' "$@" | LC_ALL=C sort -u)
    [ "$actual" = "$expected" ] || { echo "Kustomize runtime tag set mismatch: $input" >&2; exit 1; }
    ;;
  *) usage ;;
esac

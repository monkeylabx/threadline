#!/bin/sh
set -eu

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
REPO_DIR=$(CDPATH= cd -- "$DB_DIR/.." && pwd)
MIGRATION_DIR="$DB_DIR/migrations"
BASE_SHA=${THREADLINE_MIGRATION_BASE_SHA:-}

fail() {
  printf '%s\n' "migration merge policy failed: $1" >&2
  exit 1
}

manifest=$(mktemp "${TMPDIR:-/tmp}/threadline-migration-policy.XXXXXX")
base_manifest=$(mktemp "${TMPDIR:-/tmp}/threadline-migration-base.XXXXXX")
trap 'rm -f "$manifest" "$base_manifest"' EXIT HUP INT TERM

for migration_path in "$MIGRATION_DIR"/*.sql; do
  migration_name=${migration_path##*/}
  printf '%s\n' "$migration_name" | grep -Eq '^[0-9]{6}_[a-z0-9_]+\.(up|down)\.sql$' ||
    fail "invalid migration filename: $migration_name"
  version=${migration_name%%_*}
  stem=${migration_name%.*.sql}
  direction=${migration_name#"$stem".}
  direction=${direction%.sql}
  printf '%s\t%s\t%s\n' "$version" "$direction" "$stem" >>"$manifest"
done

awk -F '\t' '
  {
    key = $1 FS $2
    if (++seen[key] > 1) {
      printf "duplicate migration version %s for direction %s\n", $1, $2 > "/dev/stderr"
      failed = 1
    }
    stem[$1, $2] = $3
    version[$1] = 1
  }
  END {
    for (item in version) {
      if (!((item, "up") in stem) || !((item, "down") in stem)) {
        printf "migration version %s does not have one up/down pair\n", item > "/dev/stderr"
        failed = 1
      } else if (stem[item, "up"] != stem[item, "down"]) {
        printf "migration version %s has mismatched up/down names\n", item > "/dev/stderr"
        failed = 1
      }
    }
    exit failed
  }
' "$manifest" || fail "migration versions are not unique and paired"

if test -z "$BASE_SHA"; then
  printf '%s\n' "migration merge policy static checks passed"
  exit 0
fi

git -C "$REPO_DIR" cat-file -e "$BASE_SHA^{commit}" 2>/dev/null ||
  fail "base commit is unavailable: $BASE_SHA"
if ! git -C "$REPO_DIR" cat-file -e "$BASE_SHA:db/migrations/atlas.sum" 2>/dev/null; then
  printf '%s\n' "migration merge policy initialized; base has no Atlas integrity manifest"
  exit 0
fi
git -C "$REPO_DIR" ls-tree -r --name-only "$BASE_SHA" -- db/migrations |
  grep -E '^db/migrations/[0-9]{6}_[a-z0-9_]+\.(up|down)\.sql$' >"$base_manifest" || true

base_max=0
while IFS= read -r base_path; do
  test -n "$base_path" || continue
  current_path="$REPO_DIR/$base_path"
  test -f "$current_path" || fail "merged migration was deleted: $base_path"
  base_hash=$(git -C "$REPO_DIR" rev-parse "$BASE_SHA:$base_path")
  current_hash=$(git -C "$REPO_DIR" hash-object "$current_path")
  test "$current_hash" = "$base_hash" || fail "merged migration was modified: $base_path"
  base_name=${base_path##*/}
  base_version=${base_name%%_*}
  base_version_number=$(printf '%s\n' "$base_version" | sed 's/^0*//')
  test -n "$base_version_number" || base_version_number=0
  test "$base_version_number" -le "$base_max" || base_max=$base_version_number
done <"$base_manifest"

while IFS="$(printf '\t')" read -r version direction stem; do
  current_name="$stem.$direction.sql"
  current_path="db/migrations/$current_name"
  if grep -Fxq "$current_path" "$base_manifest"; then
    continue
  fi
  version_number=$(printf '%s\n' "$version" | sed 's/^0*//')
  test -n "$version_number" || version_number=0
  test "$version_number" -gt "$base_max" ||
    fail "new migration $current_name must be newer than base version $(printf '%06d' "$base_max")"
done <"$manifest"

printf '%s\n' "migration merge policy passed against $BASE_SHA"

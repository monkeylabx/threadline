#!/bin/sh
set -eu

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
POLICY="$DB_DIR/tests/migration_merge_policy.sh"
FIXTURE_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/threadline-migration-policy-test.XXXXXX")

cleanup() {
  case "$FIXTURE_ROOT" in
    "${TMPDIR:-/tmp}/threadline-migration-policy-test."*) rm -rf "$FIXTURE_ROOT" ;;
    *) printf '%s\n' "refusing to remove unexpected migration policy fixture" >&2 ;;
  esac
}
trap cleanup EXIT HUP INT TERM

create_fixture() {
  fixture_name=$1
  fixture_dir="$FIXTURE_ROOT/$fixture_name"
  mkdir -p "$fixture_dir/db/migrations" "$fixture_dir/db/tests"
  cp "$POLICY" "$fixture_dir/db/tests/migration_merge_policy.sh"
  printf '%s\n' 'CREATE TABLE synthetic_one (id bigint);' >"$fixture_dir/db/migrations/000001_one.up.sql"
  printf '%s\n' 'DROP TABLE synthetic_one;' >"$fixture_dir/db/migrations/000001_one.down.sql"
  printf '%s\n' 'synthetic-integrity-baseline' >"$fixture_dir/db/migrations/atlas.sum"
  git -C "$fixture_dir" init --quiet
  git -C "$fixture_dir" config user.name threadline-migration-policy-test
  git -C "$fixture_dir" config user.email migration-policy-test@threadline.invalid
  git -C "$fixture_dir" add db
  git -C "$fixture_dir" commit --quiet -m baseline
  git -C "$fixture_dir" rev-parse HEAD
}

positive_base=$(create_fixture positive)
printf '%s\n' 'CREATE TABLE synthetic_two (id bigint);' >"$FIXTURE_ROOT/positive/db/migrations/000002_two.up.sql"
printf '%s\n' 'DROP TABLE synthetic_two;' >"$FIXTURE_ROOT/positive/db/migrations/000002_two.down.sql"
THREADLINE_MIGRATION_BASE_SHA=$positive_base sh "$FIXTURE_ROOT/positive/db/tests/migration_merge_policy.sh" >/dev/null

duplicate_base=$(create_fixture duplicate)
printf '%s\n' 'CREATE TABLE synthetic_two (id bigint);' >"$FIXTURE_ROOT/duplicate/db/migrations/000002_two.up.sql"
printf '%s\n' 'DROP TABLE synthetic_two;' >"$FIXTURE_ROOT/duplicate/db/migrations/000002_two.down.sql"
printf '%s\n' 'CREATE TABLE synthetic_duplicate (id bigint);' >"$FIXTURE_ROOT/duplicate/db/migrations/000002_duplicate.up.sql"
printf '%s\n' 'DROP TABLE synthetic_duplicate;' >"$FIXTURE_ROOT/duplicate/db/migrations/000002_duplicate.down.sql"
if THREADLINE_MIGRATION_BASE_SHA=$duplicate_base sh "$FIXTURE_ROOT/duplicate/db/tests/migration_merge_policy.sh" >/dev/null 2>&1; then
  printf '%s\n' "duplicate migration version unexpectedly passed" >&2
  exit 1
fi

modified_base=$(create_fixture modified)
printf '%s\n' '-- rewritten after merge' >>"$FIXTURE_ROOT/modified/db/migrations/000001_one.up.sql"
if THREADLINE_MIGRATION_BASE_SHA=$modified_base sh "$FIXTURE_ROOT/modified/db/tests/migration_merge_policy.sh" >/dev/null 2>&1; then
  printf '%s\n' "modified merged migration unexpectedly passed" >&2
  exit 1
fi

old_base=$(create_fixture old)
printf '%s\n' 'CREATE TABLE synthetic_zero (id bigint);' >"$FIXTURE_ROOT/old/db/migrations/000000_zero.up.sql"
printf '%s\n' 'DROP TABLE synthetic_zero;' >"$FIXTURE_ROOT/old/db/migrations/000000_zero.down.sql"
if THREADLINE_MIGRATION_BASE_SHA=$old_base sh "$FIXTURE_ROOT/old/db/tests/migration_merge_policy.sh" >/dev/null 2>&1; then
  printf '%s\n' "non-append migration version unexpectedly passed" >&2
  exit 1
fi

unpaired_base=$(create_fixture unpaired)
printf '%s\n' 'CREATE TABLE synthetic_two (id bigint);' >"$FIXTURE_ROOT/unpaired/db/migrations/000002_two.up.sql"
if THREADLINE_MIGRATION_BASE_SHA=$unpaired_base sh "$FIXTURE_ROOT/unpaired/db/tests/migration_merge_policy.sh" >/dev/null 2>&1; then
  printf '%s\n' "unpaired migration unexpectedly passed" >&2
  exit 1
fi

printf '%s\n' "migration merge policy tests passed"

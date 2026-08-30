#!/bin/sh

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$DB_DIR/tests/postgres_harness.sh"

ATLAS=${ATLAS:-atlas}
ATLAS_VERSION=1.3.0

atlas_test_fail() {
  postgres_test_fail "$1"
}

atlas_run() {
  "$ATLAS" migrate "$@" --config "file://$DB_DIR/atlas.hcl" --env threadline
}

command -v "$ATLAS" >/dev/null 2>&1 || atlas_test_fail "missing Atlas Community CLI"
actual_version=$($ATLAS version 2>/dev/null | sed -n '1s/^atlas community version v//p')
test "$actual_version" = "$ATLAS_VERSION" || atlas_test_fail "Atlas Community $ATLAS_VERSION required"

atlas_run validate

if test "${1:-}" = "--static"; then
  printf '%s\n' "Atlas migration integrity passed"
  exit 0
fi

postgres_test_start atlas_migration
psql_test --command='CREATE EXTENSION pgcrypto'

if test -n "${PGPASSWORD:-}"; then
  pgpass_file="$temp_dir/pgpass"
  umask 077
  printf '%s:%s:*:%s:%s\n' "$PGHOST" "$PGPORT" "$PGUSER" "$PGPASSWORD" >"$pgpass_file"
  export PGPASSFILE="$pgpass_file"
fi

THREADLINE_MIGRATION_URL="postgres://$PGUSER@$PGHOST:$PGPORT/$test_db?sslmode=disable"
export THREADLINE_MIGRATION_URL

atlas_run apply

revision_count=$(psql_test --tuples-only --no-align --command='SELECT count(*) FROM atlas_schema_revisions.atlas_schema_revisions')
test "$revision_count" = "10" || atlas_test_fail "migration ledger does not contain all ten revisions"

atlas_run apply
second_revision_count=$(psql_test --tuples-only --no-align --command='SELECT count(*) FROM atlas_schema_revisions.atlas_schema_revisions')
test "$second_revision_count" = "$revision_count" || atlas_test_fail "repeat apply changed the migration ledger"

tampered_dir="$temp_dir/migrations"
mkdir "$tampered_dir"
cp "$DB_DIR"/migrations/*.sql "$DB_DIR/migrations/atlas.sum" "$tampered_dir/"
printf '%s\n' '-- synthetic migration-directory tamper' >>"$tampered_dir/000001_core_foundation.up.sql"
if "$ATLAS" migrate apply \
    --url "$THREADLINE_MIGRATION_URL" \
    --dir "file://$tampered_dir?format=golang-migrate" \
    >"$temp_dir/tamper.out" 2>"$temp_dir/tamper.err"; then
  atlas_test_fail "migration-directory tamper unexpectedly passed"
fi
if ! grep -Eiq 'checksum|hash|modified' "$temp_dir/tamper.err" "$temp_dir/tamper.out"; then
  atlas_test_fail "migration-directory tamper failed for an unexpected reason"
fi

post_tamper_revision_count=$(psql_test --tuples-only --no-align --command='SELECT count(*) FROM atlas_schema_revisions.atlas_schema_revisions')
test "$post_tamper_revision_count" = "$revision_count" || atlas_test_fail "migration-directory tamper changed the migration ledger"

postgres_test_finish "Atlas migration ledger passed"

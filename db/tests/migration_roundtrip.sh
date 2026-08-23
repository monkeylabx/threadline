#!/bin/sh
set -eu

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TESTS_DIR="$DB_DIR/tests"
FOUNDATION_UP="$DB_DIR/migrations/000001_core_foundation.up.sql"
FOUNDATION_DOWN="$DB_DIR/migrations/000001_core_foundation.down.sql"
ORGANIZATION_UP="$DB_DIR/migrations/000002_organization.up.sql"
ORGANIZATION_DOWN="$DB_DIR/migrations/000002_organization.down.sql"
MEMBER_UP="$DB_DIR/migrations/000003_member.up.sql"
MEMBER_DOWN="$DB_DIR/migrations/000003_member.down.sql"
SPACE_UP="$DB_DIR/migrations/000004_space.up.sql"
SPACE_DOWN="$DB_DIR/migrations/000004_space.down.sql"
CHANNEL_DM_UP="$DB_DIR/migrations/000005_channel_dm.up.sql"
CHANNEL_DM_DOWN="$DB_DIR/migrations/000005_channel_dm.down.sql"

. "$TESTS_DIR/postgres_harness.sh"
POSTGRES_TEST_SUITE=migration

verify_static_contract() {
  test -f "$TESTS_DIR/postgres_harness.sh" || postgres_test_fail "missing shared PostgreSQL harness"
  test_db=threadline_migration_test_123
  postgres_test_database_is_safe || postgres_test_fail "shared database deletion guard rejected a safe name"
  test_db=threadline_migration_test_123_unexpected
  if postgres_test_database_is_safe; then
    postgres_test_fail "shared database deletion guard accepted an unsafe name"
  fi

  test -f "$FOUNDATION_UP" || postgres_test_fail "missing 000001 up migration"
  test -f "$FOUNDATION_DOWN" || postgres_test_fail "missing 000001 down migration"
  test -f "$ORGANIZATION_UP" || postgres_test_fail "missing 000002 up migration"
  test -f "$ORGANIZATION_DOWN" || postgres_test_fail "missing 000002 down migration"
  test -f "$MEMBER_UP" || postgres_test_fail "missing 000003 up migration"
  test -f "$MEMBER_DOWN" || postgres_test_fail "missing 000003 down migration"
  test -f "$SPACE_UP" || postgres_test_fail "missing 000004 up migration"
  test -f "$SPACE_DOWN" || postgres_test_fail "missing 000004 down migration"
  test -f "$CHANNEL_DM_UP" || postgres_test_fail "missing 000005 up migration"
  test -f "$CHANNEL_DM_DOWN" || postgres_test_fail "missing 000005 down migration"
  grep -Eq '^CREATE SCHEMA domain;$' "$FOUNDATION_UP" || postgres_test_fail "000001 up must create schema domain"
  grep -Eq '^DROP SCHEMA domain;$' "$FOUNDATION_DOWN" || postgres_test_fail "000001 down must drop schema domain"
  grep -Eq '^CREATE TABLE domain\.organizations \($' "$ORGANIZATION_UP" || postgres_test_fail "000002 up must create domain.organizations"
  grep -Eq '^DROP TABLE domain\.organizations;$' "$ORGANIZATION_DOWN" || postgres_test_fail "000002 down must drop domain.organizations exactly"
  grep -Eq '^CREATE TABLE domain\.members \($' "$MEMBER_UP" || postgres_test_fail "000003 up must create domain.members"
  grep -Eq '^DROP TABLE domain\.members;$' "$MEMBER_DOWN" || postgres_test_fail "000003 down must drop domain.members exactly"
  grep -Eq '^CREATE TABLE domain\.spaces \($' "$SPACE_UP" || postgres_test_fail "000004 up must create domain.spaces"
  grep -Eq '^DROP TABLE domain\.spaces;$' "$SPACE_DOWN" || postgres_test_fail "000004 down must drop domain.spaces exactly"
  grep -Eq '^CREATE TABLE domain\.channels \($' "$CHANNEL_DM_UP" || postgres_test_fail "000005 up must create domain.channels"
  grep -Eq '^CREATE TABLE domain\.direct_messages \($' "$CHANNEL_DM_UP" || postgres_test_fail "000005 up must create domain.direct_messages"
  grep -Eq '^CREATE TABLE domain\.direct_message_participants \($' "$CHANNEL_DM_UP" || postgres_test_fail "000005 up must create domain.direct_message_participants"
  grep -Eq '^CREATE CONSTRAINT TRIGGER direct_messages_participants_sealed_at_commit$' "$CHANNEL_DM_UP" || postgres_test_fail "000005 up must defer participant sealing validation until commit"
  grep -Eq '^CREATE TRIGGER direct_messages_lifecycle_update_guard$' "$CHANNEL_DM_UP" || postgres_test_fail "000005 up must guard immutable Direct Message identity"
  grep -Eq '^  FOR UPDATE;$' "$CHANNEL_DM_UP" || postgres_test_fail "000005 up must serialize participant append with finalization"
  grep -Eq '^DROP TABLE domain\.direct_message_participants;$' "$CHANNEL_DM_DOWN" || postgres_test_fail "000005 down must drop domain.direct_message_participants exactly"
  grep -Eq '^DROP TABLE domain\.direct_messages;$' "$CHANNEL_DM_DOWN" || postgres_test_fail "000005 down must drop domain.direct_messages exactly"
  grep -Eq '^DROP TABLE domain\.channels;$' "$CHANNEL_DM_DOWN" || postgres_test_fail "000005 down must drop domain.channels exactly"
  grep -Eq '^DROP FUNCTION domain\.enforce_direct_message_participants_append_only\(\);$' "$CHANNEL_DM_DOWN" || postgres_test_fail "000005 down must drop participant lifecycle function exactly"
  grep -Eq '^DROP FUNCTION domain\.enforce_direct_message_lifecycle_update\(\);$' "$CHANNEL_DM_DOWN" || postgres_test_fail "000005 down must drop Direct Message lifecycle function exactly"
  for down_migration in "$DB_DIR"/migrations/*.down.sql; do
    if grep -Eiq '(^|[^[:alnum:]_])CASCADE([^[:alnum:]_]|$)' "$down_migration"; then
      postgres_test_fail "down migration must not use CASCADE: $(basename "$down_migration")"
    fi
  done
}

verify_static_contract

if test "${1:-}" = "--static"; then
  printf '%s\n' "migration static contract passed"
  exit 0
fi

postgres_test_start migration

apply_up_migrations() {
  psql_test --file="$FOUNDATION_UP"
  psql_test --file="$ORGANIZATION_UP"
  psql_test --file="$MEMBER_UP"
  psql_test --file="$SPACE_UP"
  psql_test --file="$CHANNEL_DM_UP"
}

apply_down_migrations() {
  psql_test --file="$CHANNEL_DM_DOWN"
  psql_test --file="$SPACE_DOWN"
  psql_test --file="$MEMBER_DOWN"
  psql_test --file="$ORGANIZATION_DOWN"
  psql_test --file="$FOUNDATION_DOWN"
}

schema_count=$(psql_test --tuples-only --no-align --command="SELECT count(*) FROM pg_namespace WHERE nspname = 'domain'")
test "$schema_count" = "0" || postgres_test_fail "disposable database is not clean"

apply_up_migrations
"$PG_DUMP" --schema-only --schema=domain --no-owner --no-privileges "$test_db" >"$temp_dir/first.sql"

apply_down_migrations
schema_count=$(psql_test --tuples-only --no-align --command="SELECT count(*) FROM pg_namespace WHERE nspname = 'domain'")
test "$schema_count" = "0" || postgres_test_fail "down migration did not remove schema domain"

apply_up_migrations
"$PG_DUMP" --schema-only --schema=domain --no-owner --no-privileges "$test_db" >"$temp_dir/second.sql"
cmp -s "$temp_dir/first.sql" "$temp_dir/second.sql" || postgres_test_fail "up migrations produced different schemas"

psql_test --file="$CHANNEL_DM_DOWN"
psql_test --file="$SPACE_DOWN"
psql_test --file="$MEMBER_DOWN"
psql_test --file="$ORGANIZATION_DOWN"
psql_test --command='CREATE TABLE domain.down_guard (marker integer NOT NULL)'
if psql_test --file="$FOUNDATION_DOWN" >"$temp_dir/down.out" 2>"$temp_dir/down.err"; then
  postgres_test_fail "down unexpectedly removed a non-empty schema"
fi
guard_count=$(
  psql_test --tuples-only --no-align \
    --command="SELECT count(*) FROM domain.down_guard"
)
test "$guard_count" = "0" || postgres_test_fail "unexpected guard table contents"
psql_test --command='DROP TABLE domain.down_guard'
psql_test --file="$FOUNDATION_DOWN"

postgres_test_finish "migration round trip passed"

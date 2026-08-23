#!/bin/sh
set -eu

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
FOUNDATION_UP="$DB_DIR/migrations/000001_core_foundation.up.sql"
FOUNDATION_DOWN="$DB_DIR/migrations/000001_core_foundation.down.sql"
ORGANIZATION_UP="$DB_DIR/migrations/000002_organization.up.sql"
ORGANIZATION_DOWN="$DB_DIR/migrations/000002_organization.down.sql"

fail() {
  printf '%s\n' "migration test failed: $1" >&2
  exit 1
}

verify_static_contract() {
  test -f "$FOUNDATION_UP" || fail "missing 000001 up migration"
  test -f "$FOUNDATION_DOWN" || fail "missing 000001 down migration"
  test -f "$ORGANIZATION_UP" || fail "missing 000002 up migration"
  test -f "$ORGANIZATION_DOWN" || fail "missing 000002 down migration"
  grep -Eq '^CREATE SCHEMA domain;$' "$FOUNDATION_UP" || fail "000001 up must create schema domain"
  grep -Eq '^DROP SCHEMA domain;$' "$FOUNDATION_DOWN" || fail "000001 down must drop schema domain"
  grep -Eq '^CREATE TABLE domain\.organizations \($' "$ORGANIZATION_UP" || fail "000002 up must create domain.organizations"
  grep -Eq '^DROP TABLE domain\.organizations;$' "$ORGANIZATION_DOWN" || fail "000002 down must drop domain.organizations exactly"
  for down_migration in "$DB_DIR"/migrations/*.down.sql; do
    if grep -Eiq '(^|[^[:alnum:]_])CASCADE([^[:alnum:]_]|$)' "$down_migration"; then
      fail "down migration must not use CASCADE: $(basename "$down_migration")"
    fi
  done
}

verify_static_contract

if test "${1:-}" = "--static"; then
  printf '%s\n' "migration static contract passed"
  exit 0
fi

resolve_tool() {
  tool_name=$1
  if test -n "${THREADLINE_PG_BIN:-}"; then
    tool_path="$THREADLINE_PG_BIN/$tool_name"
    test -x "$tool_path" || fail "missing PostgreSQL tool: $tool_name"
    printf '%s\n' "$tool_path"
    return
  fi
  command -v "$tool_name" 2>/dev/null || fail "missing PostgreSQL tool: $tool_name"
}

PSQL=$(resolve_tool psql)
CREATEDB=$(resolve_tool createdb)
DROPDB=$(resolve_tool dropdb)
PG_DUMP=$(resolve_tool pg_dump)

: "${PGHOST:=127.0.0.1}"
: "${PGPORT:=5432}"
: "${PGUSER:=threadline_postgres_dev}"
: "${PGDATABASE:=postgres}"
export PGHOST PGPORT PGUSER PGDATABASE

test_db="threadline_migration_test_$$"
case "$test_db" in
  threadline_migration_test_[0-9]*) ;;
  *) fail "unsafe disposable database name" ;;
esac

temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/threadline-migration.XXXXXX")
created=0
cleanup() {
  if test "$created" -eq 1; then
    "$DROPDB" --if-exists --maintenance-db="$PGDATABASE" "$test_db" >/dev/null 2>&1 || true
  fi
  case "$temp_dir" in
    "${TMPDIR:-/tmp}"/threadline-migration.*) rm -rf "$temp_dir" ;;
    *) printf '%s\n' "refusing to remove unexpected temporary path: $temp_dir" >&2 ;;
  esac
}
trap cleanup EXIT HUP INT TERM

server_version=$(
  "$PSQL" --no-psqlrc --tuples-only --no-align --dbname="$PGDATABASE" \
    --command='SHOW server_version'
)
case "$server_version" in
  16.4 | 16.4.*) ;;
  *) fail "PostgreSQL 16.4 required" ;;
esac

"$CREATEDB" --maintenance-db="$PGDATABASE" "$test_db" >/dev/null
created=1

psql_test() {
  "$PSQL" --no-psqlrc --quiet --set=ON_ERROR_STOP=1 --dbname="$test_db" "$@"
}

apply_up_migrations() {
  psql_test --file="$FOUNDATION_UP"
  psql_test --file="$ORGANIZATION_UP"
}

apply_down_migrations() {
  psql_test --file="$ORGANIZATION_DOWN"
  psql_test --file="$FOUNDATION_DOWN"
}

schema_count=$(psql_test --tuples-only --no-align --command="SELECT count(*) FROM pg_namespace WHERE nspname = 'domain'")
test "$schema_count" = "0" || fail "disposable database is not clean"

apply_up_migrations
"$PG_DUMP" --schema-only --schema=domain --no-owner --no-privileges "$test_db" >"$temp_dir/first.sql"

apply_down_migrations
schema_count=$(psql_test --tuples-only --no-align --command="SELECT count(*) FROM pg_namespace WHERE nspname = 'domain'")
test "$schema_count" = "0" || fail "down migration did not remove schema domain"

apply_up_migrations
"$PG_DUMP" --schema-only --schema=domain --no-owner --no-privileges "$test_db" >"$temp_dir/second.sql"
cmp -s "$temp_dir/first.sql" "$temp_dir/second.sql" || fail "up migrations produced different schemas"

psql_test --file="$ORGANIZATION_DOWN"
psql_test --command='CREATE TABLE domain.down_guard (marker integer NOT NULL)'
if psql_test --file="$FOUNDATION_DOWN" >"$temp_dir/down.out" 2>"$temp_dir/down.err"; then
  fail "down unexpectedly removed a non-empty schema"
fi
guard_count=$(
  psql_test --tuples-only --no-align \
    --command="SELECT count(*) FROM domain.down_guard"
)
test "$guard_count" = "0" || fail "unexpected guard table contents"
psql_test --command='DROP TABLE domain.down_guard'
psql_test --file="$FOUNDATION_DOWN"

"$DROPDB" --maintenance-db="$PGDATABASE" "$test_db" >/dev/null
created=0
printf '%s\n' "PostgreSQL $server_version migration round trip passed"

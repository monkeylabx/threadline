#!/bin/sh

# Shared by PostgreSQL persistence tests. Call postgres_test_start before using
# psql_test and postgres_test_finish after the final assertion.

postgres_test_fail() {
  printf '%s\n' "$POSTGRES_TEST_SUITE test failed: $1" >&2
  exit 1
}

postgres_test_resolve_tool() {
  tool_name=$1
  if test -n "${THREADLINE_PG_BIN:-}"; then
    tool_path="$THREADLINE_PG_BIN/$tool_name"
    test -x "$tool_path" || postgres_test_fail "missing PostgreSQL tool: $tool_name"
    printf '%s\n' "$tool_path"
    return
  fi
  command -v "$tool_name" 2>/dev/null || postgres_test_fail "missing PostgreSQL tool: $tool_name"
}

postgres_test_database_is_safe() {
  database_prefix="threadline_${POSTGRES_TEST_SUITE}_test_"
  case "$test_db" in
    "$database_prefix"*) database_suffix=${test_db#"$database_prefix"} ;;
    *) return 1 ;;
  esac
  case "$database_suffix" in
    '' | *[!0-9]*) return 1 ;;
    *) return 0 ;;
  esac
}

postgres_test_cleanup() {
  if test "${created:-0}" -eq 1; then
    if postgres_test_database_is_safe; then
      "$DROPDB" --if-exists --maintenance-db="$PGDATABASE" "$test_db" >/dev/null 2>&1 || true
    else
      printf '%s\n' "refusing to drop unexpected PostgreSQL test database" >&2
    fi
  fi
  if test -n "${temp_dir:-}"; then
    case "$temp_dir" in
      "${TMPDIR:-/tmp}/threadline-${POSTGRES_TEST_SUITE}."*) rm -rf "$temp_dir" ;;
      *) printf '%s\n' "refusing to remove unexpected PostgreSQL test temporary directory" >&2 ;;
    esac
  fi
}

postgres_test_start() {
  POSTGRES_TEST_SUITE=$1
  case "$POSTGRES_TEST_SUITE" in
    organization | member | space | channel_dm | channel_membership | resource_acl | authorization_current | migration) ;;
    *) printf '%s\n' "refusing unexpected PostgreSQL test suite" >&2; exit 1 ;;
  esac

  PSQL=$(postgres_test_resolve_tool psql)
  CREATEDB=$(postgres_test_resolve_tool createdb)
  DROPDB=$(postgres_test_resolve_tool dropdb)
  PG_DUMP=$(postgres_test_resolve_tool pg_dump)

  : "${PGHOST:=127.0.0.1}"
  : "${PGPORT:=5432}"
  : "${PGUSER:=threadline_postgres_dev}"
  : "${PGDATABASE:=postgres}"
  export PGHOST PGPORT PGUSER PGDATABASE

  test_db="threadline_${POSTGRES_TEST_SUITE}_test_$$"
  postgres_test_database_is_safe || postgres_test_fail "unsafe disposable database name"

  temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/threadline-${POSTGRES_TEST_SUITE}.XXXXXX")
  created=0
  trap postgres_test_cleanup EXIT HUP INT TERM

  server_version=$(
    "$PSQL" --no-psqlrc --tuples-only --no-align --dbname="$PGDATABASE" \
      --command='SHOW server_version'
  )
  case "$server_version" in
    16.4 | 16.4.*) ;;
    *) postgres_test_fail "PostgreSQL 16.4 required" ;;
  esac

  "$CREATEDB" --maintenance-db="$PGDATABASE" "$test_db" >/dev/null
  created=1
}

psql_test() {
  "$PSQL" --no-psqlrc --quiet --set=ON_ERROR_STOP=1 --dbname="$test_db" "$@"
}

expect_sql_failure() {
  label=$1
  statement=$2
  if psql_test --command="$statement" >"$temp_dir/failure.out" 2>"$temp_dir/failure.err"; then
    postgres_test_fail "$label unexpectedly succeeded"
  fi
}

expect_sql_failure_matching() {
  label=$1
  statement=$2
  pattern=$3
  expect_sql_failure "$label" "$statement"
  if ! grep -Eiq "$pattern" "$temp_dir/failure.err"; then
    postgres_test_fail "$label failed for an unexpected reason"
  fi
}

expect_sql_lock_timeout() {
  label=$1
  statement=$2
  expect_sql_failure_matching "$label" "
    SET lock_timeout = '250ms';
    $statement
  " 'lock timeout'
}

postgres_test_finish() {
  result=$1
  postgres_test_database_is_safe || postgres_test_fail "unsafe disposable database name"
  "$DROPDB" --maintenance-db="$PGDATABASE" "$test_db" >/dev/null
  created=0
  printf '%s\n' "PostgreSQL $server_version $result"
}

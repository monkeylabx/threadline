#!/bin/sh
set -eu

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
FOUNDATION_UP="$DB_DIR/migrations/000001_core_foundation.up.sql"
ORGANIZATION_UP="$DB_DIR/migrations/000002_organization.up.sql"

fail() {
  printf '%s\n' "organization persistence test failed: $1" >&2
  exit 1
}

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

: "${PGHOST:=127.0.0.1}"
: "${PGPORT:=5432}"
: "${PGUSER:=threadline_postgres_dev}"
: "${PGDATABASE:=postgres}"
export PGHOST PGPORT PGUSER PGDATABASE

test_db="threadline_organization_test_$$"
case "$test_db" in
  threadline_organization_test_[0-9]*) ;;
  *) fail "unsafe disposable database name" ;;
esac

temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/threadline-organization.XXXXXX")
created=0
cleanup() {
  if test "$created" -eq 1; then
    "$DROPDB" --if-exists --maintenance-db="$PGDATABASE" "$test_db" >/dev/null 2>&1 || true
  fi
  case "$temp_dir" in
    "${TMPDIR:-/tmp}"/threadline-organization.*) rm -rf "$temp_dir" ;;
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

expect_sql_failure() {
  label=$1
  statement=$2
  if psql_test --command="$statement" >"$temp_dir/failure.out" 2>"$temp_dir/failure.err"; then
    fail "$label unexpectedly succeeded"
  fi
}

psql_test --file="$FOUNDATION_UP"
psql_test --file="$ORGANIZATION_UP"

psql_test --command="
  INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
  VALUES ('tenant-alpha-synthetic', 'Alpha Synthetic', 1, 'policy-alpha-v1');
"

created_at=$(
  psql_test --tuples-only --no-align \
    --command="SELECT created_at FROM domain.organizations WHERE tenant_id = 'tenant-alpha-synthetic'"
)
test -n "$created_at" || fail "create did not assign creation time"

expect_sql_failure "duplicate Tenant identity" "
  INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
  VALUES ('tenant-alpha-synthetic', 'Duplicate Synthetic', 1, 'policy-duplicate-v1');
"
expect_sql_failure "unspecified lifecycle state" "
  INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
  VALUES ('tenant-unspecified-synthetic', 'Unspecified Synthetic', 0, 'policy-unspecified-v1');
"
expect_sql_failure "unknown lifecycle state" "
  INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
  VALUES ('tenant-unknown-synthetic', 'Unknown Synthetic', 4, 'policy-unknown-v1');
"

wrong_scope_lookup=$(
  psql_test --tuples-only --no-align \
    --command="SELECT count(*) FROM domain.organizations WHERE tenant_id = 'tenant-beta-synthetic'"
)
test "$wrong_scope_lookup" = "0" || fail "wrong Tenant scope lookup returned a row"

wrong_scope_update=$(
  psql_test --tuples-only --no-align --command="
    WITH updated AS (
      UPDATE domain.organizations
      SET state = 2, policy_version = 'policy-beta-v2'
      WHERE tenant_id = 'tenant-beta-synthetic'
      RETURNING tenant_id
    )
    SELECT count(*) FROM updated;
  "
)
test "$wrong_scope_update" = "0" || fail "wrong Tenant scope update changed a row"

updated=$(
  psql_test --tuples-only --no-align --field-separator='|' --command="
    UPDATE domain.organizations
    SET state = 2, policy_version = 'policy-alpha-v2'
    WHERE tenant_id = 'tenant-alpha-synthetic'
    RETURNING tenant_id, display_name, state, policy_version, created_at;
  "
)
test "$updated" = "tenant-alpha-synthetic|Alpha Synthetic|2|policy-alpha-v2|$created_at" || \
  fail "state-policy update mutated identity, display name, or creation time"

row_count=$(
  psql_test --tuples-only --no-align \
    --command="SELECT count(*) FROM domain.organizations"
)
test "$row_count" = "1" || fail "negative cases changed persisted row count"

"$DROPDB" --maintenance-db="$PGDATABASE" "$test_db" >/dev/null
created=0
printf '%s\n' "PostgreSQL $server_version Organization persistence passed"

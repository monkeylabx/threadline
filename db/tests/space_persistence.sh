#!/bin/sh
set -eu

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
FOUNDATION_UP="$DB_DIR/migrations/000001_core_foundation.up.sql"
ORGANIZATION_UP="$DB_DIR/migrations/000002_organization.up.sql"
MEMBER_UP="$DB_DIR/migrations/000003_member.up.sql"
SPACE_UP="$DB_DIR/migrations/000004_space.up.sql"

fail() {
  printf '%s\n' "space persistence test failed: $1" >&2
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

test_db="threadline_space_test_$$"
case "$test_db" in
  threadline_space_test_[0-9]*) ;;
  *) fail "unsafe disposable database name" ;;
esac

temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/threadline-space.XXXXXX")
created=0
cleanup() {
  if test "$created" -eq 1; then
    "$DROPDB" --if-exists --maintenance-db="$PGDATABASE" "$test_db" >/dev/null 2>&1 || true
  fi
  case "$temp_dir" in
    "${TMPDIR:-/tmp}"/threadline-space.*) rm -rf "$temp_dir" ;;
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
psql_test --file="$MEMBER_UP"
psql_test --file="$SPACE_UP"

psql_test --command="
  INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
  VALUES
    ('tenant-space-alpha-synthetic', 'Space Alpha Synthetic', 1, 'policy-space-alpha-v1'),
    ('tenant-space-beta-synthetic', 'Space Beta Synthetic', 1, 'policy-space-beta-v1');

  INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable)
  VALUES
    ('tenant-space-alpha-synthetic', 'space-shared-synthetic', 'Alpha Shared Synthetic', TRUE),
    ('tenant-space-beta-synthetic', 'space-shared-synthetic', 'Beta Shared Synthetic', FALSE);
"

created_at=$(
  psql_test --tuples-only --no-align --command="
    SELECT created_at
    FROM domain.spaces
    WHERE tenant_id = 'tenant-space-alpha-synthetic'
      AND space_id = 'space-shared-synthetic';
  "
)
test -n "$created_at" || fail "create did not assign creation time"

expect_sql_failure "duplicate Space identity" "
  INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable)
  VALUES ('tenant-space-alpha-synthetic', 'space-shared-synthetic', 'Duplicate Synthetic', FALSE);
"
expect_sql_failure "missing Organization FK" "
  INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable)
  VALUES ('tenant-space-missing-synthetic', 'space-missing-synthetic', 'Missing Synthetic', FALSE);
"
expect_sql_failure "blank Space identifier" "
  INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable)
  VALUES ('tenant-space-alpha-synthetic', '', 'Blank Synthetic', FALSE);
"
expect_sql_failure "untrimmed Space identifier" "
  INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable)
  VALUES ('tenant-space-alpha-synthetic', ' space-untrimmed-synthetic ', 'Untrimmed Synthetic', FALSE);
"

exact_scope_count=$(
  psql_test --tuples-only --no-align --command="
    SELECT count(*)
    FROM domain.spaces
    WHERE tenant_id = 'tenant-space-alpha-synthetic'
      AND space_id = 'space-shared-synthetic';
  "
)
test "$exact_scope_count" = "1" || fail "exact Space key did not resolve one row"

missing_key_count=$(
  psql_test --tuples-only --no-align --command="
    SELECT count(*)
    FROM domain.spaces
    WHERE tenant_id = 'tenant-space-alpha-synthetic'
      AND space_id = 'space-missing-synthetic';
  "
)
test "$missing_key_count" = "0" || fail "missing exact Space key returned a row"

missing_update=$(
  psql_test --tuples-only --no-align --command="
    WITH updated AS (
      UPDATE domain.spaces
      SET display_name = 'Missing Updated Synthetic', discoverable = TRUE
      WHERE tenant_id = 'tenant-space-alpha-synthetic'
        AND space_id = 'space-missing-synthetic'
      RETURNING tenant_id
    )
    SELECT count(*) FROM updated;
  "
)
test "$missing_update" = "0" || fail "missing exact Space update changed a row"

updated=$(
  psql_test --tuples-only --no-align --field-separator='|' --command="
    UPDATE domain.spaces
    SET display_name = 'Alpha Renamed Synthetic', discoverable = FALSE
    WHERE tenant_id = 'tenant-space-alpha-synthetic'
      AND space_id = 'space-shared-synthetic'
    RETURNING tenant_id, space_id, display_name, discoverable, created_at;
  "
)
test "$updated" = "tenant-space-alpha-synthetic|space-shared-synthetic|Alpha Renamed Synthetic|f|$created_at" || \
  fail "directory metadata update mutated Space identity or creation time"

beta_unchanged=$(
  psql_test --tuples-only --no-align --field-separator='|' --command="
    SELECT display_name, discoverable
    FROM domain.spaces
    WHERE tenant_id = 'tenant-space-beta-synthetic'
      AND space_id = 'space-shared-synthetic';
  "
)
test "$beta_unchanged" = "Beta Shared Synthetic|f" || fail "exact-Tenant update changed another Space row"

row_count=$(psql_test --tuples-only --no-align --command="SELECT count(*) FROM domain.spaces")
test "$row_count" = "2" || fail "negative cases changed persisted Space row count"

"$DROPDB" --maintenance-db="$PGDATABASE" "$test_db" >/dev/null
created=0
printf '%s\n' "PostgreSQL $server_version Space persistence passed"

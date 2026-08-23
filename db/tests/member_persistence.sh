#!/bin/sh
set -eu

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
FOUNDATION_UP="$DB_DIR/migrations/000001_core_foundation.up.sql"
ORGANIZATION_UP="$DB_DIR/migrations/000002_organization.up.sql"
MEMBER_UP="$DB_DIR/migrations/000003_member.up.sql"

fail() {
  printf '%s\n' "member persistence test failed: $1" >&2
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

test_db="threadline_member_test_$$"
case "$test_db" in
  threadline_member_test_[0-9]*) ;;
  *) fail "unsafe disposable database name" ;;
esac

temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/threadline-member.XXXXXX")
created=0
cleanup() {
  if test "$created" -eq 1; then
    "$DROPDB" --if-exists --maintenance-db="$PGDATABASE" "$test_db" >/dev/null 2>&1 || true
  fi
  case "$temp_dir" in
    "${TMPDIR:-/tmp}"/threadline-member.*) rm -rf "$temp_dir" ;;
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

psql_test --command="
  INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
  VALUES
    ('tenant-member-alpha-synthetic', 'Member Alpha Synthetic', 1, 'policy-member-alpha-v1'),
    ('tenant-member-beta-synthetic', 'Member Beta Synthetic', 1, 'policy-member-beta-v1');

  INSERT INTO domain.members (
    tenant_id, actor_type, actor_id, display_name, role, state, org_unit_path
  ) VALUES
    ('tenant-member-alpha-synthetic', 1, 'actor-shared-synthetic', 'Human Alpha Synthetic', 4, 1, 'engineering/synthetic'),
    ('tenant-member-beta-synthetic', 1, 'actor-shared-synthetic', 'Human Beta Synthetic', 5, 2, NULL),
    ('tenant-member-alpha-synthetic', 2, 'actor-agent-synthetic', 'Agent Alpha Synthetic', 4, 2, NULL),
    ('tenant-member-alpha-synthetic', 3, 'actor-service-synthetic', 'Service Alpha Synthetic', 4, 3, NULL);
"

joined_at=$(
  psql_test --tuples-only --no-align --command="
    SELECT joined_at
    FROM domain.members
    WHERE tenant_id = 'tenant-member-alpha-synthetic'
      AND actor_type = 1
      AND actor_id = 'actor-shared-synthetic';
  "
)
test -n "$joined_at" || fail "create did not assign join time"

expect_sql_failure "duplicate Member identity" "
  INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state)
  VALUES ('tenant-member-alpha-synthetic', 1, 'actor-shared-synthetic', 'Duplicate Synthetic', 4, 1);
"
expect_sql_failure "missing Organization FK" "
  INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state)
  VALUES ('tenant-member-missing-synthetic', 1, 'actor-missing-synthetic', 'Missing Synthetic', 4, 1);
"
expect_sql_failure "unspecified ActorType" "
  INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state)
  VALUES ('tenant-member-alpha-synthetic', 0, 'actor-type-zero-synthetic', 'Actor Zero Synthetic', 4, 1);
"
expect_sql_failure "unknown ActorType" "
  INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state)
  VALUES ('tenant-member-alpha-synthetic', 4, 'actor-type-four-synthetic', 'Actor Four Synthetic', 4, 1);
"
expect_sql_failure "unspecified Role" "
  INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state)
  VALUES ('tenant-member-alpha-synthetic', 1, 'actor-role-zero-synthetic', 'Role Zero Synthetic', 0, 1);
"
expect_sql_failure "unknown Role" "
  INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state)
  VALUES ('tenant-member-alpha-synthetic', 1, 'actor-role-six-synthetic', 'Role Six Synthetic', 6, 1);
"
expect_sql_failure "unspecified MemberState" "
  INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state)
  VALUES ('tenant-member-alpha-synthetic', 1, 'actor-state-zero-synthetic', 'State Zero Synthetic', 4, 0);
"
expect_sql_failure "unknown MemberState" "
  INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state)
  VALUES ('tenant-member-alpha-synthetic', 1, 'actor-state-four-synthetic', 'State Four Synthetic', 4, 4);
"

exact_scope_count=$(
  psql_test --tuples-only --no-align --command="
    SELECT count(*)
    FROM domain.members
    WHERE tenant_id = 'tenant-member-alpha-synthetic'
      AND actor_type = 1
      AND actor_id = 'actor-shared-synthetic';
  "
)
test "$exact_scope_count" = "1" || fail "exact Member key did not resolve one row"

missing_key_count=$(
  psql_test --tuples-only --no-align --command="
    SELECT count(*)
    FROM domain.members
    WHERE tenant_id = 'tenant-member-alpha-synthetic'
      AND actor_type = 3
      AND actor_id = 'actor-shared-synthetic';
  "
)
test "$missing_key_count" = "0" || fail "missing exact Member key returned a row"

missing_update=$(
  psql_test --tuples-only --no-align --command="
    WITH updated AS (
      UPDATE domain.members
      SET role = 2, state = 2
      WHERE tenant_id = 'tenant-member-alpha-synthetic'
        AND actor_type = 3
        AND actor_id = 'actor-shared-synthetic'
      RETURNING tenant_id
    )
    SELECT count(*) FROM updated;
  "
)
test "$missing_update" = "0" || fail "missing exact Member update changed a row"

updated=$(
  psql_test --tuples-only --no-align --field-separator='|' --command="
    UPDATE domain.members
    SET role = 2, state = 2
    WHERE tenant_id = 'tenant-member-alpha-synthetic'
      AND actor_type = 1
      AND actor_id = 'actor-shared-synthetic'
    RETURNING tenant_id, actor_type, actor_id, display_name, role, state, org_unit_path, joined_at;
  "
)
test "$updated" = "tenant-member-alpha-synthetic|1|actor-shared-synthetic|Human Alpha Synthetic|2|2|engineering/synthetic|$joined_at" || \
  fail "role-state update mutated Member identity, directory fields, or join time"

null_path_count=$(
  psql_test --tuples-only --no-align --command="
    SELECT count(*) FROM domain.members
    WHERE tenant_id = 'tenant-member-alpha-synthetic'
      AND actor_type IN (2, 3)
      AND org_unit_path IS NULL;
  "
)
test "$null_path_count" = "2" || fail "optional org-unit paths were not preserved as NULL"

beta_unchanged=$(
  psql_test --tuples-only --no-align --field-separator='|' --command="
    SELECT display_name, role, state
    FROM domain.members
    WHERE tenant_id = 'tenant-member-beta-synthetic'
      AND actor_type = 1
      AND actor_id = 'actor-shared-synthetic';
  "
)
test "$beta_unchanged" = "Human Beta Synthetic|5|2" || fail "exact-Tenant update changed another Member row"

row_count=$(psql_test --tuples-only --no-align --command="SELECT count(*) FROM domain.members")
test "$row_count" = "4" || fail "negative cases changed persisted Member row count"

"$DROPDB" --maintenance-db="$PGDATABASE" "$test_db" >/dev/null
created=0
printf '%s\n' "PostgreSQL $server_version Member persistence passed"

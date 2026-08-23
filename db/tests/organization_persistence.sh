#!/bin/sh
set -eu

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TESTS_DIR="$DB_DIR/tests"
FOUNDATION_UP="$DB_DIR/migrations/000001_core_foundation.up.sql"
ORGANIZATION_UP="$DB_DIR/migrations/000002_organization.up.sql"

. "$TESTS_DIR/postgres_harness.sh"
postgres_test_start organization

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
test -n "$created_at" || postgres_test_fail "create did not assign creation time"

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
test "$wrong_scope_lookup" = "0" || postgres_test_fail "wrong Tenant scope lookup returned a row"

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
test "$wrong_scope_update" = "0" || postgres_test_fail "wrong Tenant scope update changed a row"

updated=$(
  psql_test --tuples-only --no-align --field-separator='|' --command="
    UPDATE domain.organizations
    SET state = 2, policy_version = 'policy-alpha-v2'
    WHERE tenant_id = 'tenant-alpha-synthetic'
    RETURNING tenant_id, display_name, state, policy_version, created_at;
  "
)
test "$updated" = "tenant-alpha-synthetic|Alpha Synthetic|2|policy-alpha-v2|$created_at" || \
  postgres_test_fail "state-policy update mutated identity, display name, or creation time"

row_count=$(
  psql_test --tuples-only --no-align \
    --command="SELECT count(*) FROM domain.organizations"
)
test "$row_count" = "1" || postgres_test_fail "negative cases changed persisted row count"

postgres_test_finish "Organization persistence passed"

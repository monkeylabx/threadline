#!/bin/sh
set -eu

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TESTS_DIR="$DB_DIR/tests"
FOUNDATION_UP="$DB_DIR/migrations/000001_core_foundation.up.sql"
ORGANIZATION_UP="$DB_DIR/migrations/000002_organization.up.sql"
MEMBER_UP="$DB_DIR/migrations/000003_member.up.sql"
SPACE_UP="$DB_DIR/migrations/000004_space.up.sql"

. "$TESTS_DIR/postgres_harness.sh"
postgres_test_start space

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
test -n "$created_at" || postgres_test_fail "create did not assign creation time"

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
test "$exact_scope_count" = "1" || postgres_test_fail "exact Space key did not resolve one row"

missing_key_count=$(
  psql_test --tuples-only --no-align --command="
    SELECT count(*)
    FROM domain.spaces
    WHERE tenant_id = 'tenant-space-alpha-synthetic'
      AND space_id = 'space-missing-synthetic';
  "
)
test "$missing_key_count" = "0" || postgres_test_fail "missing exact Space key returned a row"

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
test "$missing_update" = "0" || postgres_test_fail "missing exact Space update changed a row"

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
  postgres_test_fail "directory metadata update mutated Space identity or creation time"

beta_unchanged=$(
  psql_test --tuples-only --no-align --field-separator='|' --command="
    SELECT display_name, discoverable
    FROM domain.spaces
    WHERE tenant_id = 'tenant-space-beta-synthetic'
      AND space_id = 'space-shared-synthetic';
  "
)
test "$beta_unchanged" = "Beta Shared Synthetic|f" || postgres_test_fail "exact-Tenant update changed another Space row"

row_count=$(psql_test --tuples-only --no-align --command="SELECT count(*) FROM domain.spaces")
test "$row_count" = "2" || postgres_test_fail "negative cases changed persisted Space row count"

postgres_test_finish "Space persistence passed"

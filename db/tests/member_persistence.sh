#!/bin/sh
set -eu

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TESTS_DIR="$DB_DIR/tests"
FOUNDATION_UP="$DB_DIR/migrations/000001_core_foundation.up.sql"
ORGANIZATION_UP="$DB_DIR/migrations/000002_organization.up.sql"
MEMBER_UP="$DB_DIR/migrations/000003_member.up.sql"

. "$TESTS_DIR/postgres_harness.sh"
postgres_test_start member

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
test -n "$joined_at" || postgres_test_fail "create did not assign join time"

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
test "$exact_scope_count" = "1" || postgres_test_fail "exact Member key did not resolve one row"

missing_key_count=$(
  psql_test --tuples-only --no-align --command="
    SELECT count(*)
    FROM domain.members
    WHERE tenant_id = 'tenant-member-alpha-synthetic'
      AND actor_type = 3
      AND actor_id = 'actor-shared-synthetic';
  "
)
test "$missing_key_count" = "0" || postgres_test_fail "missing exact Member key returned a row"

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
test "$missing_update" = "0" || postgres_test_fail "missing exact Member update changed a row"

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
  postgres_test_fail "role-state update mutated Member identity, directory fields, or join time"

null_path_count=$(
  psql_test --tuples-only --no-align --command="
    SELECT count(*) FROM domain.members
    WHERE tenant_id = 'tenant-member-alpha-synthetic'
      AND actor_type IN (2, 3)
      AND org_unit_path IS NULL;
  "
)
test "$null_path_count" = "2" || postgres_test_fail "optional org-unit paths were not preserved as NULL"

beta_unchanged=$(
  psql_test --tuples-only --no-align --field-separator='|' --command="
    SELECT display_name, role, state
    FROM domain.members
    WHERE tenant_id = 'tenant-member-beta-synthetic'
      AND actor_type = 1
      AND actor_id = 'actor-shared-synthetic';
  "
)
test "$beta_unchanged" = "Human Beta Synthetic|5|2" || postgres_test_fail "exact-Tenant update changed another Member row"

row_count=$(psql_test --tuples-only --no-align --command="SELECT count(*) FROM domain.members")
test "$row_count" = "4" || postgres_test_fail "negative cases changed persisted Member row count"

postgres_test_finish "Member persistence passed"

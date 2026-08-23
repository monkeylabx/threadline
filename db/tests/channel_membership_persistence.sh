#!/bin/sh
set -eu

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TESTS_DIR="$DB_DIR/tests"
FOUNDATION_UP="$DB_DIR/migrations/000001_core_foundation.up.sql"
ORGANIZATION_UP="$DB_DIR/migrations/000002_organization.up.sql"
MEMBER_UP="$DB_DIR/migrations/000003_member.up.sql"
SPACE_UP="$DB_DIR/migrations/000004_space.up.sql"
CHANNEL_DM_UP="$DB_DIR/migrations/000005_channel_dm.up.sql"
CHANNEL_MEMBERSHIP_UP="$DB_DIR/migrations/000006_channel_membership.up.sql"

. "$TESTS_DIR/postgres_harness.sh"
postgres_test_start channel_membership

psql_test --file="$FOUNDATION_UP"
psql_test --file="$ORGANIZATION_UP"
psql_test --file="$MEMBER_UP"
psql_test --file="$SPACE_UP"
psql_test --file="$CHANNEL_DM_UP"
psql_test --file="$CHANNEL_MEMBERSHIP_UP"

psql_test --command="
  INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
  VALUES
    ('tenant-membership-alpha-synthetic', 'Membership Alpha Synthetic', 1, 'policy-membership-alpha-v1'),
    ('tenant-membership-beta-synthetic', 'Membership Beta Synthetic', 1, 'policy-membership-beta-v1');

  INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state)
  VALUES
    ('tenant-membership-alpha-synthetic', 1, 'actor-alice-synthetic', 'Alice Synthetic', 4, 2),
    ('tenant-membership-alpha-synthetic', 1, 'actor-bob-synthetic', 'Bob Synthetic', 4, 2),
    ('tenant-membership-alpha-synthetic', 1, 'actor-dave-synthetic', 'Dave Synthetic', 4, 2),
    ('tenant-membership-alpha-synthetic', 1, 'actor-invited-synthetic', 'Invited Synthetic', 4, 1),
    ('tenant-membership-alpha-synthetic', 1, 'actor-deactivated-synthetic', 'Deactivated Synthetic', 4, 3),
    ('tenant-membership-beta-synthetic', 1, 'actor-alice-synthetic', 'Beta Alice Synthetic', 4, 2),
    ('tenant-membership-beta-synthetic', 1, 'actor-beta-only-synthetic', 'Beta Only Synthetic', 4, 2);

  INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable)
  VALUES
    ('tenant-membership-alpha-synthetic', 'space-shared-synthetic', 'Alpha Space Synthetic', TRUE),
    ('tenant-membership-beta-synthetic', 'space-shared-synthetic', 'Beta Space Synthetic', FALSE);

  INSERT INTO domain.channels (
    tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id
  ) VALUES
    ('tenant-membership-alpha-synthetic', 'channel-shared-synthetic', 'space-shared-synthetic', 'Alpha Channel Synthetic', 1, 1, 'group-membership-alpha-synthetic'),
    ('tenant-membership-beta-synthetic', 'channel-shared-synthetic', 'space-shared-synthetic', 'Beta Channel Synthetic', 2, 1, 'group-membership-beta-synthetic'),
    ('tenant-membership-beta-synthetic', 'channel-beta-only-synthetic', 'space-shared-synthetic', 'Beta Only Channel Synthetic', 2, 1, 'group-membership-beta-only-synthetic');

  INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
  VALUES
    ('tenant-membership-alpha-synthetic', 'channel-shared-synthetic', 1, 'actor-alice-synthetic', 1),
    ('tenant-membership-beta-synthetic', 'channel-shared-synthetic', 1, 'actor-alice-synthetic', 4);
"

first_interval=$(psql_test --tuples-only --no-align --field-separator='|' --command="
  SELECT interval_id, tenant_id, channel_id, actor_type, actor_id, role, joined_at, left_at IS NULL
  FROM domain.channel_memberships
  WHERE tenant_id = 'tenant-membership-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic'
    AND actor_type = 1
    AND actor_id = 'actor-alice-synthetic'
    AND left_at IS NULL;
")
first_interval_id=${first_interval%%|*}
test "$first_interval" != "" || postgres_test_fail "active Channel Membership did not round trip"
test "$first_interval_id" -gt 0 || postgres_test_fail "Channel Membership interval identity was not server-assigned"
case "$first_interval" in
  "$first_interval_id|tenant-membership-alpha-synthetic|channel-shared-synthetic|1|actor-alice-synthetic|1|"*"|t") ;;
  *) postgres_test_fail "active Channel Membership returned unexpected facts" ;;
esac

expect_sql_failure "unspecified Channel Membership role" "
  INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
  VALUES ('tenant-membership-alpha-synthetic', 'channel-shared-synthetic', 1, 'actor-bob-synthetic', 0);
"
expect_sql_failure "unknown Channel Membership role" "
  INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
  VALUES ('tenant-membership-alpha-synthetic', 'channel-shared-synthetic', 1, 'actor-bob-synthetic', 5);
"
expect_sql_failure "duplicate active Channel Membership" "
  INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
  VALUES ('tenant-membership-alpha-synthetic', 'channel-shared-synthetic', 1, 'actor-alice-synthetic', 1);
"
expect_sql_failure "missing Channel Membership Channel" "
  INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
  VALUES ('tenant-membership-alpha-synthetic', 'channel-missing-synthetic', 1, 'actor-bob-synthetic', 3);
"
expect_sql_failure "cross-Tenant Channel Membership Channel" "
  INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
  VALUES ('tenant-membership-alpha-synthetic', 'channel-beta-only-synthetic', 1, 'actor-bob-synthetic', 3);
"
expect_sql_failure "missing Channel Membership Member" "
  INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
  VALUES ('tenant-membership-alpha-synthetic', 'channel-shared-synthetic', 1, 'actor-missing-synthetic', 3);
"
expect_sql_failure "cross-Tenant Channel Membership Member" "
  INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
  VALUES ('tenant-membership-alpha-synthetic', 'channel-shared-synthetic', 1, 'actor-beta-only-synthetic', 3);
"
expect_sql_failure "invited Channel Membership Member" "
  INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
  VALUES ('tenant-membership-alpha-synthetic', 'channel-shared-synthetic', 1, 'actor-invited-synthetic', 3);
"
expect_sql_failure "deactivated Channel Membership Member" "
  INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
  VALUES ('tenant-membership-alpha-synthetic', 'channel-shared-synthetic', 1, 'actor-deactivated-synthetic', 3);
"
expect_sql_failure "departed Channel Membership insert" "
  INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role, left_at)
  VALUES ('tenant-membership-alpha-synthetic', 'channel-shared-synthetic', 1, 'actor-bob-synthetic', 3, CURRENT_TIMESTAMP);
"

server_joined_year=$(psql_test --tuples-only --no-align --command="
  INSERT INTO domain.channel_memberships (
    tenant_id, channel_id, actor_type, actor_id, role, joined_at
  ) VALUES (
    'tenant-membership-alpha-synthetic', 'channel-shared-synthetic',
    1, 'actor-bob-synthetic', 2, TIMESTAMPTZ '2000-01-01 00:00:00+00'
  ) RETURNING EXTRACT(YEAR FROM joined_at)::integer;
")
test "$server_joined_year" != "2000" || postgres_test_fail "Channel Membership accepted caller-assigned join time"
psql_test --command="
  UPDATE domain.channel_memberships SET left_at = CURRENT_TIMESTAMP
  WHERE tenant_id = 'tenant-membership-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic'
    AND actor_type = 1
    AND actor_id = 'actor-bob-synthetic'
    AND left_at IS NULL;
"

missing_active=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.channel_memberships
  WHERE tenant_id = 'tenant-membership-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic'
    AND actor_type = 1
    AND actor_id = 'actor-missing-synthetic'
    AND left_at IS NULL;
")
test "$missing_active" = "0" || postgres_test_fail "missing exact active Membership key returned a row"

expect_sql_failure "Channel Membership role mutation" "
  UPDATE domain.channel_memberships SET role = 2
  WHERE tenant_id = 'tenant-membership-alpha-synthetic'
    AND interval_id = $first_interval_id;
"
expect_sql_failure "Channel Membership joined-at mutation" "
  UPDATE domain.channel_memberships SET joined_at = joined_at - INTERVAL '1 second'
  WHERE tenant_id = 'tenant-membership-alpha-synthetic'
    AND interval_id = $first_interval_id;
"
expect_sql_failure "active Channel Membership deletion" "
  DELETE FROM domain.channel_memberships
  WHERE tenant_id = 'tenant-membership-alpha-synthetic'
    AND interval_id = $first_interval_id;
"

ended_interval=$(psql_test --tuples-only --no-align --field-separator='|' --command="
  UPDATE domain.channel_memberships
  SET left_at = CURRENT_TIMESTAMP
  WHERE tenant_id = 'tenant-membership-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic'
    AND actor_type = 1
    AND actor_id = 'actor-alice-synthetic'
    AND left_at IS NULL
  RETURNING interval_id, role, joined_at, left_at;
")
first_left_at=$(printf '%s' "$ended_interval" | cut -d '|' -f 4)
test -n "$first_left_at" || postgres_test_fail "ending active Channel Membership did not assign departure time"

double_leave=$(psql_test --tuples-only --no-align --command="
  WITH ended AS (
    UPDATE domain.channel_memberships SET left_at = CURRENT_TIMESTAMP
    WHERE tenant_id = 'tenant-membership-alpha-synthetic'
      AND channel_id = 'channel-shared-synthetic'
      AND actor_type = 1
      AND actor_id = 'actor-alice-synthetic'
      AND left_at IS NULL
    RETURNING interval_id
  ) SELECT count(*) FROM ended;
")
test "$double_leave" = "0" || postgres_test_fail "double leave changed a departed Channel Membership"

expect_sql_failure "Channel Membership reopen" "
  UPDATE domain.channel_memberships SET left_at = NULL
  WHERE tenant_id = 'tenant-membership-alpha-synthetic'
    AND interval_id = $first_interval_id;
"
expect_sql_failure "Channel Membership departure mutation" "
  UPDATE domain.channel_memberships SET left_at = left_at + INTERVAL '1 second'
  WHERE tenant_id = 'tenant-membership-alpha-synthetic'
    AND interval_id = $first_interval_id;
"
expect_sql_failure "departed Channel Membership deletion" "
  DELETE FROM domain.channel_memberships
  WHERE tenant_id = 'tenant-membership-alpha-synthetic'
    AND interval_id = $first_interval_id;
"

rejoined=$(psql_test --tuples-only --no-align --field-separator='|' --command="
  INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
  VALUES ('tenant-membership-alpha-synthetic', 'channel-shared-synthetic', 1, 'actor-alice-synthetic', 3)
  RETURNING interval_id, role, joined_at, left_at IS NULL;
")
second_interval_id=${rejoined%%|*}
test "$second_interval_id" -ne "$first_interval_id" || postgres_test_fail "rejoin reused historical interval identity"

history=$(psql_test --tuples-only --no-align --field-separator='|' --command="
  SELECT interval_id, role, left_at IS NOT NULL
  FROM domain.channel_memberships
  WHERE tenant_id = 'tenant-membership-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic'
    AND actor_type = 1
    AND actor_id = 'actor-alice-synthetic'
  ORDER BY joined_at, interval_id;
")
test "$history" = "$first_interval_id|1|t
$second_interval_id|3|f" || postgres_test_fail "leave-rejoin did not preserve ordered Membership intervals"

preserved_left_at=$(psql_test --tuples-only --no-align --command="
  SELECT left_at FROM domain.channel_memberships
  WHERE tenant_id = 'tenant-membership-alpha-synthetic'
    AND interval_id = $first_interval_id;
")
test "$preserved_left_at" = "$first_left_at" || postgres_test_fail "rejoin changed historical departure time"

expect_sql_failure "failed Channel Membership transaction" "
  BEGIN;
  INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
  VALUES ('tenant-membership-alpha-synthetic', 'channel-shared-synthetic', 1, 'actor-dave-synthetic', 3);
  UPDATE domain.channel_memberships SET role = 2
  WHERE tenant_id = 'tenant-membership-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic'
    AND actor_type = 1
    AND actor_id = 'actor-dave-synthetic';
  COMMIT;
"
failed_rows=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.channel_memberships
  WHERE tenant_id = 'tenant-membership-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic'
    AND actor_type = 1
    AND actor_id = 'actor-dave-synthetic';
")
test "$failed_rows" = "0" || postgres_test_fail "failed Membership transaction left a partial row"

active_members=$(psql_test --tuples-only --no-align --field-separator='|' --command="
  SELECT actor_type, actor_id, role FROM domain.channel_memberships
  WHERE tenant_id = 'tenant-membership-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic'
    AND left_at IS NULL
  ORDER BY actor_type, actor_id;
")
test "$active_members" = "1|actor-alice-synthetic|3" || postgres_test_fail "active Membership list was not exact and deterministic"

postgres_test_finish "Channel Membership persistence passed"

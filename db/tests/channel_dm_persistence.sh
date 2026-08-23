#!/bin/sh
set -eu

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TESTS_DIR="$DB_DIR/tests"
FOUNDATION_UP="$DB_DIR/migrations/000001_core_foundation.up.sql"
ORGANIZATION_UP="$DB_DIR/migrations/000002_organization.up.sql"
MEMBER_UP="$DB_DIR/migrations/000003_member.up.sql"
SPACE_UP="$DB_DIR/migrations/000004_space.up.sql"
CHANNEL_DM_UP="$DB_DIR/migrations/000005_channel_dm.up.sql"

. "$TESTS_DIR/postgres_harness.sh"
postgres_test_start channel_dm

psql_test --file="$FOUNDATION_UP"
psql_test --file="$ORGANIZATION_UP"
psql_test --file="$MEMBER_UP"
psql_test --file="$SPACE_UP"
psql_test --file="$CHANNEL_DM_UP"

psql_test --command="
  INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
  VALUES
    ('tenant-conversation-alpha-synthetic', 'Conversation Alpha Synthetic', 1, 'policy-conversation-alpha-v1'),
    ('tenant-conversation-beta-synthetic', 'Conversation Beta Synthetic', 1, 'policy-conversation-beta-v1');

  INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state)
  VALUES
    ('tenant-conversation-alpha-synthetic', 1, 'actor-alice-synthetic', 'Alice Synthetic', 4, 2),
    ('tenant-conversation-alpha-synthetic', 1, 'actor-bob-synthetic', 'Bob Synthetic', 4, 2),
    ('tenant-conversation-beta-synthetic', 1, 'actor-alice-synthetic', 'Beta Alice Synthetic', 4, 2),
    ('tenant-conversation-beta-synthetic', 1, 'actor-beta-only-synthetic', 'Beta Only Synthetic', 4, 2);

  INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable)
  VALUES
    ('tenant-conversation-alpha-synthetic', 'space-shared-synthetic', 'Alpha Space Synthetic', TRUE),
    ('tenant-conversation-beta-synthetic', 'space-shared-synthetic', 'Beta Space Synthetic', FALSE),
    ('tenant-conversation-beta-synthetic', 'space-beta-only-synthetic', 'Beta Only Space Synthetic', FALSE);

  INSERT INTO domain.channels (
    tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id
  ) VALUES
    ('tenant-conversation-alpha-synthetic', 'channel-shared-synthetic', 'space-shared-synthetic', 'Alpha Channel Synthetic', 1, 1, 'group-channel-alpha-synthetic'),
    ('tenant-conversation-beta-synthetic', 'channel-shared-synthetic', 'space-shared-synthetic', 'Beta Channel Synthetic', 2, 1, 'group-channel-beta-synthetic');

  INSERT INTO domain.direct_messages (tenant_id, dm_id, e2ee_group_id)
  VALUES
    ('tenant-conversation-alpha-synthetic', 'dm-shared-synthetic', 'group-dm-alpha-synthetic'),
    ('tenant-conversation-beta-synthetic', 'dm-shared-synthetic', 'group-dm-beta-synthetic');

  INSERT INTO domain.direct_message_participants (tenant_id, dm_id, actor_type, actor_id)
  VALUES
    ('tenant-conversation-alpha-synthetic', 'dm-shared-synthetic', 1, 'actor-alice-synthetic'),
    ('tenant-conversation-alpha-synthetic', 'dm-shared-synthetic', 1, 'actor-bob-synthetic'),
    ('tenant-conversation-beta-synthetic', 'dm-shared-synthetic', 1, 'actor-alice-synthetic');
"

channel_created_at=$(psql_test --tuples-only --no-align --command="
  SELECT created_at FROM domain.channels
  WHERE tenant_id = 'tenant-conversation-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic';
")
test -n "$channel_created_at" || postgres_test_fail "Channel create did not assign creation time"

dm_created_at=$(psql_test --tuples-only --no-align --command="
  SELECT created_at FROM domain.direct_messages
  WHERE tenant_id = 'tenant-conversation-alpha-synthetic'
    AND dm_id = 'dm-shared-synthetic';
")
test -n "$dm_created_at" || postgres_test_fail "Direct Message create did not assign creation time"

expect_sql_failure "duplicate Channel identity" "
  INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'channel-shared-synthetic', 'space-shared-synthetic', 'Duplicate Channel Synthetic', 1, 1, 'group-channel-duplicate-synthetic');
"
expect_sql_failure "blank Channel identifier" "
  INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', '', 'space-shared-synthetic', 'Blank Channel Synthetic', 1, 1, 'group-channel-blank-synthetic');
"
expect_sql_failure "untrimmed Channel identifier" "
  INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', ' channel-untrimmed-synthetic ', 'space-shared-synthetic', 'Untrimmed Channel Synthetic', 1, 1, 'group-channel-untrimmed-synthetic');
"
expect_sql_failure "blank Channel name" "
  INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'channel-blank-name-synthetic', 'space-shared-synthetic', '', 1, 1, 'group-channel-blank-name-synthetic');
"
expect_sql_failure "untrimmed Channel name" "
  INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'channel-untrimmed-name-synthetic', 'space-shared-synthetic', ' Untrimmed Name Synthetic ', 1, 1, 'group-channel-untrimmed-name-synthetic');
"
expect_sql_failure "unspecified Channel visibility" "
  INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'channel-visibility-zero-synthetic', 'space-shared-synthetic', 'Visibility Zero Synthetic', 0, 1, 'group-channel-visibility-zero-synthetic');
"
expect_sql_failure "unknown Channel visibility" "
  INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'channel-visibility-three-synthetic', 'space-shared-synthetic', 'Visibility Three Synthetic', 3, 1, 'group-channel-visibility-three-synthetic');
"
expect_sql_failure "unspecified Channel state" "
  INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'channel-state-zero-synthetic', 'space-shared-synthetic', 'State Zero Synthetic', 1, 0, 'group-channel-state-zero-synthetic');
"
expect_sql_failure "unknown Channel state" "
  INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'channel-state-four-synthetic', 'space-shared-synthetic', 'State Four Synthetic', 1, 4, 'group-channel-state-four-synthetic');
"
expect_sql_failure "missing Channel Space" "
  INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'channel-missing-space-synthetic', 'space-missing-synthetic', 'Missing Space Synthetic', 1, 1, 'group-channel-missing-space-synthetic');
"
expect_sql_failure "cross-Tenant Channel Space" "
  INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'channel-cross-tenant-space-synthetic', 'space-beta-only-synthetic', 'Cross Tenant Space Synthetic', 1, 1, 'group-channel-cross-tenant-space-synthetic');
"
expect_sql_failure "duplicate Channel E2EE group binding" "
  INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'channel-duplicate-group-synthetic', 'space-shared-synthetic', 'Duplicate Group Synthetic', 1, 1, 'group-channel-alpha-synthetic');
"
expect_sql_failure "blank Channel E2EE group binding" "
  INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'channel-blank-group-synthetic', 'space-shared-synthetic', 'Blank Group Synthetic', 1, 1, '');
"

missing_channel_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.channels
  WHERE tenant_id = 'tenant-conversation-alpha-synthetic'
    AND channel_id = 'channel-missing-synthetic';
")
test "$missing_channel_count" = "0" || postgres_test_fail "missing exact Channel key returned a row"

missing_channel_update=$(psql_test --tuples-only --no-align --command="
  WITH updated AS (
    UPDATE domain.channels SET name = 'Missing Updated Synthetic', state = 2
    WHERE tenant_id = 'tenant-conversation-alpha-synthetic'
      AND channel_id = 'channel-missing-synthetic'
    RETURNING tenant_id
  ) SELECT count(*) FROM updated;
")
test "$missing_channel_update" = "0" || postgres_test_fail "missing exact Channel update changed a row"

updated_channel=$(psql_test --tuples-only --no-align --field-separator='|' --command="
  UPDATE domain.channels SET name = 'Alpha Channel Renamed Synthetic', state = 2
  WHERE tenant_id = 'tenant-conversation-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic'
  RETURNING tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id, created_at;
")
test "$updated_channel" = "tenant-conversation-alpha-synthetic|channel-shared-synthetic|space-shared-synthetic|Alpha Channel Renamed Synthetic|1|2|group-channel-alpha-synthetic|$channel_created_at" || \
  postgres_test_fail "Channel name-state update mutated immutable fields"

beta_channel=$(psql_test --tuples-only --no-align --field-separator='|' --command="
  SELECT name, visibility, state, e2ee_group_id FROM domain.channels
  WHERE tenant_id = 'tenant-conversation-beta-synthetic'
    AND channel_id = 'channel-shared-synthetic';
")
test "$beta_channel" = "Beta Channel Synthetic|2|1|group-channel-beta-synthetic" || \
  postgres_test_fail "exact-Tenant Channel update changed another Tenant"

expect_sql_failure "duplicate Direct Message identity" "
  INSERT INTO domain.direct_messages (tenant_id, dm_id, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'dm-shared-synthetic', 'group-dm-duplicate-synthetic');
"
expect_sql_failure "blank Direct Message identifier" "
  INSERT INTO domain.direct_messages (tenant_id, dm_id, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', '', 'group-dm-blank-synthetic');
"
expect_sql_failure "untrimmed Direct Message identifier" "
  INSERT INTO domain.direct_messages (tenant_id, dm_id, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', ' dm-untrimmed-synthetic ', 'group-dm-untrimmed-synthetic');
"
expect_sql_failure "missing Direct Message Organization" "
  INSERT INTO domain.direct_messages (tenant_id, dm_id, e2ee_group_id)
  VALUES ('tenant-conversation-missing-synthetic', 'dm-missing-tenant-synthetic', 'group-dm-missing-tenant-synthetic');
"
expect_sql_failure "duplicate Direct Message E2EE group binding" "
  INSERT INTO domain.direct_messages (tenant_id, dm_id, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'dm-duplicate-group-synthetic', 'group-dm-alpha-synthetic');
"
expect_sql_failure "untrimmed Direct Message E2EE group binding" "
  INSERT INTO domain.direct_messages (tenant_id, dm_id, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'dm-untrimmed-group-synthetic', ' group-dm-untrimmed-binding-synthetic ');
"
expect_sql_failure "duplicate Direct Message participant" "
  INSERT INTO domain.direct_message_participants (tenant_id, dm_id, actor_type, actor_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'dm-shared-synthetic', 1, 'actor-alice-synthetic');
"
expect_sql_failure "missing Direct Message parent" "
  INSERT INTO domain.direct_message_participants (tenant_id, dm_id, actor_type, actor_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'dm-missing-synthetic', 1, 'actor-alice-synthetic');
"
expect_sql_failure "missing Direct Message Member" "
  INSERT INTO domain.direct_message_participants (tenant_id, dm_id, actor_type, actor_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'dm-shared-synthetic', 1, 'actor-missing-synthetic');
"
expect_sql_failure "cross-Tenant Direct Message Member" "
  INSERT INTO domain.direct_message_participants (tenant_id, dm_id, actor_type, actor_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'dm-shared-synthetic', 1, 'actor-beta-only-synthetic');
"

missing_dm_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.direct_messages
  WHERE tenant_id = 'tenant-conversation-alpha-synthetic'
    AND dm_id = 'dm-missing-synthetic';
")
test "$missing_dm_count" = "0" || postgres_test_fail "missing exact Direct Message key returned a row"

participants=$(psql_test --tuples-only --no-align --field-separator='|' --command="
  SELECT actor_type, actor_id FROM domain.direct_message_participants
  WHERE tenant_id = 'tenant-conversation-alpha-synthetic'
    AND dm_id = 'dm-shared-synthetic'
  ORDER BY actor_type, actor_id;
")
test "$participants" = "1|actor-alice-synthetic
1|actor-bob-synthetic" || postgres_test_fail "Direct Message participant set did not round trip"

expect_sql_failure "failed Direct Message transaction" "
  BEGIN;
  INSERT INTO domain.direct_messages (tenant_id, dm_id, e2ee_group_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'dm-rollback-synthetic', 'group-dm-rollback-synthetic');
  INSERT INTO domain.direct_message_participants (tenant_id, dm_id, actor_type, actor_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'dm-rollback-synthetic', 1, 'actor-alice-synthetic');
  INSERT INTO domain.direct_message_participants (tenant_id, dm_id, actor_type, actor_id)
  VALUES ('tenant-conversation-alpha-synthetic', 'dm-rollback-synthetic', 1, 'actor-missing-synthetic');
  COMMIT;
"
rollback_rows=$(psql_test --tuples-only --no-align --command="
  SELECT
    (SELECT count(*) FROM domain.direct_messages WHERE tenant_id = 'tenant-conversation-alpha-synthetic' AND dm_id = 'dm-rollback-synthetic') +
    (SELECT count(*) FROM domain.direct_message_participants WHERE tenant_id = 'tenant-conversation-alpha-synthetic' AND dm_id = 'dm-rollback-synthetic');
")
test "$rollback_rows" = "0" || postgres_test_fail "failed Direct Message transaction left partial rows"

dm_round_trip=$(psql_test --tuples-only --no-align --field-separator='|' --command="
  SELECT tenant_id, dm_id, e2ee_group_id, created_at FROM domain.direct_messages
  WHERE tenant_id = 'tenant-conversation-alpha-synthetic'
    AND dm_id = 'dm-shared-synthetic';
")
test "$dm_round_trip" = "tenant-conversation-alpha-synthetic|dm-shared-synthetic|group-dm-alpha-synthetic|$dm_created_at" || \
  postgres_test_fail "Direct Message identity, group binding, or creation time changed"

postgres_test_finish "Channel and Direct Message persistence passed"

#!/bin/sh
set -eu

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TESTS_DIR="$DB_DIR/tests"
FOUNDATION_UP="$DB_DIR/migrations/000001_core_foundation.up.sql"
ORGANIZATION_UP="$DB_DIR/migrations/000002_organization.up.sql"
MEMBER_UP="$DB_DIR/migrations/000003_member.up.sql"
SYNC_CURSOR_UP="$DB_DIR/migrations/000012_sync_cursor.up.sql"
SYNC_CURSOR_DOWN="$DB_DIR/migrations/000012_sync_cursor.down.sql"

. "$TESTS_DIR/postgres_harness.sh"

test -f "$SYNC_CURSOR_UP" || postgres_test_fail "missing 000012 up migration"
test -f "$SYNC_CURSOR_DOWN" || postgres_test_fail "missing 000012 down migration"
if grep -Eiq '(^|[^[:alnum:]_])CASCADE([^[:alnum:]_]|$)' "$SYNC_CURSOR_DOWN"; then
  postgres_test_fail "000012 down must not use CASCADE"
fi

postgres_test_start sync_cursor

psql_test --file="$FOUNDATION_UP"
psql_test --file="$ORGANIZATION_UP"
psql_test --file="$MEMBER_UP"

psql_test --command="
  CREATE TABLE domain.sync_cursor_down_sentinel (marker text PRIMARY KEY);
  INSERT INTO domain.sync_cursor_down_sentinel (marker) VALUES ('owned-by-test-synthetic');
  INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version) VALUES
    ('tenant-cursor-alpha-synthetic', 'Cursor Alpha Synthetic', 1, 'policy-cursor-alpha-v1'),
    ('tenant-cursor-beta-synthetic', 'Cursor Beta Synthetic', 1, 'policy-cursor-beta-v1');
  INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state) VALUES
    ('tenant-cursor-alpha-synthetic', 1, 'actor-cursor-owner-synthetic', 'Cursor Owner Synthetic', 4, 2),
    ('tenant-cursor-beta-synthetic', 1, 'actor-cursor-owner-synthetic', 'Beta Cursor Owner Synthetic', 4, 2);
"

psql_test --file="$SYNC_CURSOR_UP"

insert_cursor() {
  psql_test --command="
    INSERT INTO domain.sync_cursors (tenant_id, device_id, conversation_kind, conversation_id, actor_type, actor_id, last_applied_seq)
    VALUES ('$1', '$2', 1, 'channel-cursor-synthetic', 1, 'actor-cursor-owner-synthetic', $3);
  "
}

insert_cursor tenant-cursor-alpha-synthetic device-cursor-laptop-synthetic 3
insert_cursor tenant-cursor-alpha-synthetic device-cursor-phone-synthetic 1
insert_cursor tenant-cursor-beta-synthetic device-cursor-laptop-synthetic 7

expect_sql_failure_matching "duplicate device cursor" "
  INSERT INTO domain.sync_cursors (tenant_id, device_id, conversation_kind, conversation_id, actor_type, actor_id, last_applied_seq)
  VALUES ('tenant-cursor-alpha-synthetic', 'device-cursor-laptop-synthetic', 1, 'channel-cursor-synthetic', 1, 'actor-cursor-owner-synthetic', 4);
" 'sync_cursors_pkey'
expect_sql_failure_matching "non-member owner" "
  INSERT INTO domain.sync_cursors (tenant_id, device_id, conversation_kind, conversation_id, actor_type, actor_id, last_applied_seq)
  VALUES ('tenant-cursor-alpha-synthetic', 'device-cursor-stranger-synthetic', 1, 'channel-cursor-synthetic', 1, 'actor-cursor-stranger-synthetic', 0);
" 'sync_cursors_member_fk'
expect_sql_failure_matching "unknown kind" "
  INSERT INTO domain.sync_cursors (tenant_id, device_id, conversation_kind, conversation_id, actor_type, actor_id, last_applied_seq)
  VALUES ('tenant-cursor-alpha-synthetic', 'device-cursor-kind-synthetic', 3, 'channel-cursor-synthetic', 1, 'actor-cursor-owner-synthetic', 0);
" 'sync_cursors_kind_known'
expect_sql_failure_matching "negative position" "
  INSERT INTO domain.sync_cursors (tenant_id, device_id, conversation_kind, conversation_id, actor_type, actor_id, last_applied_seq)
  VALUES ('tenant-cursor-alpha-synthetic', 'device-cursor-negative-synthetic', 1, 'channel-cursor-synthetic', 1, 'actor-cursor-owner-synthetic', -1);
" 'sync_cursors_last_applied_seq_non_negative'

# Advance, idempotent equal update, then every forbidden change.
psql_test --command="
  UPDATE domain.sync_cursors SET last_applied_seq = 5
  WHERE tenant_id = 'tenant-cursor-alpha-synthetic' AND device_id = 'device-cursor-laptop-synthetic' AND conversation_kind = 1 AND conversation_id = 'channel-cursor-synthetic';
  UPDATE domain.sync_cursors SET last_applied_seq = 5
  WHERE tenant_id = 'tenant-cursor-alpha-synthetic' AND device_id = 'device-cursor-laptop-synthetic' AND conversation_kind = 1 AND conversation_id = 'channel-cursor-synthetic';
"
position=$(psql_test --tuples-only --no-align --command="
  SELECT last_applied_seq FROM domain.sync_cursors
  WHERE tenant_id = 'tenant-cursor-alpha-synthetic' AND device_id = 'device-cursor-laptop-synthetic';
")
test "$position" = "5" || postgres_test_fail "cursor did not advance to 5"
expect_sql_failure_matching "cursor regression" "
  UPDATE domain.sync_cursors SET last_applied_seq = 4
  WHERE tenant_id = 'tenant-cursor-alpha-synthetic' AND device_id = 'device-cursor-laptop-synthetic';
" 'never regress'
expect_sql_failure_matching "cursor identity change" "
  UPDATE domain.sync_cursors SET conversation_id = 'channel-other-synthetic'
  WHERE tenant_id = 'tenant-cursor-alpha-synthetic' AND device_id = 'device-cursor-laptop-synthetic';
" 'identity is immutable'
expect_sql_failure_matching "cursor owner change" "
  UPDATE domain.sync_cursors SET actor_id = 'actor-cursor-stranger-synthetic'
  WHERE tenant_id = 'tenant-cursor-alpha-synthetic' AND device_id = 'device-cursor-laptop-synthetic';
" 'identity is immutable'
expect_sql_failure_matching "cursor delete" "
  DELETE FROM domain.sync_cursors WHERE tenant_id = 'tenant-cursor-alpha-synthetic';
" 'cannot be deleted'

other_positions=$(psql_test --tuples-only --no-align --command="
  SELECT string_agg(tenant_id || ':' || device_id || '=' || last_applied_seq, ',' ORDER BY tenant_id, device_id)
  FROM domain.sync_cursors WHERE device_id <> 'device-cursor-laptop-synthetic' OR tenant_id <> 'tenant-cursor-alpha-synthetic';
")
test "$other_positions" = "tenant-cursor-alpha-synthetic:device-cursor-phone-synthetic=1,tenant-cursor-beta-synthetic:device-cursor-laptop-synthetic=7" || \
  postgres_test_fail "other devices or tenants were affected: $other_positions"

psql_test --file="$SYNC_CURSOR_DOWN"

sentinel_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.sync_cursor_down_sentinel WHERE marker = 'owned-by-test-synthetic';
")
test "$sentinel_count" = "1" || postgres_test_fail "000012 down removed an unrelated table or row"
member_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.members WHERE actor_id = 'actor-cursor-owner-synthetic';
")
test "$member_count" = "2" || postgres_test_fail "000012 down removed prior-migration rows"
owned_count=$(psql_test --tuples-only --no-align --command="
  SELECT (SELECT count(*) FROM information_schema.tables WHERE table_schema = 'domain' AND table_name = 'sync_cursors')
       + (SELECT count(*) FROM pg_proc JOIN pg_namespace ON pg_namespace.oid = pg_proc.pronamespace
          WHERE nspname = 'domain' AND proname = 'enforce_sync_cursor_lifecycle');
")
test "$owned_count" = "0" || postgres_test_fail "000012 down left owned objects behind"

psql_test --file="$SYNC_CURSOR_UP"
table_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM information_schema.tables WHERE table_schema = 'domain' AND table_name = 'sync_cursors';
")
test "$table_count" = "1" || postgres_test_fail "000012 up after down did not restore the table"

postgres_test_finish "sync cursor persistence passed"

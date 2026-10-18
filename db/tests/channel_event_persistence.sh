#!/bin/sh
set -eu

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TESTS_DIR="$DB_DIR/tests"
FOUNDATION_UP="$DB_DIR/migrations/000001_core_foundation.up.sql"
ORGANIZATION_UP="$DB_DIR/migrations/000002_organization.up.sql"
MEMBER_UP="$DB_DIR/migrations/000003_member.up.sql"
SPACE_UP="$DB_DIR/migrations/000004_space.up.sql"
CHANNEL_DM_UP="$DB_DIR/migrations/000005_channel_dm.up.sql"
CHANNEL_EVENT_UP="$DB_DIR/migrations/000011_channel_event.up.sql"
CHANNEL_EVENT_DOWN="$DB_DIR/migrations/000011_channel_event.down.sql"

. "$TESTS_DIR/postgres_harness.sh"

test -f "$CHANNEL_EVENT_UP" || postgres_test_fail "missing 000011 up migration"
test -f "$CHANNEL_EVENT_DOWN" || postgres_test_fail "missing 000011 down migration"
if grep -Eiq '(^|[^[:alnum:]_])CASCADE([^[:alnum:]_]|$)' "$CHANNEL_EVENT_DOWN"; then
  postgres_test_fail "000011 down must not use CASCADE"
fi

postgres_test_start channel_event

psql_test --file="$FOUNDATION_UP"
psql_test --file="$ORGANIZATION_UP"
psql_test --file="$MEMBER_UP"
psql_test --file="$SPACE_UP"
psql_test --file="$CHANNEL_DM_UP"

psql_test --command="
  CREATE TABLE domain.channel_event_down_sentinel (marker text PRIMARY KEY);
  INSERT INTO domain.channel_event_down_sentinel (marker) VALUES ('owned-by-test-synthetic');
  INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version) VALUES
    ('tenant-event-alpha-synthetic', 'Event Alpha Synthetic', 1, 'policy-event-alpha-v1'),
    ('tenant-event-beta-synthetic', 'Event Beta Synthetic', 1, 'policy-event-beta-v1');
  INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state) VALUES
    ('tenant-event-alpha-synthetic', 1, 'actor-event-sender-synthetic', 'Sender Synthetic', 4, 2),
    ('tenant-event-beta-synthetic', 1, 'actor-event-sender-synthetic', 'Beta Sender Synthetic', 4, 2);
  INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable) VALUES
    ('tenant-event-alpha-synthetic', 'space-event-synthetic', 'Event Space Synthetic', TRUE),
    ('tenant-event-beta-synthetic', 'space-event-synthetic', 'Beta Event Space Synthetic', TRUE);
  INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id) VALUES
    ('tenant-event-alpha-synthetic', 'channel-event-synthetic', 'space-event-synthetic', 'Event Channel Synthetic', 2, 1, 'group-event-alpha-synthetic'),
    ('tenant-event-beta-synthetic', 'channel-event-synthetic', 'space-event-synthetic', 'Beta Event Channel Synthetic', 2, 1, 'group-event-beta-synthetic');
"

psql_test --file="$CHANNEL_EVENT_UP"

HASH_HEX=0102030405060708091011121314151617181920212223242526272829303132

insert_event() {
  event_tenant=$1
  event_id=$2
  event_seq=$3
  event_device=$4
  event_key=$5
  psql_test --command="
    INSERT INTO domain.channel_events (
      tenant_id, event_id, channel_id, channel_seq, sender_actor_type, sender_actor_id,
      sender_device_id, idempotency_key, category, envelope, content_hash
    ) VALUES (
      '$event_tenant', '$event_id', 'channel-event-synthetic', $event_seq, 1, 'actor-event-sender-synthetic',
      '$event_device', '$event_key', 1, '\\x6f70617175652d656e76656c6f70652d73796e746865746963'::bytea, decode('$HASH_HEX', 'hex')
    );
  "
}

# Sequence head: insert, advance by exactly one, and reject every other change.
psql_test --command="
  INSERT INTO domain.conversation_sequence_heads (tenant_id, conversation_kind, conversation_id)
  VALUES ('tenant-event-alpha-synthetic', 1, 'channel-event-synthetic');
  UPDATE domain.conversation_sequence_heads SET last_seq = last_seq + 1
  WHERE tenant_id = 'tenant-event-alpha-synthetic' AND conversation_kind = 1 AND conversation_id = 'channel-event-synthetic';
"
head_seq=$(psql_test --tuples-only --no-align --command="
  SELECT last_seq FROM domain.conversation_sequence_heads
  WHERE tenant_id = 'tenant-event-alpha-synthetic' AND conversation_kind = 1 AND conversation_id = 'channel-event-synthetic';
")
test "$head_seq" = "1" || postgres_test_fail "sequence head did not advance to 1"
expect_sql_failure_matching "sequence head skip" "
  UPDATE domain.conversation_sequence_heads SET last_seq = last_seq + 2
  WHERE tenant_id = 'tenant-event-alpha-synthetic' AND conversation_kind = 1 AND conversation_id = 'channel-event-synthetic';
" 'advance by exactly one'
expect_sql_failure_matching "sequence head rewind" "
  UPDATE domain.conversation_sequence_heads SET last_seq = 0
  WHERE tenant_id = 'tenant-event-alpha-synthetic' AND conversation_kind = 1 AND conversation_id = 'channel-event-synthetic';
" 'advance by exactly one'
expect_sql_failure_matching "sequence head identity change" "
  UPDATE domain.conversation_sequence_heads SET conversation_id = 'channel-other-synthetic', last_seq = last_seq + 1
  WHERE tenant_id = 'tenant-event-alpha-synthetic' AND conversation_kind = 1 AND conversation_id = 'channel-event-synthetic';
" 'identity is immutable'
expect_sql_failure_matching "sequence head delete" "
  DELETE FROM domain.conversation_sequence_heads WHERE tenant_id = 'tenant-event-alpha-synthetic';
" 'cannot be deleted'
expect_sql_failure "sequence head unknown kind" "
  INSERT INTO domain.conversation_sequence_heads (tenant_id, conversation_kind, conversation_id)
  VALUES ('tenant-event-alpha-synthetic', 3, 'channel-event-synthetic');
"

# A rolled-back advance leaves the head untouched: no gap can appear.
psql_test --command="
  BEGIN;
  UPDATE domain.conversation_sequence_heads SET last_seq = last_seq + 1
  WHERE tenant_id = 'tenant-event-alpha-synthetic' AND conversation_kind = 1 AND conversation_id = 'channel-event-synthetic';
  ROLLBACK;
"
head_seq=$(psql_test --tuples-only --no-align --command="
  SELECT last_seq FROM domain.conversation_sequence_heads
  WHERE tenant_id = 'tenant-event-alpha-synthetic' AND conversation_kind = 1 AND conversation_id = 'channel-event-synthetic';
")
test "$head_seq" = "1" || postgres_test_fail "rolled-back advance changed the sequence head"

# Channel events: one position per conversation, exactly one conversation, member sender, immutable.
insert_event tenant-event-alpha-synthetic event-alpha-1-synthetic 1 device-alpha-synthetic key-alpha-1-synthetic
insert_event tenant-event-beta-synthetic event-alpha-1-synthetic 1 device-beta-synthetic key-alpha-1-synthetic
expect_sql_failure_matching "duplicate channel sequence" "$(
  printf '%s' "INSERT INTO domain.channel_events (tenant_id, event_id, channel_id, channel_seq, sender_actor_type, sender_actor_id, sender_device_id, idempotency_key, category, envelope, content_hash) VALUES ('tenant-event-alpha-synthetic', 'event-alpha-dup-synthetic', 'channel-event-synthetic', 1, 1, 'actor-event-sender-synthetic', 'device-alpha-synthetic', 'key-alpha-dup-synthetic', 1, '\\x01'::bytea, decode('$HASH_HEX', 'hex'));"
)" 'channel_events_channel_seq_unique'
expect_sql_failure_matching "both conversations null" "
  INSERT INTO domain.channel_events (tenant_id, event_id, channel_seq, sender_actor_type, sender_actor_id, sender_device_id, idempotency_key, category, envelope, content_hash)
  VALUES ('tenant-event-alpha-synthetic', 'event-alpha-none-synthetic', 2, 1, 'actor-event-sender-synthetic', 'device-alpha-synthetic', 'key-alpha-none-synthetic', 1, '\\x01'::bytea, decode('$HASH_HEX', 'hex'));
" 'channel_events_exactly_one_conversation'
expect_sql_failure_matching "both conversations set" "
  INSERT INTO domain.channel_events (tenant_id, event_id, channel_id, dm_id, channel_seq, sender_actor_type, sender_actor_id, sender_device_id, idempotency_key, category, envelope, content_hash)
  VALUES ('tenant-event-alpha-synthetic', 'event-alpha-both-synthetic', 'channel-event-synthetic', 'dm-event-synthetic', 2, 1, 'actor-event-sender-synthetic', 'device-alpha-synthetic', 'key-alpha-both-synthetic', 1, '\\x01'::bytea, decode('$HASH_HEX', 'hex'));
" 'channel_events_exactly_one_conversation'
expect_sql_failure_matching "unknown channel" "
  INSERT INTO domain.channel_events (tenant_id, event_id, channel_id, channel_seq, sender_actor_type, sender_actor_id, sender_device_id, idempotency_key, category, envelope, content_hash)
  VALUES ('tenant-event-alpha-synthetic', 'event-alpha-missing-synthetic', 'channel-missing-synthetic', 2, 1, 'actor-event-sender-synthetic', 'device-alpha-synthetic', 'key-alpha-missing-synthetic', 1, '\\x01'::bytea, decode('$HASH_HEX', 'hex'));
" 'channel_events_channel_fk'
expect_sql_failure_matching "non-member sender" "
  INSERT INTO domain.channel_events (tenant_id, event_id, channel_id, channel_seq, sender_actor_type, sender_actor_id, sender_device_id, idempotency_key, category, envelope, content_hash)
  VALUES ('tenant-event-alpha-synthetic', 'event-alpha-stranger-synthetic', 'channel-event-synthetic', 2, 1, 'actor-event-stranger-synthetic', 'device-alpha-synthetic', 'key-alpha-stranger-synthetic', 1, '\\x01'::bytea, decode('$HASH_HEX', 'hex'));
" 'channel_events_sender_member_fk'
expect_sql_failure_matching "unknown category" "
  INSERT INTO domain.channel_events (tenant_id, event_id, channel_id, channel_seq, sender_actor_type, sender_actor_id, sender_device_id, idempotency_key, category, envelope, content_hash)
  VALUES ('tenant-event-alpha-synthetic', 'event-alpha-category-synthetic', 'channel-event-synthetic', 2, 1, 'actor-event-sender-synthetic', 'device-alpha-synthetic', 'key-alpha-category-synthetic', 9, '\\x01'::bytea, decode('$HASH_HEX', 'hex'));
" 'channel_events_category_known'
expect_sql_failure_matching "short content hash" "
  INSERT INTO domain.channel_events (tenant_id, event_id, channel_id, channel_seq, sender_actor_type, sender_actor_id, sender_device_id, idempotency_key, category, envelope, content_hash)
  VALUES ('tenant-event-alpha-synthetic', 'event-alpha-hash-synthetic', 'channel-event-synthetic', 2, 1, 'actor-event-sender-synthetic', 'device-alpha-synthetic', 'key-alpha-hash-synthetic', 1, '\\x01'::bytea, '\\x01'::bytea);
" 'channel_events_content_hash_size'
expect_sql_failure_matching "empty envelope" "
  INSERT INTO domain.channel_events (tenant_id, event_id, channel_id, channel_seq, sender_actor_type, sender_actor_id, sender_device_id, idempotency_key, category, envelope, content_hash)
  VALUES ('tenant-event-alpha-synthetic', 'event-alpha-empty-synthetic', 'channel-event-synthetic', 2, 1, 'actor-event-sender-synthetic', 'device-alpha-synthetic', 'key-alpha-empty-synthetic', 1, ''::bytea, decode('$HASH_HEX', 'hex'));
" 'channel_events_envelope_size'
expect_sql_failure_matching "event update" "
  UPDATE domain.channel_events SET envelope = '\\x02'::bytea WHERE tenant_id = 'tenant-event-alpha-synthetic';
" 'immutable'
expect_sql_failure_matching "event delete" "
  DELETE FROM domain.channel_events WHERE tenant_id = 'tenant-event-alpha-synthetic';
" 'immutable'

# Send command bindings: one key per device, one binding per event, consistent with the event, immutable.
psql_test --command="
  INSERT INTO domain.send_commands (tenant_id, sender_device_id, idempotency_key, event_id, envelope_digest)
  VALUES ('tenant-event-alpha-synthetic', 'device-alpha-synthetic', 'key-alpha-1-synthetic', 'event-alpha-1-synthetic', decode('$HASH_HEX', 'hex'));
  INSERT INTO domain.send_commands (tenant_id, sender_device_id, idempotency_key, event_id, envelope_digest)
  VALUES ('tenant-event-beta-synthetic', 'device-beta-synthetic', 'key-alpha-1-synthetic', 'event-alpha-1-synthetic', decode('$HASH_HEX', 'hex'));
"
insert_event tenant-event-alpha-synthetic event-alpha-2-synthetic 2 device-alpha-synthetic key-alpha-2-synthetic
expect_sql_failure_matching "same key second event" "
  INSERT INTO domain.send_commands (tenant_id, sender_device_id, idempotency_key, event_id, envelope_digest)
  VALUES ('tenant-event-alpha-synthetic', 'device-alpha-synthetic', 'key-alpha-1-synthetic', 'event-alpha-2-synthetic', decode('$HASH_HEX', 'hex'));
" 'send_commands_pkey'
expect_sql_failure_matching "same event second key" "
  INSERT INTO domain.send_commands (tenant_id, sender_device_id, idempotency_key, event_id, envelope_digest)
  VALUES ('tenant-event-alpha-synthetic', 'device-alpha-synthetic', 'key-alpha-other-synthetic', 'event-alpha-1-synthetic', decode('$HASH_HEX', 'hex'));
" 'send_commands_event_unique|send_commands_exact_event_fk'
expect_sql_failure_matching "binding from another device" "
  INSERT INTO domain.send_commands (tenant_id, sender_device_id, idempotency_key, event_id, envelope_digest)
  VALUES ('tenant-event-alpha-synthetic', 'device-other-synthetic', 'key-alpha-2-synthetic', 'event-alpha-2-synthetic', decode('$HASH_HEX', 'hex'));
" 'send_commands_exact_event_fk'
expect_sql_failure_matching "binding update" "
  UPDATE domain.send_commands SET event_id = 'event-alpha-2-synthetic' WHERE tenant_id = 'tenant-event-alpha-synthetic';
" 'immutable'
expect_sql_failure_matching "binding delete" "
  DELETE FROM domain.send_commands WHERE tenant_id = 'tenant-event-alpha-synthetic';
" 'immutable'

event_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.channel_events WHERE channel_id = 'channel-event-synthetic';
")
test "$event_count" = "3" || postgres_test_fail "expected exactly three committed synthetic events, found $event_count"

psql_test --file="$CHANNEL_EVENT_DOWN"

sentinel_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.channel_event_down_sentinel WHERE marker = 'owned-by-test-synthetic';
")
test "$sentinel_count" = "1" || postgres_test_fail "000011 down removed an unrelated table or row"
channel_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.channels WHERE channel_id = 'channel-event-synthetic';
")
test "$channel_count" = "2" || postgres_test_fail "000011 down removed prior-migration rows"
owned_table_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM information_schema.tables
  WHERE table_schema = 'domain'
    AND table_name IN ('conversation_sequence_heads', 'channel_events', 'send_commands');
")
test "$owned_table_count" = "0" || postgres_test_fail "000011 down left owned tables behind"
owned_function_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM pg_proc JOIN pg_namespace ON pg_namespace.oid = pg_proc.pronamespace
  WHERE nspname = 'domain'
    AND proname IN ('enforce_conversation_sequence_head_lifecycle', 'enforce_channel_event_immutability', 'enforce_send_command_immutability');
")
test "$owned_function_count" = "0" || postgres_test_fail "000011 down left owned functions behind"

psql_test --file="$CHANNEL_EVENT_UP"
owned_table_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM information_schema.tables
  WHERE table_schema = 'domain'
    AND table_name IN ('conversation_sequence_heads', 'channel_events', 'send_commands');
")
test "$owned_table_count" = "3" || postgres_test_fail "000011 up after down did not restore all owned tables"

postgres_test_finish "channel event persistence passed"

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
RESOURCE_ACL_UP="$DB_DIR/migrations/000007_resource_acl.up.sql"
OUTBOX_UP="$DB_DIR/migrations/000008_transactional_outbox.up.sql"
OUTBOX_DOWN="$DB_DIR/migrations/000008_transactional_outbox.down.sql"

. "$TESTS_DIR/postgres_harness.sh"

test -f "$OUTBOX_UP" || postgres_test_fail "missing 000008 up migration"
test -f "$OUTBOX_DOWN" || postgres_test_fail "missing 000008 down migration"
if grep -Eiq '(^|[^[:alnum:]_])CASCADE([^[:alnum:]_]|$)' "$OUTBOX_DOWN"; then
  postgres_test_fail "000008 down must not use CASCADE"
fi
if grep -Eiq 'DROP[[:space:]]+(SCHEMA[[:space:]]+domain|EXTENSION)' "$OUTBOX_DOWN"; then
  postgres_test_fail "000008 down must not drop shared schema or extensions"
fi

postgres_test_start transactional_outbox

psql_test --file="$FOUNDATION_UP"
psql_test --file="$ORGANIZATION_UP"
psql_test --file="$MEMBER_UP"
psql_test --file="$SPACE_UP"
psql_test --file="$CHANNEL_DM_UP"
psql_test --file="$CHANNEL_MEMBERSHIP_UP"
psql_test --file="$RESOURCE_ACL_UP"

psql_test --command="
  CREATE TABLE domain.outbox_down_sentinel (marker text PRIMARY KEY);
  CREATE TABLE domain.outbox_atomic_fixture (marker text PRIMARY KEY);
  INSERT INTO domain.outbox_down_sentinel (marker) VALUES ('owned-by-test-synthetic');
  INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
  VALUES
    ('tenant-outbox-alpha-synthetic', 'Outbox Alpha Synthetic', 1, 'policy-outbox-alpha-v1'),
    ('tenant-outbox-beta-synthetic', 'Outbox Beta Synthetic', 1, 'policy-outbox-beta-v1');
"

psql_test --file="$OUTBOX_UP"

POLICY_DIGEST_HEX=9c9d9ea3a4a5a6a7a8a9aaabacadaeafb0b1b2b3b4b5b6b7b8b9babbbcbdbebf

insert_event() {
  event_tenant=$1
  event_id=$2
  event_payload=$3
  psql_test --command="
    INSERT INTO domain.domain_events (
      tenant_id, event_id, event_type, schema_version,
      aggregate_kind, aggregate_id, payload, occurred_at
    ) VALUES (
      '$event_tenant', '$event_id', 'outbox.synthetic', 1,
      'SyntheticAggregate', '$event_id', $event_payload, '2026-01-01 00:00:00+00'
    );
  "
}

insert_pending_entry() {
  entry_tenant=$1
  entry_event=$2
  psql_test --tuples-only --no-align --command="
    INSERT INTO domain.transactional_outbox (
      tenant_id, event_id, destination, delivery_state, next_attempt_at,
      policy_id, policy_snapshot_digest,
      effective_lease_ms, effective_absolute_lifetime_ms,
      effective_event_retry_ceiling,
      effective_transport_base_ms, effective_transport_cap_ms,
      effective_unknown_base_ms, effective_unknown_cap_ms,
      effective_event_base_ms, effective_event_cap_ms,
      effective_retention_days
    ) VALUES (
      '$entry_tenant', '$entry_event', 'domain-events', 'pending',
      '2026-01-01 00:00:00+00',
      'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'),
      30000, 300000, 5,
      1000, 30000, 2000, 60000, 2000, 60000, 90
    )
    RETURNING outbox_entry_id;
  "
}

# Clone a known-valid pending Entry while replacing one value. This keeps each
# boundary assertion focused on a single contract field.
expect_cloned_entry_failure() {
  clone_label=$1
  clone_event=$2
  clone_field=$3
  clone_value=$4

  total_attempt_count=total_attempt_count
  replay_generation=replay_generation
  generation_attempt_count=generation_attempt_count
  generation_transport_failure_count=generation_transport_failure_count
  generation_unknown_outcome_count=generation_unknown_outcome_count
  generation_failure_count=generation_failure_count
  policy_id=policy_id
  policy_snapshot_digest=policy_snapshot_digest
  effective_lease_ms=effective_lease_ms
  effective_absolute_lifetime_ms=effective_absolute_lifetime_ms
  effective_event_retry_ceiling=effective_event_retry_ceiling
  effective_transport_base_ms=effective_transport_base_ms
  effective_transport_cap_ms=effective_transport_cap_ms
  effective_unknown_base_ms=effective_unknown_base_ms
  effective_unknown_cap_ms=effective_unknown_cap_ms
  effective_event_base_ms=effective_event_base_ms
  effective_event_cap_ms=effective_event_cap_ms
  effective_retention_days=effective_retention_days

  case "$clone_field" in
    total_attempt_count) total_attempt_count=$clone_value ;;
    replay_generation) replay_generation=$clone_value ;;
    generation_attempt_count) generation_attempt_count=$clone_value ;;
    generation_transport_failure_count) generation_transport_failure_count=$clone_value ;;
    generation_unknown_outcome_count) generation_unknown_outcome_count=$clone_value ;;
    generation_failure_count) generation_failure_count=$clone_value ;;
    policy_id) policy_id=$clone_value ;;
    policy_snapshot_digest) policy_snapshot_digest=$clone_value ;;
    effective_lease_ms) effective_lease_ms=$clone_value ;;
    effective_absolute_lifetime_ms) effective_absolute_lifetime_ms=$clone_value ;;
    effective_event_retry_ceiling) effective_event_retry_ceiling=$clone_value ;;
    effective_transport_base_ms) effective_transport_base_ms=$clone_value ;;
    effective_transport_cap_ms) effective_transport_cap_ms=$clone_value ;;
    effective_unknown_base_ms) effective_unknown_base_ms=$clone_value ;;
    effective_unknown_cap_ms) effective_unknown_cap_ms=$clone_value ;;
    effective_event_base_ms) effective_event_base_ms=$clone_value ;;
    effective_event_cap_ms) effective_event_cap_ms=$clone_value ;;
    effective_retention_days) effective_retention_days=$clone_value ;;
    *) postgres_test_fail "test bug: unsupported cloned Entry field" ;;
  esac

  expect_sql_failure "$clone_label" "
    INSERT INTO domain.transactional_outbox (
      tenant_id, event_id, destination, delivery_state,
      total_attempt_count, replay_generation, generation_attempt_count,
      generation_transport_failure_count, generation_unknown_outcome_count,
      generation_failure_count, next_attempt_at, current_attempt_id,
      last_failure_code, parked_at, policy_id, policy_snapshot_digest,
      effective_lease_ms, effective_absolute_lifetime_ms,
      effective_event_retry_ceiling,
      effective_transport_base_ms, effective_transport_cap_ms,
      effective_unknown_base_ms, effective_unknown_cap_ms,
      effective_event_base_ms, effective_event_cap_ms,
      effective_retention_days
    )
    SELECT
      tenant_id, '$clone_event', destination, delivery_state,
      $total_attempt_count, $replay_generation, $generation_attempt_count,
      $generation_transport_failure_count, $generation_unknown_outcome_count,
      $generation_failure_count, next_attempt_at, current_attempt_id,
      last_failure_code, parked_at, $policy_id, $policy_snapshot_digest,
      $effective_lease_ms, $effective_absolute_lifetime_ms,
      $effective_event_retry_ceiling,
      $effective_transport_base_ms, $effective_transport_cap_ms,
      $effective_unknown_base_ms, $effective_unknown_cap_ms,
      $effective_event_base_ms, $effective_event_cap_ms,
      $effective_retention_days
    FROM domain.transactional_outbox
    WHERE outbox_entry_id = $main_entry_id;
  "
}

# Domain Event payload storage is byte-exact at the hard limit. The oversized
# transaction also proves a failed Event write cannot leave its sibling domain
# mutation committed.
insert_event tenant-outbox-alpha-synthetic event-payload-0-synthetic "decode('', 'hex')"
insert_event tenant-outbox-alpha-synthetic event-payload-invalid-utf8-synthetic "decode('00ff80', 'hex')"
insert_event tenant-outbox-alpha-synthetic event-payload-262143-synthetic "decode(repeat('00', 262143), 'hex')"
insert_event tenant-outbox-alpha-synthetic event-payload-262144-synthetic "decode(repeat('00', 262144), 'hex')"

payload_lengths=$(psql_test --tuples-only --no-align --field-separator=, --command="
  SELECT event_id, octet_length(payload)
  FROM domain.domain_events
  WHERE event_id LIKE 'event-payload-%-synthetic'
  ORDER BY event_id;
")
test "$payload_lengths" = "event-payload-0-synthetic,0
event-payload-262143-synthetic,262143
event-payload-262144-synthetic,262144
event-payload-invalid-utf8-synthetic,3" || postgres_test_fail "payload bytes changed at a valid boundary"

invalid_utf8_hex=$(psql_test --tuples-only --no-align --command="
  SELECT encode(payload, 'hex')
  FROM domain.domain_events
  WHERE tenant_id = 'tenant-outbox-alpha-synthetic'
    AND event_id = 'event-payload-invalid-utf8-synthetic';
")
test "$invalid_utf8_hex" = "00ff80" || postgres_test_fail "opaque invalid UTF-8/NUL payload bytes changed"

expect_sql_failure "262145-byte Domain Event payload" "
  BEGIN;
  INSERT INTO domain.outbox_atomic_fixture (marker) VALUES ('oversized-event-sibling');
  INSERT INTO domain.domain_events (
    tenant_id, event_id, event_type, schema_version,
    aggregate_kind, aggregate_id, payload, occurred_at
  ) VALUES (
    'tenant-outbox-alpha-synthetic', 'event-payload-262145-synthetic',
    'outbox.synthetic', 1, 'SyntheticAggregate', 'event-payload-262145-synthetic',
    decode(repeat('00', 262145), 'hex'), '2026-01-01 00:00:00+00'
  );
  COMMIT;
"

atomic_sibling_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.outbox_atomic_fixture
  WHERE marker = 'oversized-event-sibling';
")
test "$atomic_sibling_count" = "0" || postgres_test_fail "oversized payload left a sibling mutation committed"

expect_sql_failure "NULL Domain Event payload" "
  INSERT INTO domain.domain_events (
    tenant_id, event_id, event_type, schema_version,
    aggregate_kind, aggregate_id, payload, occurred_at
  ) VALUES (
    'tenant-outbox-alpha-synthetic', 'event-null-payload-synthetic',
    'outbox.synthetic', 1, 'SyntheticAggregate', 'event-null-payload-synthetic',
    NULL, '2026-01-01 00:00:00+00'
  );
"

insert_event tenant-outbox-alpha-synthetic event-main-synthetic "decode('010203', 'hex')"
insert_event tenant-outbox-alpha-synthetic event-shared-synthetic "decode('04', 'hex')"
insert_event tenant-outbox-beta-synthetic event-shared-synthetic "decode('05', 'hex')"
insert_event tenant-outbox-beta-synthetic event-beta-only-synthetic "decode('06', 'hex')"
insert_event tenant-outbox-alpha-synthetic event-terminal-matrix-synthetic "decode('07', 'hex')"
insert_event tenant-outbox-alpha-synthetic event-claimed-synthetic "decode('08', 'hex')"
insert_event tenant-outbox-alpha-synthetic event-current-target-synthetic "decode('09', 'hex')"
insert_event tenant-outbox-alpha-synthetic event-max-counter-synthetic "decode('0a', 'hex')"

main_entry_id=$(insert_pending_entry tenant-outbox-alpha-synthetic event-main-synthetic)
alpha_shared_entry_id=$(insert_pending_entry tenant-outbox-alpha-synthetic event-shared-synthetic)
beta_shared_entry_id=$(insert_pending_entry tenant-outbox-beta-synthetic event-shared-synthetic)
terminal_entry_id=$(insert_pending_entry tenant-outbox-alpha-synthetic event-terminal-matrix-synthetic)
claimed_entry_id=$(insert_pending_entry tenant-outbox-alpha-synthetic event-claimed-synthetic)
current_target_entry_id=$(insert_pending_entry tenant-outbox-alpha-synthetic event-current-target-synthetic)
max_counter_entry_id=$(insert_pending_entry tenant-outbox-alpha-synthetic event-max-counter-synthetic)

# Composite Tenant/Event ownership and one-destination-per-Event are enforced by
# the database, not inferred from scalar IDs.
expect_sql_failure "cross-Tenant Event reference" "
  INSERT INTO domain.transactional_outbox (
    tenant_id, event_id, destination, delivery_state, next_attempt_at,
    policy_id, policy_snapshot_digest,
    effective_lease_ms, effective_absolute_lifetime_ms,
    effective_event_retry_ceiling,
    effective_transport_base_ms, effective_transport_cap_ms,
    effective_unknown_base_ms, effective_unknown_cap_ms,
    effective_event_base_ms, effective_event_cap_ms,
    effective_retention_days
  ) VALUES (
    'tenant-outbox-alpha-synthetic', 'event-beta-only-synthetic',
    'domain-events', 'pending', '2026-01-01 00:00:00+00',
    'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'),
    30000, 300000, 5, 1000, 30000, 2000, 60000, 2000, 60000, 90
  );
"

expect_sql_failure "duplicate Event destination" "
  INSERT INTO domain.transactional_outbox (
    tenant_id, event_id, destination, delivery_state, next_attempt_at,
    policy_id, policy_snapshot_digest,
    effective_lease_ms, effective_absolute_lifetime_ms,
    effective_event_retry_ceiling,
    effective_transport_base_ms, effective_transport_cap_ms,
    effective_unknown_base_ms, effective_unknown_cap_ms,
    effective_event_base_ms, effective_event_cap_ms,
    effective_retention_days
  ) VALUES (
    'tenant-outbox-alpha-synthetic', 'event-main-synthetic',
    'domain-events', 'pending', '2026-01-01 00:00:00+00',
    'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'),
    30000, 300000, 5, 1000, 30000, 2000, 60000, 2000, 60000, 90
  );
"

expect_sql_failure_matching "unknown destination" "
  INSERT INTO domain.transactional_outbox (
    tenant_id, event_id, destination, delivery_state, next_attempt_at,
    policy_id, policy_snapshot_digest,
    effective_lease_ms, effective_absolute_lifetime_ms,
    effective_event_retry_ceiling,
    effective_transport_base_ms, effective_transport_cap_ms,
    effective_unknown_base_ms, effective_unknown_cap_ms,
    effective_event_base_ms, effective_event_cap_ms,
    effective_retention_days
  ) VALUES (
    'tenant-outbox-beta-synthetic', 'event-beta-only-synthetic',
    'another-stream', 'pending', '2026-01-01 00:00:00+00',
    'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'),
    30000, 300000, 5, 1000, 30000, 2000, 60000, 2000, 60000, 90
  );
" 'transactional_outbox_destination_known'

# Policy snapshots are immutable, canonical, and bounded. Each malformed clone
# has a real matching Event so a failure cannot be attributed to the Event FK.
for policy_event in \
  event-policy-id-invalid-synthetic \
  event-policy-digest-invalid-synthetic \
  event-policy-lease-invalid-synthetic \
  event-policy-lifetime-invalid-synthetic \
  event-policy-retry-invalid-synthetic \
  event-policy-transport-invalid-synthetic \
  event-policy-unknown-invalid-synthetic \
  event-policy-event-invalid-synthetic \
  event-policy-retention-invalid-synthetic \
  event-counter-negative-synthetic \
  event-counter-bounded-synthetic
do
  insert_event tenant-outbox-alpha-synthetic "$policy_event" "decode('0b', 'hex')"
done

expect_cloned_entry_failure "unknown policy version" \
  event-policy-id-invalid-synthetic policy_id "'threadline.outbox.policy/v2'"
expect_cloned_entry_failure "31-byte policy digest" \
  event-policy-digest-invalid-synthetic policy_snapshot_digest "decode(repeat('11', 31), 'hex')"
expect_cloned_entry_failure "lease below minimum" \
  event-policy-lease-invalid-synthetic effective_lease_ms 4999
expect_cloned_entry_failure "absolute lifetime below twice lease" \
  event-policy-lifetime-invalid-synthetic effective_absolute_lifetime_ms 59999
expect_cloned_entry_failure "retry ceiling above maximum" \
  event-policy-retry-invalid-synthetic effective_event_retry_ceiling 21
expect_cloned_entry_failure "transport cap below base" \
  event-policy-transport-invalid-synthetic effective_transport_cap_ms 999
expect_cloned_entry_failure "unknown-outcome base above maximum" \
  event-policy-unknown-invalid-synthetic effective_unknown_base_ms 30001
expect_cloned_entry_failure "event backoff cap below minimum" \
  event-policy-event-invalid-synthetic effective_event_cap_ms 4999
expect_cloned_entry_failure "retention below minimum" \
  event-policy-retention-invalid-synthetic effective_retention_days 29
for counter_field in \
  total_attempt_count \
  generation_attempt_count \
  generation_transport_failure_count \
  generation_unknown_outcome_count \
  generation_failure_count
do
  expect_cloned_entry_failure "negative $counter_field" \
    event-counter-negative-synthetic "$counter_field" -1
done
expect_cloned_entry_failure "generation counter above total" \
  event-counter-bounded-synthetic generation_attempt_count 1

insert_event tenant-outbox-alpha-synthetic event-policy-minimum-synthetic "decode('0d', 'hex')"
insert_event tenant-outbox-alpha-synthetic event-policy-maximum-synthetic "decode('0e', 'hex')"
psql_test --command="
  INSERT INTO domain.transactional_outbox (
    tenant_id, event_id, destination, delivery_state, next_attempt_at,
    policy_id, policy_snapshot_digest,
    effective_lease_ms, effective_absolute_lifetime_ms,
    effective_event_retry_ceiling,
    effective_transport_base_ms, effective_transport_cap_ms,
    effective_unknown_base_ms, effective_unknown_cap_ms,
    effective_event_base_ms, effective_event_cap_ms,
    effective_retention_days
  ) VALUES
    (
      'tenant-outbox-alpha-synthetic', 'event-policy-minimum-synthetic',
      'domain-events', 'pending', '2026-01-01 00:00:00+00',
      'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'),
      5000, 30000, 1, 100, 1000, 500, 5000, 500, 5000, 30
    ),
    (
      'tenant-outbox-alpha-synthetic', 'event-policy-maximum-synthetic',
      'domain-events', 'pending', '2026-01-01 00:00:00+00',
      'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'),
      120000, 900000, 20, 10000, 300000,
      30000, 900000, 30000, 900000, 365
    );
"

policy_boundary_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.transactional_outbox
  WHERE event_id IN (
    'event-policy-minimum-synthetic', 'event-policy-maximum-synthetic'
  );
")
test "$policy_boundary_count" = "2" || postgres_test_fail "valid policy boundary Entries were not stored"

expect_sql_failure "pending Entry without next attempt" "
  UPDATE domain.transactional_outbox
  SET next_attempt_at = NULL
  WHERE outbox_entry_id = $main_entry_id;
"
expect_sql_failure "claimed Entry without current Attempt" "
  UPDATE domain.transactional_outbox
  SET delivery_state = 'claimed', next_attempt_at = NULL
  WHERE outbox_entry_id = $main_entry_id;
"
expect_sql_failure "unknown Entry state" "
  UPDATE domain.transactional_outbox
  SET delivery_state = 'unknown'
  WHERE outbox_entry_id = $main_entry_id;
"
expect_sql_failure "pending-to-delivered shortcut" "
  UPDATE domain.transactional_outbox
  SET delivery_state = 'delivered', next_attempt_at = NULL
  WHERE outbox_entry_id = $main_entry_id;
"

expect_sql_failure "Domain Event update" "
  UPDATE domain.domain_events
  SET payload = decode('ff', 'hex')
  WHERE tenant_id = 'tenant-outbox-alpha-synthetic'
    AND event_id = 'event-main-synthetic';
"
expect_sql_failure "Domain Event delete" "
  DELETE FROM domain.domain_events
  WHERE tenant_id = 'tenant-outbox-alpha-synthetic'
    AND event_id = 'event-main-synthetic';
"
expect_sql_failure "Entry immutable policy change" "
  UPDATE domain.transactional_outbox
  SET effective_retention_days = 91
  WHERE outbox_entry_id = $main_entry_id;
"
expect_sql_failure "Entry counter decrease" "
  UPDATE domain.transactional_outbox
  SET total_attempt_count = total_attempt_count - 1
  WHERE outbox_entry_id = $terminal_entry_id;
"
expect_sql_failure "Entry delete" "
  DELETE FROM domain.transactional_outbox
  WHERE outbox_entry_id = $main_entry_id;
"

# Claim material is one-way storage. The database has no raw-token column and
# both digest columns enforce the frozen 32-byte representation.
raw_token_column_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*)
  FROM information_schema.columns
  WHERE table_schema = 'domain'
    AND table_name IN ('transactional_outbox', 'outbox_delivery_attempts')
    AND column_name IN (
      'claim_token', 'raw_claim_token', 'claim_token_raw',
      'claim_token_plaintext', 'claim_token_secret'
    );
")
test "$raw_token_column_count" = "0" || postgres_test_fail "schema exposes raw claim-token storage"

digest_column_shape=$(psql_test --tuples-only --no-align --field-separator=, --command="
  SELECT data_type, is_nullable, COALESCE(column_default, '')
  FROM information_schema.columns
  WHERE table_schema = 'domain'
    AND table_name = 'outbox_delivery_attempts'
    AND column_name = 'claim_token_digest';
")
test "$digest_column_shape" = "bytea,NO," || postgres_test_fail "claim digest storage shape changed"

# Counter arithmetic must fail before wrapping and leave the persisted row
# unchanged. PostgreSQL bigint is the contract counterpart of Go int64.
counter_shape_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*)
  FROM information_schema.columns
  WHERE table_schema = 'domain'
    AND table_name = 'transactional_outbox'
    AND column_name IN (
      'total_attempt_count', 'replay_generation',
      'generation_attempt_count', 'generation_transport_failure_count',
      'generation_unknown_outcome_count', 'generation_failure_count'
    )
    AND data_type = 'bigint'
    AND is_nullable = 'NO';
")
test "$counter_shape_count" = "6" || postgres_test_fail "Outbox counters are not non-null signed bigint"

psql_test --command="
  UPDATE domain.transactional_outbox
  SET total_attempt_count = 9223372036854775807,
      generation_attempt_count = 9223372036854775807
  WHERE outbox_entry_id = $max_counter_entry_id;
"
expect_sql_failure "Entry counter overflow" "
  UPDATE domain.transactional_outbox
  SET total_attempt_count = total_attempt_count + 1,
      generation_attempt_count = generation_attempt_count + 1
  WHERE outbox_entry_id = $max_counter_entry_id;
"
max_counter_snapshot=$(psql_test --tuples-only --no-align --field-separator=, --command="
  SELECT total_attempt_count, generation_attempt_count
  FROM domain.transactional_outbox
  WHERE outbox_entry_id = $max_counter_entry_id;
")
test "$max_counter_snapshot" = "9223372036854775807,9223372036854775807" || \
  postgres_test_fail "failed counter overflow changed persisted counters"

for counter_field in \
  generation_transport_failure_count \
  generation_unknown_outcome_count \
  generation_failure_count
do
  counter_event="event-max-$counter_field-synthetic"
  insert_event tenant-outbox-alpha-synthetic "$counter_event" "decode('0f', 'hex')"
  counter_entry_id=$(insert_pending_entry tenant-outbox-alpha-synthetic "$counter_event")
  psql_test --command="
    UPDATE domain.transactional_outbox
    SET total_attempt_count = 9223372036854775807,
        generation_attempt_count = 9223372036854775807,
        $counter_field = 9223372036854775807
    WHERE outbox_entry_id = $counter_entry_id;
  "
  expect_sql_failure "$counter_field overflow" "
    UPDATE domain.transactional_outbox
    SET $counter_field = $counter_field + 1
    WHERE outbox_entry_id = $counter_entry_id;
  "
  counter_snapshot=$(psql_test --tuples-only --no-align --command="
    SELECT $counter_field
    FROM domain.transactional_outbox
    WHERE outbox_entry_id = $counter_entry_id;
  ")
  test "$counter_snapshot" = "9223372036854775807" || \
    postgres_test_fail "failed $counter_field overflow changed persisted value"
done

expect_sql_failure_matching "replay generation is frozen until replay support" "
  UPDATE domain.transactional_outbox
  SET replay_generation = 9223372036854775807
  WHERE outbox_entry_id = $main_entry_id;
" 'Transactional Outbox immutable facts cannot be changed'

claim_entry() {
  claim_entry_id=$1
  claim_total_ordinal=$2
  claim_generation_ordinal=$3
  psql_test --tuples-only --no-align --command="
    BEGIN;
    WITH claimed_attempt AS (
      INSERT INTO domain.outbox_delivery_attempts (
        tenant_id, event_id, outbox_entry_id, replay_generation,
        total_attempt_number, generation_attempt_number,
        claim_owner_id, claim_token_digest,
        claimed_at, lease_expires_at, absolute_lease_expires_at,
        outcome, broker_message_id,
        policy_id, policy_snapshot_digest,
        effective_lease_ms, effective_retention_days
      )
      SELECT
        tenant_id, event_id, outbox_entry_id, replay_generation,
        $claim_total_ordinal, $claim_generation_ordinal,
        'worker-outbox-synthetic', decode(repeat('10', 32), 'hex'),
        '2026-01-01 00:00:00+00', '2026-01-01 00:00:30+00',
        '2026-01-01 00:05:00+00', 'active', repeat('a', 64),
        policy_id, policy_snapshot_digest,
        effective_lease_ms, effective_retention_days
      FROM domain.transactional_outbox
      WHERE outbox_entry_id = $claim_entry_id
      RETURNING delivery_attempt_id
    )
    UPDATE domain.transactional_outbox
    SET delivery_state = 'claimed',
        total_attempt_count = $claim_total_ordinal,
        generation_attempt_count = $claim_generation_ordinal,
        next_attempt_at = NULL,
        current_attempt_id = (SELECT delivery_attempt_id FROM claimed_attempt)
    WHERE outbox_entry_id = $claim_entry_id;
    COMMIT;
    SELECT current_attempt_id
    FROM domain.transactional_outbox
    WHERE outbox_entry_id = $claim_entry_id;
  "
}

claimed_attempt_id=$(claim_entry "$claimed_entry_id" 1 1)

# Attempt ownership is exact across Tenant, Event, Entry, generation, and policy
# snapshot. Every malformed insert otherwise satisfies the active-row contract.
expect_sql_failure_matching "cross-Tenant Attempt reference" "
  INSERT INTO domain.outbox_delivery_attempts (
    tenant_id, event_id, outbox_entry_id, replay_generation,
    total_attempt_number, generation_attempt_number,
    claim_owner_id, claim_token_digest,
    claimed_at, lease_expires_at, absolute_lease_expires_at,
    outcome, broker_message_id, policy_id, policy_snapshot_digest,
    effective_lease_ms, effective_retention_days
  )
  SELECT
    'tenant-outbox-beta-synthetic', 'event-shared-synthetic',
    $alpha_shared_entry_id, replay_generation,
    1, 1, 'worker-cross-tenant-synthetic', decode(repeat('20', 32), 'hex'),
    '2026-01-01 00:00:00+00', '2026-01-01 00:00:30+00',
    '2026-01-01 00:05:00+00', 'active', repeat('b', 64),
    policy_id, policy_snapshot_digest, effective_lease_ms, effective_retention_days
  FROM domain.transactional_outbox
  WHERE outbox_entry_id = $alpha_shared_entry_id;
" 'outbox_delivery_attempts_exact_entry_policy_fk'

expect_sql_failure_matching "cross-Event Attempt reference" "
  INSERT INTO domain.outbox_delivery_attempts (
    tenant_id, event_id, outbox_entry_id, replay_generation,
    total_attempt_number, generation_attempt_number,
    claim_owner_id, claim_token_digest,
    claimed_at, lease_expires_at, absolute_lease_expires_at,
    outcome, broker_message_id, policy_id, policy_snapshot_digest,
    effective_lease_ms, effective_retention_days
  )
  SELECT
    tenant_id, 'event-main-synthetic', $alpha_shared_entry_id, replay_generation,
    1, 1, 'worker-cross-event-synthetic', decode(repeat('30', 32), 'hex'),
    '2026-01-01 00:00:00+00', '2026-01-01 00:00:30+00',
    '2026-01-01 00:05:00+00', 'active', repeat('c', 64),
    policy_id, policy_snapshot_digest, effective_lease_ms, effective_retention_days
  FROM domain.transactional_outbox
  WHERE outbox_entry_id = $alpha_shared_entry_id;
" 'outbox_delivery_attempts_exact_entry_policy_fk'

expect_sql_failure_matching "Attempt policy digest mismatch" "
  INSERT INTO domain.outbox_delivery_attempts (
    tenant_id, event_id, outbox_entry_id, replay_generation,
    total_attempt_number, generation_attempt_number,
    claim_owner_id, claim_token_digest,
    claimed_at, lease_expires_at, absolute_lease_expires_at,
    outcome, broker_message_id, policy_id, policy_snapshot_digest,
    effective_lease_ms, effective_retention_days
  )
  SELECT
    tenant_id, event_id, outbox_entry_id, replay_generation,
    1, 1, 'worker-policy-mismatch-synthetic', decode(repeat('40', 32), 'hex'),
    '2026-01-01 00:00:00+00', '2026-01-01 00:00:30+00',
    '2026-01-01 00:05:00+00', 'active', repeat('d', 64),
    policy_id, decode(repeat('41', 32), 'hex'),
    effective_lease_ms, effective_retention_days
  FROM domain.transactional_outbox
  WHERE outbox_entry_id = $alpha_shared_entry_id;
" 'outbox_delivery_attempts_exact_entry_policy_fk'

expect_sql_failure "31-byte claim digest" "
  INSERT INTO domain.outbox_delivery_attempts (
    tenant_id, event_id, outbox_entry_id, replay_generation,
    total_attempt_number, generation_attempt_number,
    claim_owner_id, claim_token_digest,
    claimed_at, lease_expires_at, absolute_lease_expires_at,
    outcome, broker_message_id, policy_id, policy_snapshot_digest,
    effective_lease_ms, effective_retention_days
  )
  SELECT
    tenant_id, event_id, outbox_entry_id, replay_generation,
    1, 1, 'worker-short-digest-synthetic', decode(repeat('11', 31), 'hex'),
    '2026-01-01 00:00:00+00', '2026-01-01 00:00:30+00',
    '2026-01-01 00:05:00+00', 'active', repeat('e', 64),
    policy_id, policy_snapshot_digest, effective_lease_ms, effective_retention_days
  FROM domain.transactional_outbox
  WHERE outbox_entry_id = $alpha_shared_entry_id;
"

expect_sql_failure "non-canonical broker message ID" "
  INSERT INTO domain.outbox_delivery_attempts (
    tenant_id, event_id, outbox_entry_id, replay_generation,
    total_attempt_number, generation_attempt_number,
    claim_owner_id, claim_token_digest,
    claimed_at, lease_expires_at, absolute_lease_expires_at,
    outcome, broker_message_id, policy_id, policy_snapshot_digest,
    effective_lease_ms, effective_retention_days
  )
  SELECT
    tenant_id, event_id, outbox_entry_id, replay_generation,
    1, 1, 'worker-message-id-synthetic', decode(repeat('50', 32), 'hex'),
    '2026-01-01 00:00:00+00', '2026-01-01 00:00:30+00',
    '2026-01-01 00:05:00+00', 'active', repeat('A', 64),
    policy_id, policy_snapshot_digest, effective_lease_ms, effective_retention_days
  FROM domain.transactional_outbox
  WHERE outbox_entry_id = $alpha_shared_entry_id;
"

expect_sql_failure "zero Attempt ordinal" "
  INSERT INTO domain.outbox_delivery_attempts (
    tenant_id, event_id, outbox_entry_id, replay_generation,
    total_attempt_number, generation_attempt_number,
    claim_owner_id, claim_token_digest,
    claimed_at, lease_expires_at, absolute_lease_expires_at,
    outcome, broker_message_id, policy_id, policy_snapshot_digest,
    effective_lease_ms, effective_retention_days
  )
  SELECT
    tenant_id, event_id, outbox_entry_id, replay_generation,
    0, 0, 'worker-zero-ordinal-synthetic', decode(repeat('60', 32), 'hex'),
    '2026-01-01 00:00:00+00', '2026-01-01 00:00:30+00',
    '2026-01-01 00:05:00+00', 'active', repeat('f', 64),
    policy_id, policy_snapshot_digest, effective_lease_ms, effective_retention_days
  FROM domain.transactional_outbox
  WHERE outbox_entry_id = $alpha_shared_entry_id;
"

expect_sql_failure "active Attempt on pending Entry" "
  BEGIN;
  INSERT INTO domain.outbox_delivery_attempts (
    tenant_id, event_id, outbox_entry_id, replay_generation,
    total_attempt_number, generation_attempt_number,
    claim_owner_id, claim_token_digest,
    claimed_at, lease_expires_at, absolute_lease_expires_at,
    outcome, broker_message_id, policy_id, policy_snapshot_digest,
    effective_lease_ms, effective_retention_days
  )
  SELECT
    tenant_id, event_id, outbox_entry_id, replay_generation,
    1, 1, 'worker-orphan-active-synthetic', decode(repeat('70', 32), 'hex'),
    '2026-01-01 00:00:00+00', '2026-01-01 00:00:30+00',
    '2026-01-01 00:05:00+00', 'active', repeat('1', 64),
    policy_id, policy_snapshot_digest, effective_lease_ms, effective_retention_days
  FROM domain.transactional_outbox
  WHERE outbox_entry_id = $alpha_shared_entry_id;
  COMMIT;
"

expect_sql_failure_matching "second active Attempt for exact Entry" "
  INSERT INTO domain.outbox_delivery_attempts (
    tenant_id, event_id, outbox_entry_id, replay_generation,
    total_attempt_number, generation_attempt_number,
    claim_owner_id, claim_token_digest,
    claimed_at, lease_expires_at, absolute_lease_expires_at,
    outcome, broker_message_id, policy_id, policy_snapshot_digest,
    effective_lease_ms, effective_retention_days
  )
  SELECT
    tenant_id, event_id, outbox_entry_id, replay_generation,
    2, 2, 'worker-second-active-synthetic', decode(repeat('80', 32), 'hex'),
    '2026-01-01 00:00:00+00', '2026-01-01 00:00:30+00',
    '2026-01-01 00:05:00+00', 'active', repeat('2', 64),
    policy_id, policy_snapshot_digest, effective_lease_ms, effective_retention_days
  FROM domain.transactional_outbox
  WHERE outbox_entry_id = $claimed_entry_id;
" 'outbox_delivery_attempts_one_active_per_entry'

expect_sql_failure "claimed Entry ordinal mismatch" "
  BEGIN;
  WITH mismatched_attempt AS (
    INSERT INTO domain.outbox_delivery_attempts (
      tenant_id, event_id, outbox_entry_id, replay_generation,
      total_attempt_number, generation_attempt_number,
      claim_owner_id, claim_token_digest,
      claimed_at, lease_expires_at, absolute_lease_expires_at,
      outcome, broker_message_id, policy_id, policy_snapshot_digest,
      effective_lease_ms, effective_retention_days
    )
    SELECT
      tenant_id, event_id, outbox_entry_id, replay_generation,
      2, 2, 'worker-ordinal-mismatch-synthetic', decode(repeat('90', 32), 'hex'),
      '2026-01-01 00:00:00+00', '2026-01-01 00:00:30+00',
      '2026-01-01 00:05:00+00', 'active', repeat('3', 64),
      policy_id, policy_snapshot_digest, effective_lease_ms, effective_retention_days
    FROM domain.transactional_outbox
    WHERE outbox_entry_id = $current_target_entry_id
    RETURNING delivery_attempt_id
  )
  UPDATE domain.transactional_outbox
  SET delivery_state = 'claimed', total_attempt_count = 1,
      generation_attempt_count = 1, next_attempt_at = NULL,
      current_attempt_id = (SELECT delivery_attempt_id FROM mismatched_attempt)
  WHERE outbox_entry_id = $current_target_entry_id;
  COMMIT;
"

expect_sql_failure "cross-Entry current Attempt reference" "
  BEGIN;
  UPDATE domain.transactional_outbox
  SET delivery_state = 'claimed', total_attempt_count = 1,
      generation_attempt_count = 1, next_attempt_at = NULL,
      current_attempt_id = $claimed_attempt_id
  WHERE outbox_entry_id = $current_target_entry_id;
  COMMIT;
"

# An active claim can only renew monotonically and never beyond its absolute
# lease. Failed renewals leave the authoritative lease unchanged.
psql_test --command="
  UPDATE domain.outbox_delivery_attempts
  SET lease_expires_at = '2026-01-01 00:00:45+00'
  WHERE delivery_attempt_id = $claimed_attempt_id;
"
expect_sql_failure "equal lease renewal" "
  UPDATE domain.outbox_delivery_attempts
  SET lease_expires_at = '2026-01-01 00:00:45+00'
  WHERE delivery_attempt_id = $claimed_attempt_id;
"
expect_sql_failure "decreasing lease renewal" "
  UPDATE domain.outbox_delivery_attempts
  SET lease_expires_at = '2026-01-01 00:00:44+00'
  WHERE delivery_attempt_id = $claimed_attempt_id;
"
expect_sql_failure "renewal beyond absolute lease" "
  UPDATE domain.outbox_delivery_attempts
  SET lease_expires_at = '2026-01-01 00:05:01+00'
  WHERE delivery_attempt_id = $claimed_attempt_id;
"
renewed_lease=$(psql_test --tuples-only --no-align --command="
  SELECT lease_expires_at = '2026-01-01 00:00:45+00'::timestamptz
  FROM domain.outbox_delivery_attempts
  WHERE delivery_attempt_id = $claimed_attempt_id;
")
test "$renewed_lease" = "t" || postgres_test_fail "failed renewal changed the active lease"

# Invalid terminal matrices are rejected while the Attempt remains active.
expect_sql_failure_matching "delivered Attempt without broker ACK fields" "
  UPDATE domain.outbox_delivery_attempts
  SET outcome = 'delivered',
      finished_at = '2026-01-01 00:00:20+00',
      evidence_not_before = '2026-04-01 00:00:20+00'
  WHERE delivery_attempt_id = $claimed_attempt_id;
" 'outbox_delivery_attempts_terminal_fields_consistent'
expect_sql_failure_matching "transport outcome with mismatched failure code" "
  UPDATE domain.outbox_delivery_attempts
  SET outcome = 'transport_unavailable',
      finished_at = '2026-01-01 00:00:20+00',
      failure_code = 'event-retryable',
      evidence_not_before = '2026-04-01 00:00:20+00'
  WHERE delivery_attempt_id = $claimed_attempt_id;
" 'outbox_delivery_attempts_terminal_fields_consistent'
expect_sql_failure_matching "retention cutoff off by one second" "
  UPDATE domain.outbox_delivery_attempts
  SET outcome = 'lease_expired',
      finished_at = '2026-01-01 00:01:00+00',
      evidence_not_before = '2026-04-01 00:01:01+00'
  WHERE delivery_attempt_id = $claimed_attempt_id;
" 'outbox_delivery_attempts_evidence_cutoff_exact'
expect_sql_failure_matching "unknown Attempt outcome" "
  UPDATE domain.outbox_delivery_attempts
  SET outcome = 'unknown',
      finished_at = '2026-01-01 00:00:20+00',
      evidence_not_before = '2026-04-01 00:00:20+00'
  WHERE delivery_attempt_id = $claimed_attempt_id;
" 'outbox_delivery_attempts_outcome_known'
expect_sql_failure_matching "terminalization changes lease" "
  UPDATE domain.outbox_delivery_attempts
  SET outcome = 'lease_expired',
      lease_expires_at = '2026-01-01 00:00:46+00',
      finished_at = '2026-01-01 00:01:00+00',
      evidence_not_before = '2026-04-01 00:01:00+00'
  WHERE delivery_attempt_id = $claimed_attempt_id;
" 'Terminalizing an Outbox Delivery Attempt cannot change its lease'

# Delivered is the ACK-observed terminal path: broker evidence is mandatory,
# the Entry and Attempt advance atomically, and neither terminal row can change.
psql_test --command="
  BEGIN;
  UPDATE domain.outbox_delivery_attempts
  SET outcome = 'delivered',
      finished_at = '2026-01-01 00:00:20+00',
      broker_stream = 'THREADLINE_DOMAIN_EVENTS',
      broker_sequence = 18446744073709551615,
      broker_duplicate = false,
      evidence_not_before = '2026-04-01 00:00:20+00'
  WHERE delivery_attempt_id = $claimed_attempt_id;
  UPDATE domain.transactional_outbox
  SET delivery_state = 'delivered', current_attempt_id = NULL
  WHERE outbox_entry_id = $claimed_entry_id;
  COMMIT;
"

delivered_snapshot=$(psql_test --tuples-only --no-align --field-separator=, --command="
  SELECT e.delivery_state, a.outcome, a.broker_stream,
         a.broker_sequence, a.broker_duplicate
  FROM domain.transactional_outbox e
  JOIN domain.outbox_delivery_attempts a
    ON a.delivery_attempt_id = $claimed_attempt_id
  WHERE e.outbox_entry_id = $claimed_entry_id;
")
test "$delivered_snapshot" = "delivered,delivered,THREADLINE_DOMAIN_EVENTS,18446744073709551615,f" || \
  postgres_test_fail "delivered ACK evidence was not stored exactly"

expect_sql_failure "terminal Attempt rewrite" "
  UPDATE domain.outbox_delivery_attempts
  SET broker_duplicate = true
  WHERE delivery_attempt_id = $claimed_attempt_id;
"
expect_sql_failure "terminal Attempt delete" "
  DELETE FROM domain.outbox_delivery_attempts
  WHERE delivery_attempt_id = $claimed_attempt_id;
"
expect_sql_failure "terminal Entry rewrite" "
  UPDATE domain.transactional_outbox
  SET total_attempt_count = total_attempt_count + 1
  WHERE outbox_entry_id = $claimed_entry_id;
"
expect_sql_failure "terminal Entry delete" "
  DELETE FROM domain.transactional_outbox
  WHERE outbox_entry_id = $claimed_entry_id;
"

exercise_terminal_outcome() {
  terminal_outcome=$1
  terminal_failure_code=$2
  terminal_entry_state=$3
  terminal_last_failure=$4
  terminal_counter_field=$5

  terminal_event="event-outcome-$terminal_outcome-synthetic"
  insert_event tenant-outbox-alpha-synthetic "$terminal_event" "decode('0c', 'hex')"
  terminal_case_entry_id=$(insert_pending_entry tenant-outbox-alpha-synthetic "$terminal_event")
  terminal_case_attempt_id=$(claim_entry "$terminal_case_entry_id" 1 1)

  terminal_next_attempt="'2026-01-01 00:02:00+00'"
  terminal_parked_at=NULL
  case "$terminal_entry_state" in
    parked)
      terminal_next_attempt=NULL
      terminal_parked_at="'2026-01-01 00:01:00+00'"
      ;;
    pending) ;;
    *) postgres_test_fail "test bug: unsupported terminal Entry state" ;;
  esac

  transport_failures=0
  unknown_failures=0
  event_failures=0
  case "$terminal_counter_field" in
    transport) transport_failures=1 ;;
    unknown) unknown_failures=1 ;;
    event) event_failures=1 ;;
    none) ;;
    *) postgres_test_fail "test bug: unsupported terminal counter" ;;
  esac

  psql_test --command="
    BEGIN;
    UPDATE domain.outbox_delivery_attempts
    SET outcome = '$terminal_outcome',
        finished_at = '2026-01-01 00:01:00+00',
        failure_code = $terminal_failure_code,
        evidence_not_before = '2026-04-01 00:01:00+00'
    WHERE delivery_attempt_id = $terminal_case_attempt_id;
    UPDATE domain.transactional_outbox
    SET delivery_state = '$terminal_entry_state',
        generation_transport_failure_count = $transport_failures,
        generation_unknown_outcome_count = $unknown_failures,
        generation_failure_count = $event_failures,
        next_attempt_at = $terminal_next_attempt,
        current_attempt_id = NULL,
        last_failure_code = $terminal_last_failure,
        parked_at = $terminal_parked_at
    WHERE outbox_entry_id = $terminal_case_entry_id;
    COMMIT;
  "
}

exercise_terminal_outcome transport_unavailable "'transport-unavailable'" pending "'transport-unavailable'" transport
exercise_terminal_outcome publish_outcome_unknown "'publish-outcome-unknown'" pending "'publish-outcome-unknown'" unknown
exercise_terminal_outcome event_retryable "'event-retryable'" pending "'event-retryable'" event
exercise_terminal_outcome event_permanent "'event-permanent'" parked "'event-permanent'" none
exercise_terminal_outcome lease_expired NULL pending NULL none

outcome_matrix=$(psql_test --tuples-only --no-align --field-separator=, --command="
  SELECT outcome, count(*)
  FROM domain.outbox_delivery_attempts
  GROUP BY outcome
  ORDER BY outcome;
")
test "$outcome_matrix" = "delivered,1
event_permanent,1
event_retryable,1
lease_expired,1
publish_outcome_unknown,1
transport_unavailable,1" || postgres_test_fail "valid Attempt outcome matrix is incomplete"

terminal_counter_matrix=$(psql_test --tuples-only --no-align --field-separator=, --command="
  SELECT
    event_id,
    generation_transport_failure_count,
    generation_unknown_outcome_count,
    generation_failure_count
  FROM domain.transactional_outbox
  WHERE event_id LIKE 'event-outcome-%-synthetic'
  ORDER BY event_id;
")
test "$terminal_counter_matrix" = "event-outcome-event_permanent-synthetic,0,0,0
event-outcome-event_retryable-synthetic,0,0,1
event-outcome-lease_expired-synthetic,0,0,0
event-outcome-publish_outcome_unknown-synthetic,0,1,0
event-outcome-transport_unavailable-synthetic,1,0,0" || \
  postgres_test_fail "terminal outcomes changed the wrong retry-budget counters"

retention_seconds=$(psql_test --tuples-only --no-align --command="
  SELECT string_agg(
    extract(epoch FROM (evidence_not_before - finished_at))::bigint::text,
    ',' ORDER BY outcome
  )
  FROM domain.outbox_delivery_attempts
  WHERE outcome <> 'active';
")
test "$retention_seconds" = "7776000,7776000,7776000,7776000,7776000,7776000" || \
  postgres_test_fail "evidence retention is not exactly 90 days in seconds"

psql_test --file="$OUTBOX_DOWN"

sentinel_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.outbox_down_sentinel
  WHERE marker = 'owned-by-test-synthetic';
")
test "$sentinel_count" = "1" || postgres_test_fail "000008 down removed an unrelated table or row"

organization_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.organizations
  WHERE tenant_id IN ('tenant-outbox-alpha-synthetic', 'tenant-outbox-beta-synthetic');
")
test "$organization_count" = "2" || postgres_test_fail "000008 down removed prior-migration rows"

extension_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM pg_extension WHERE extname = 'plpgsql';
")
test "$extension_count" = "1" || postgres_test_fail "000008 down removed shared plpgsql"

owned_table_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*)
  FROM information_schema.tables
  WHERE table_schema = 'domain'
    AND table_name IN ('domain_events', 'transactional_outbox', 'outbox_delivery_attempts');
")
test "$owned_table_count" = "0" || postgres_test_fail "000008 down left owned tables behind"

psql_test --file="$OUTBOX_UP"
owned_table_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*)
  FROM information_schema.tables
  WHERE table_schema = 'domain'
    AND table_name IN ('domain_events', 'transactional_outbox', 'outbox_delivery_attempts');
")
test "$owned_table_count" = "3" || postgres_test_fail "000008 up after down did not restore all owned tables"

postgres_test_finish "transactional Outbox persistence passed"

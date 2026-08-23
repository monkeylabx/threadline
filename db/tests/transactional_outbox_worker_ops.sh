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
WORKER_OPS_UP="$DB_DIR/migrations/000009_transactional_outbox_worker_ops.up.sql"
WORKER_OPS_DOWN="$DB_DIR/migrations/000009_transactional_outbox_worker_ops.down.sql"

. "$TESTS_DIR/postgres_harness.sh"

test -f "$WORKER_OPS_UP" || postgres_test_fail "missing 000009 Worker operations up migration"
test -f "$WORKER_OPS_DOWN" || postgres_test_fail "missing 000009 Worker operations down migration"
if grep -Eiq '(^|[^[:alnum:]_])CASCADE([^[:alnum:]_]|$)' "$WORKER_OPS_DOWN"; then
  postgres_test_fail "000009 down must not use CASCADE"
fi
if grep -Eiq 'DROP[[:space:]]+(SCHEMA|EXTENSION|TABLE)' "$WORKER_OPS_DOWN"; then
  postgres_test_fail "000009 down must not drop shared schema, extensions, or tables"
fi

postgres_test_start transactional_outbox_worker_ops

psql_test --file="$FOUNDATION_UP"
psql_test --file="$ORGANIZATION_UP"
psql_test --file="$MEMBER_UP"
psql_test --file="$SPACE_UP"
psql_test --file="$CHANNEL_DM_UP"
psql_test --file="$CHANNEL_MEMBERSHIP_UP"
psql_test --file="$RESOURCE_ACL_UP"
psql_test --file="$OUTBOX_UP"

# pgcrypto is a deployment precondition, not a migration-owned dependency.
# Use --file directly: psql's --command mode does not accept a backslash
# include command after leading whitespace consistently across platforms.
if psql_test --file="$WORKER_OPS_UP" >"$temp_dir/failure.out" 2>"$temp_dir/failure.err"; then
  postgres_test_fail "000009 without pre-provisioned pgcrypto unexpectedly succeeded"
fi
if ! grep -Eiq 'pgcrypto|digest|gen_random_bytes|does not exist' "$temp_dir/failure.err"; then
  postgres_test_fail "000009 without pre-provisioned pgcrypto failed for an unexpected reason"
fi

worker_function_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*)
  FROM pg_proc AS procedure
  JOIN pg_namespace AS namespace
    ON namespace.oid = procedure.pronamespace
  WHERE namespace.nspname = 'domain'
    AND procedure.proname IN (
      'transactional_outbox_claim_digest',
      'transactional_outbox_message_id',
      'transactional_outbox_backoff_delay_ms',
      'claim_transactional_outbox_batch',
      'renew_transactional_outbox_claim',
      'acknowledge_transactional_outbox_published',
      'record_transactional_outbox_publish_failure'
    );
")
test "$worker_function_count" = "0" || postgres_test_fail "failed 000009 up left Worker functions installed"

# pgcrypto is a deployment precondition owned outside migration 000009.
psql_test --command="
  CREATE EXTENSION pgcrypto;
  CREATE TABLE domain.worker_ops_down_sentinel (marker text PRIMARY KEY);
  INSERT INTO domain.worker_ops_down_sentinel (marker)
  VALUES ('worker-ops-owned-by-test-synthetic');
  CREATE FUNCTION domain.worker_ops_unrelated_sentinel()
  RETURNS integer
  LANGUAGE sql
  IMMUTABLE
  AS 'SELECT 1';
"

psql_test --file="$WORKER_OPS_UP"

assert_sql_equals() {
  assertion_label=$1
  assertion_expected=$2
  assertion_sql=$3
  assertion_actual=$(psql_test --tuples-only --no-align --command="$assertion_sql")
  test "$assertion_actual" = "$assertion_expected" || postgres_test_fail "$assertion_label (expected: $assertion_expected; actual: $assertion_actual)"
}

expect_invalid_input() {
  invalid_label=$1
  invalid_statement=$2
  expect_sql_failure_matching "$invalid_label" "$invalid_statement" 'transactional outbox: invalid-input'
}

POLICY_DIGEST_HEX=9c9d9ea3a4a5a6a7a8a9aaabacadaeafb0b1b2b3b4b5b6b7b8b9babbbcbdbebf
TOKEN_RAW_HEX=000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f
TOKEN_DIGEST_HEX=0fced3787dc44e7855171187da0812df307108fb766c97f6082824715b310994
MESSAGE_ID_GOLDEN=e57ad815a402753dd7698b0e941f70108383c92afecfc5d0c2b699ac36c82e97

psql_test --command="
  INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
  VALUES
    ('tenant-worker-alpha-synthetic', 'Worker Alpha Synthetic', 1, 'policy-worker-alpha-v1'),
    ('tenant-worker-beta-synthetic', 'Worker Beta Synthetic', 1, 'policy-worker-beta-v1');
"

# Shared cryptographic and deterministic-scheduling Goldens pin exact bytes,
# domain separation, signed-bigint boundaries, and cap behavior.
assert_sql_equals "claim digest Golden changed" "$TOKEN_DIGEST_HEX" "
  SELECT encode(
    domain.transactional_outbox_claim_digest(decode('$TOKEN_RAW_HEX', 'hex')),
    'hex'
  );
"
assert_sql_equals "message ID Golden changed" "$MESSAGE_ID_GOLDEN" "
  SELECT domain.transactional_outbox_message_id('t', 'e', 'domain-events', 0);
"
assert_sql_equals "transport backoff Golden changed" "443" "
  SELECT domain.transactional_outbox_backoff_delay_ms(
    decode('$TOKEN_DIGEST_HEX', 'hex'), 1::smallint, 1, 1000, 60000
  );
"
assert_sql_equals "unknown-outcome backoff Golden changed" "3268" "
  SELECT domain.transactional_outbox_backoff_delay_ms(
    decode('$TOKEN_DIGEST_HEX', 'hex'), 2::smallint, 1, 5000, 300000
  );
"
assert_sql_equals "event-retryable backoff Golden changed" "4312" "
  SELECT domain.transactional_outbox_backoff_delay_ms(
    decode('$TOKEN_DIGEST_HEX', 'hex'), 3::smallint, 1, 5000, 300000
  );
"
assert_sql_equals "backoff ordinal boundary Goldens changed" "537,19157,21531,1626,199278,49087,6138,283716,94714" "
  WITH cases(failure_class, ordinal, base_ms, cap_ms) AS (
    VALUES
      (1::smallint, 2::bigint, 1000, 60000),
      (1::smallint, 21::bigint, 1000, 60000),
      (1::smallint, 9223372036854775807::bigint, 1000, 60000),
      (2::smallint, 2::bigint, 5000, 300000),
      (2::smallint, 21::bigint, 5000, 300000),
      (2::smallint, 9223372036854775807::bigint, 5000, 300000),
      (3::smallint, 2::bigint, 5000, 300000),
      (3::smallint, 21::bigint, 5000, 300000),
      (3::smallint, 9223372036854775807::bigint, 5000, 300000)
  )
  SELECT string_agg(
    domain.transactional_outbox_backoff_delay_ms(
      decode('$TOKEN_DIGEST_HEX', 'hex'), failure_class, ordinal, base_ms, cap_ms
    )::text,
    ',' ORDER BY failure_class, ordinal
  )
  FROM cases;
"
expect_invalid_input "short raw claim token" "
  SELECT domain.transactional_outbox_claim_digest(decode('00', 'hex'));
"
expect_invalid_input "zero backoff ordinal" "
  SELECT domain.transactional_outbox_backoff_delay_ms(
    decode('$TOKEN_DIGEST_HEX', 'hex'), 1::smallint, 0, 1000, 60000
  );
"
expect_invalid_input "unknown backoff class" "
  SELECT domain.transactional_outbox_backoff_delay_ms(
    decode('$TOKEN_DIGEST_HEX', 'hex'), 4::smallint, 1, 1000, 60000
  );
"
expect_invalid_input "backoff base above cap" "
  SELECT domain.transactional_outbox_backoff_delay_ms(
    decode('$TOKEN_DIGEST_HEX', 'hex'), 1::smallint, 1, 60001, 60000
  );
"

# Deterministic claim order is eligibility time, Event enqueue time, then Entry
# identity. Attempt identities let the assertion observe the returned order
# without depending on an unordered aggregate implementation detail.
psql_test --command="
  INSERT INTO domain.domain_events (
    tenant_id, event_id, event_type, schema_version,
    aggregate_kind, aggregate_id, payload, occurred_at, enqueued_at
  ) VALUES
    ('tenant-worker-alpha-synthetic', 'event-order-a-synthetic', 'worker.synthetic', 1,
     'SyntheticAggregate', 'event-order-a-synthetic', decode('01', 'hex'),
     '2026-01-01 00:00:00+00', '2026-01-01 00:00:03+00'),
    ('tenant-worker-alpha-synthetic', 'event-order-b-synthetic', 'worker.synthetic', 1,
     'SyntheticAggregate', 'event-order-b-synthetic', decode('02', 'hex'),
     '2026-01-01 00:00:00+00', '2026-01-01 00:00:01+00'),
    ('tenant-worker-alpha-synthetic', 'event-order-c-synthetic', 'worker.synthetic', 1,
     'SyntheticAggregate', 'event-order-c-synthetic', decode('03', 'hex'),
     '2026-01-01 00:00:00+00', '2026-01-01 00:00:01+00'),
    ('tenant-worker-alpha-synthetic', 'event-order-d-synthetic', 'worker.synthetic', 1,
     'SyntheticAggregate', 'event-order-d-synthetic', decode('04', 'hex'),
     '2026-01-01 00:00:00+00', '2026-01-01 00:00:00+00');

  INSERT INTO domain.transactional_outbox (
    tenant_id, event_id, destination, next_attempt_at,
    policy_id, policy_snapshot_digest,
    effective_lease_ms, effective_absolute_lifetime_ms,
    effective_event_retry_ceiling,
    effective_transport_base_ms, effective_transport_cap_ms,
    effective_unknown_base_ms, effective_unknown_cap_ms,
    effective_event_base_ms, effective_event_cap_ms,
    effective_retention_days
  ) VALUES
    ('tenant-worker-alpha-synthetic', 'event-order-b-synthetic', 'domain-events', '2025-01-01 00:00:00+00',
     'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'),
     30000, 300000, 8, 1000, 60000, 5000, 300000, 5000, 300000, 90),
    ('tenant-worker-alpha-synthetic', 'event-order-c-synthetic', 'domain-events', '2025-01-01 00:00:00+00',
     'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'),
     30000, 300000, 8, 1000, 60000, 5000, 300000, 5000, 300000, 90),
    ('tenant-worker-alpha-synthetic', 'event-order-a-synthetic', 'domain-events', '2025-01-01 00:00:00+00',
     'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'),
     30000, 300000, 8, 1000, 60000, 5000, 300000, 5000, 300000, 90),
    ('tenant-worker-alpha-synthetic', 'event-order-d-synthetic', 'domain-events', '2025-01-02 00:00:00+00',
     'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'),
     30000, 300000, 8, 1000, 60000, 5000, 300000, 5000, 300000, 90);

  CREATE TABLE domain.worker_ops_order_claims AS
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-order-synthetic', 4);
"

assert_sql_equals "claim eligibility order changed" "event-order-b-synthetic,event-order-c-synthetic,event-order-a-synthetic,event-order-d-synthetic" "
  SELECT string_agg(event_id, ',' ORDER BY delivery_attempt_id)
  FROM domain.worker_ops_order_claims;
"
assert_sql_equals "claim did not return exact immutable facts and four claimed rows" "4" "
  SELECT count(*)
  FROM domain.worker_ops_order_claims
  WHERE result_code = 'claimed'
    AND destination = 'domain-events'
    AND event_type = 'worker.synthetic'
    AND schema_version = 1
    AND aggregate_kind = 'SyntheticAggregate'
    AND octet_length(payload) = 1
    AND policy_id = 'threadline.outbox.policy/v1'
    AND policy_snapshot_digest = decode('$POLICY_DIGEST_HEX', 'hex');
"
assert_sql_equals "claim token was not fresh, 32-byte, raw-only authority" "4|4|4" "
  SELECT
    count(*) FILTER (WHERE octet_length(raw_claim_token) = 32) || '|' ||
    count(DISTINCT encode(raw_claim_token, 'hex')) || '|' ||
    count(*) FILTER (
      WHERE domain.transactional_outbox_claim_digest(raw_claim_token) = attempt.claim_token_digest
    )
  FROM domain.worker_ops_order_claims AS claimed
  JOIN domain.outbox_delivery_attempts AS attempt
    USING (tenant_id, event_id, outbox_entry_id, delivery_attempt_id, replay_generation);
"
assert_sql_equals "claim result exposed stored claim-token digest" "f" "
  SELECT pg_get_function_result(
    'domain.claim_transactional_outbox_batch(text,integer)'::regprocedure
  ) ~ 'claim_token_digest';
"
assert_sql_equals "persistent tables gained a raw claim-token column" "0" "
  SELECT count(*)
  FROM information_schema.columns
  WHERE table_schema = 'domain'
    AND table_name IN (
      'domain_events', 'transactional_outbox', 'outbox_delivery_attempts'
    )
    AND column_name IN ('raw_claim_token', 'claim_token');
"

# Batch boundaries share one large deterministic fixture so the sum of the
# exact returned row counts also proves no call exceeded its accepted batch.
psql_test --command="
  INSERT INTO domain.domain_events (
    tenant_id, event_id, event_type, schema_version,
    aggregate_kind, aggregate_id, payload, occurred_at, enqueued_at
  )
  SELECT
    'tenant-worker-alpha-synthetic',
    'event-batch-' || lpad(ordinal::text, 3, '0') || '-synthetic',
    'worker.synthetic', 1, 'SyntheticAggregate',
    'event-batch-' || lpad(ordinal::text, 3, '0') || '-synthetic',
    decode('', 'hex'), '2026-01-01 00:00:00+00',
    '2026-01-02 00:00:00+00'::timestamptz + ordinal * interval '1 microsecond'
  FROM generate_series(1, 322) AS ordinal;

  INSERT INTO domain.transactional_outbox (
    tenant_id, event_id, destination, next_attempt_at,
    policy_id, policy_snapshot_digest,
    effective_lease_ms, effective_absolute_lifetime_ms,
    effective_event_retry_ceiling,
    effective_transport_base_ms, effective_transport_cap_ms,
    effective_unknown_base_ms, effective_unknown_cap_ms,
    effective_event_base_ms, effective_event_cap_ms,
    effective_retention_days
  )
  SELECT
    tenant_id, event_id, 'domain-events', '2025-01-01 00:00:00+00',
    'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'),
    30000, 300000, 8, 1000, 60000, 5000, 300000, 5000, 300000, 90
  FROM domain.domain_events
  WHERE tenant_id = 'tenant-worker-alpha-synthetic'
    AND event_id LIKE 'event-batch-%-synthetic';

  CREATE TABLE domain.worker_ops_batch_utf8 AS
  SELECT * FROM domain.claim_transactional_outbox_batch(repeat('é', 64), 1);
  CREATE TABLE domain.worker_ops_batch_1 AS
  SELECT * FROM domain.claim_transactional_outbox_batch(repeat('a', 128), 1);
  CREATE TABLE domain.worker_ops_batch_64 AS
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-batch-64-synthetic', 64);
  CREATE TABLE domain.worker_ops_batch_256 AS
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-batch-256-synthetic', 256);
"
assert_sql_equals "claim batch boundaries returned the wrong counts" "1|1|64|256" "
  SELECT
    (SELECT count(*) FROM domain.worker_ops_batch_utf8) || '|' ||
    (SELECT count(*) FROM domain.worker_ops_batch_1) || '|' ||
    (SELECT count(*) FROM domain.worker_ops_batch_64) || '|' ||
    (SELECT count(*) FROM domain.worker_ops_batch_256);
"

attempt_count_before_invalid=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.outbox_delivery_attempts;
")
expect_invalid_input "zero claim batch" "
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-invalid-synthetic', 0);
"
expect_invalid_input "negative claim batch" "
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-invalid-synthetic', -1);
"
expect_invalid_input "claim batch above 256" "
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-invalid-synthetic', 257);
"
expect_invalid_input "claim batch integer maximum" "
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-invalid-synthetic', 2147483647);
"
expect_invalid_input "NULL claim batch" "
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-invalid-synthetic', NULL);
"
expect_invalid_input "blank claim owner" "
  SELECT * FROM domain.claim_transactional_outbox_batch('', 1);
"
expect_invalid_input "trim-variant claim owner" "
  SELECT * FROM domain.claim_transactional_outbox_batch(' worker-invalid-synthetic', 1);
"
expect_invalid_input "129-octet claim owner" "
  SELECT * FROM domain.claim_transactional_outbox_batch(repeat('a', 129), 1);
"
attempt_count_after_invalid=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.outbox_delivery_attempts;
")
test "$attempt_count_after_invalid" = "$attempt_count_before_invalid" || postgres_test_fail "invalid ClaimBatch wrote an Attempt"

insert_pending_fixture() {
  fixture_tenant_id=$1
  fixture_event_id=$2
  fixture_lease_ms=$3
  fixture_absolute_ms=$4
  fixture_retry_ceiling=$5
  psql_test --command="
    INSERT INTO domain.domain_events (
      tenant_id, event_id, event_type, schema_version,
      aggregate_kind, aggregate_id, payload, occurred_at
    ) VALUES (
      '$fixture_tenant_id', '$fixture_event_id', 'worker.synthetic', 1,
      'SyntheticAggregate', '$fixture_event_id', decode('00ff80', 'hex'),
      '2026-01-01 00:00:00+00'
    );
    INSERT INTO domain.transactional_outbox (
      tenant_id, event_id, destination, next_attempt_at,
      policy_id, policy_snapshot_digest,
      effective_lease_ms, effective_absolute_lifetime_ms,
      effective_event_retry_ceiling,
      effective_transport_base_ms, effective_transport_cap_ms,
      effective_unknown_base_ms, effective_unknown_cap_ms,
      effective_event_base_ms, effective_event_cap_ms,
      effective_retention_days
    ) VALUES (
      '$fixture_tenant_id', '$fixture_event_id', 'domain-events',
      clock_timestamp() - interval '1 second',
      'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'),
      $fixture_lease_ms, $fixture_absolute_ms, $fixture_retry_ceiling,
      1000, 60000, 5000, 300000, 5000, 300000, 90
    );
  "
}

# Two independent sessions claim the same 64-row pool. Exact union and
# intersection checks catch duplicates even when one process wins scheduling.
psql_test --command="
  INSERT INTO domain.domain_events (
    tenant_id, event_id, event_type, schema_version,
    aggregate_kind, aggregate_id, payload, occurred_at, enqueued_at
  )
  SELECT
    'tenant-worker-beta-synthetic',
    'event-concurrent-' || lpad(ordinal::text, 3, '0') || '-synthetic',
    'worker.synthetic', 1, 'SyntheticAggregate',
    'event-concurrent-' || lpad(ordinal::text, 3, '0') || '-synthetic',
    decode('', 'hex'), '2026-01-01 00:00:00+00',
    '2026-01-03 00:00:00+00'::timestamptz + ordinal * interval '1 microsecond'
  FROM generate_series(1, 64) AS ordinal;
  INSERT INTO domain.transactional_outbox (
    tenant_id, event_id, destination, next_attempt_at,
    policy_id, policy_snapshot_digest,
    effective_lease_ms, effective_absolute_lifetime_ms,
    effective_event_retry_ceiling,
    effective_transport_base_ms, effective_transport_cap_ms,
    effective_unknown_base_ms, effective_unknown_cap_ms,
    effective_event_base_ms, effective_event_cap_ms,
    effective_retention_days
  )
  SELECT
    tenant_id, event_id, 'domain-events', clock_timestamp() - interval '1 second',
    'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'),
    120000, 900000, 8, 1000, 60000, 5000, 300000, 5000, 300000, 90
  FROM domain.domain_events
  WHERE tenant_id = 'tenant-worker-beta-synthetic'
    AND event_id LIKE 'event-concurrent-%-synthetic';
  CREATE TABLE domain.worker_ops_concurrent_a AS
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-concurrent-template', 1)
  WITH NO DATA;
  CREATE TABLE domain.worker_ops_concurrent_b AS
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-concurrent-template', 1)
  WITH NO DATA;
"

psql_test --command="
  INSERT INTO domain.worker_ops_concurrent_a
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-concurrent-a-synthetic', 32);
" >"$temp_dir/concurrent-a.out" 2>"$temp_dir/concurrent-a.err" &
concurrent_a_pid=$!
psql_test --command="
  INSERT INTO domain.worker_ops_concurrent_b
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-concurrent-b-synthetic', 32);
" >"$temp_dir/concurrent-b.out" 2>"$temp_dir/concurrent-b.err" &
concurrent_b_pid=$!
if ! wait "$concurrent_a_pid"; then
  postgres_test_fail "first concurrent ClaimBatch session failed"
fi
if ! wait "$concurrent_b_pid"; then
  postgres_test_fail "second concurrent ClaimBatch session failed"
fi
assert_sql_equals "concurrent claimers did not receive exactly 64 unique Entries" "64|64|0" "
  SELECT
    (SELECT count(*) FROM (
      SELECT outbox_entry_id FROM domain.worker_ops_concurrent_a
      UNION ALL
      SELECT outbox_entry_id FROM domain.worker_ops_concurrent_b
    ) AS claimed) || '|' ||
    (SELECT count(DISTINCT outbox_entry_id) FROM (
      SELECT outbox_entry_id FROM domain.worker_ops_concurrent_a
      UNION ALL
      SELECT outbox_entry_id FROM domain.worker_ops_concurrent_b
    ) AS claimed) || '|' ||
    (SELECT count(*)
     FROM domain.worker_ops_concurrent_a AS left_claim
     JOIN domain.worker_ops_concurrent_b AS right_claim USING (outbox_entry_id));
"
assert_sql_equals "concurrent claim returned the wrong owner" "64" "
  SELECT count(*)
  FROM (
    SELECT claim_owner_id, 'worker-concurrent-a-synthetic' AS expected_owner
    FROM domain.worker_ops_concurrent_a
    UNION ALL
    SELECT claim_owner_id, 'worker-concurrent-b-synthetic' AS expected_owner
    FROM domain.worker_ops_concurrent_b
  ) AS claims
  WHERE claim_owner_id = expected_owner;
"

# Replacement closes an expired Attempt and creates its successor in the same
# transaction, without borrowing any failure counter or scheduling backoff.
insert_pending_fixture tenant-worker-alpha-synthetic event-expired-synthetic 5000 30000 8
psql_test --command="
  DO \$fixture\$
  DECLARE
    v_entry_id bigint;
    v_attempt_id bigint;
    v_now timestamptz := clock_timestamp();
  BEGIN
    SELECT outbox_entry_id INTO v_entry_id
    FROM domain.transactional_outbox
    WHERE tenant_id = 'tenant-worker-alpha-synthetic'
      AND event_id = 'event-expired-synthetic';
    INSERT INTO domain.outbox_delivery_attempts (
      tenant_id, event_id, outbox_entry_id, replay_generation,
      total_attempt_number, generation_attempt_number,
      claim_owner_id, claim_token_digest,
      claimed_at, lease_expires_at, absolute_lease_expires_at,
      broker_message_id, policy_id, policy_snapshot_digest,
      effective_lease_ms, effective_retention_days
    ) VALUES (
      'tenant-worker-alpha-synthetic', 'event-expired-synthetic', v_entry_id, 0,
      1, 1, 'worker-expired-old-synthetic',
      domain.transactional_outbox_claim_digest(decode('$TOKEN_RAW_HEX', 'hex')),
      v_now - interval '40 seconds', v_now - interval '35 seconds',
      v_now - interval '10 seconds',
      domain.transactional_outbox_message_id(
        'tenant-worker-alpha-synthetic', 'event-expired-synthetic', 'domain-events', 0
      ),
      'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'), 5000, 90
    ) RETURNING delivery_attempt_id INTO v_attempt_id;
    UPDATE domain.transactional_outbox
    SET delivery_state = 'claimed', total_attempt_count = 1,
        generation_attempt_count = 1, next_attempt_at = NULL,
        current_attempt_id = v_attempt_id
    WHERE outbox_entry_id = v_entry_id;
  END
  \$fixture\$;
  CREATE TABLE domain.worker_ops_expiry_replacement AS
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-expired-new-synthetic', 1);
"
assert_sql_equals "expired claim replacement was not atomic and counter-neutral" "claimed|lease_expired|2|2|0|0|0|1" "
  SELECT
    replacement.result_code || '|' || old_attempt.outcome || '|' ||
    entry.total_attempt_count || '|' || entry.generation_attempt_count || '|' ||
    entry.generation_transport_failure_count || '|' ||
    entry.generation_unknown_outcome_count || '|' ||
    entry.generation_failure_count || '|' ||
    count(*) FILTER (WHERE all_attempts.outcome = 'active')
  FROM domain.worker_ops_expiry_replacement AS replacement
  JOIN domain.transactional_outbox AS entry USING (outbox_entry_id)
  JOIN domain.outbox_delivery_attempts AS old_attempt
    ON old_attempt.outbox_entry_id = entry.outbox_entry_id
   AND old_attempt.delivery_attempt_id <> replacement.delivery_attempt_id
  JOIN domain.outbox_delivery_attempts AS all_attempts
    ON all_attempts.outbox_entry_id = entry.outbox_entry_id
  GROUP BY replacement.result_code, old_attempt.outcome,
    entry.total_attempt_count, entry.generation_attempt_count,
    entry.generation_transport_failure_count,
    entry.generation_unknown_outcome_count, entry.generation_failure_count;
"
assert_sql_equals "expired Attempt retention timestamp was not exact" "t" "
  SELECT bool_and(
    evidence_not_before = finished_at + 90::bigint * 86400 * interval '1 second'
  )
  FROM domain.outbox_delivery_attempts
  WHERE event_id = 'event-expired-synthetic'
    AND outcome = 'lease_expired';
"
psql_test --command="
  WITH expired AS (
    UPDATE domain.outbox_delivery_attempts
    SET outcome = 'lease_expired',
        finished_at = clock_timestamp(),
        evidence_not_before = clock_timestamp()
          + effective_retention_days::bigint * 86400 * interval '1 second'
    WHERE event_id = 'event-expired-synthetic'
      AND outcome = 'active'
    RETURNING delivery_attempt_id
  )
  UPDATE domain.transactional_outbox
  SET delivery_state = 'pending',
      next_attempt_at = clock_timestamp() + interval '1 day',
      current_attempt_id = NULL
  WHERE event_id = 'event-expired-synthetic'
    AND current_attempt_id IN (SELECT delivery_attempt_id FROM expired);
"

# Renew uses database time, is strictly monotonic, returns the stored lease, and
# never crosses the immutable absolute cap.
insert_pending_fixture tenant-worker-alpha-synthetic event-renew-synthetic 120000 900000 8
psql_test --command="
  CREATE TABLE domain.worker_ops_renew_claim AS
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-renew-synthetic', 1);
  CREATE TABLE domain.worker_ops_renew_result AS
  SELECT renewed.*
  FROM domain.worker_ops_renew_claim AS claim
  CROSS JOIN LATERAL domain.renew_transactional_outbox_claim(
    claim.tenant_id, claim.event_id, claim.outbox_entry_id,
    claim.delivery_attempt_id, claim.replay_generation, claim.claim_owner_id,
    domain.transactional_outbox_claim_digest(claim.raw_claim_token)
  ) AS renewed;
"
assert_sql_equals "RenewClaim did not use DB time or monotonically persist its result" "renewed|true|true|true" "
  SELECT
    renewed.result_code || '|' ||
    (renewed.lease_expires_at = attempt.lease_expires_at) || '|' ||
    (renewed.lease_expires_at > claim.lease_expires_at) || '|' ||
    (renewed.lease_expires_at <= claim.absolute_lease_expires_at)
  FROM domain.worker_ops_renew_claim AS claim
  JOIN domain.worker_ops_renew_result AS renewed ON true
  JOIN domain.outbox_delivery_attempts AS attempt
    ON attempt.delivery_attempt_id = claim.delivery_attempt_id;
"

# Every authority component is independently fenced. All secret, missing, and
# stale variants collapse to claim-denied and leave both rows byte-for-byte
# unchanged.
insert_pending_fixture tenant-worker-alpha-synthetic event-fence-synthetic 120000 900000 8
psql_test --command="
  CREATE TABLE domain.worker_ops_fence_claim AS
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-fence-synthetic', 1);
"
fence_snapshot_before=$(psql_test --tuples-only --no-align --command="
  SELECT md5(to_jsonb(entry)::text || to_jsonb(attempt)::text)
  FROM domain.worker_ops_fence_claim AS claim
  JOIN domain.transactional_outbox AS entry USING (outbox_entry_id)
  JOIN domain.outbox_delivery_attempts AS attempt USING (delivery_attempt_id);
")
assert_sql_equals "exact Claim fencing did not collapse all mismatches" "claim-denied,claim-denied,claim-denied,claim-denied,claim-denied,claim-denied,claim-denied" "
  WITH claim AS (
    SELECT * FROM domain.worker_ops_fence_claim
  ), denials AS (
    SELECT 1 AS ordinal, renewed.result_code
    FROM claim CROSS JOIN LATERAL domain.renew_transactional_outbox_claim(
      'tenant-worker-beta-synthetic', event_id, outbox_entry_id,
      delivery_attempt_id, replay_generation, claim_owner_id,
      domain.transactional_outbox_claim_digest(raw_claim_token)
    ) AS renewed
    UNION ALL
    SELECT 2, renewed.result_code
    FROM claim CROSS JOIN LATERAL domain.renew_transactional_outbox_claim(
      tenant_id, 'event-order-a-synthetic', outbox_entry_id,
      delivery_attempt_id, replay_generation, claim_owner_id,
      domain.transactional_outbox_claim_digest(raw_claim_token)
    ) AS renewed
    UNION ALL
    SELECT 3, renewed.result_code
    FROM claim CROSS JOIN LATERAL domain.renew_transactional_outbox_claim(
      tenant_id, event_id, outbox_entry_id + 1,
      delivery_attempt_id, replay_generation, claim_owner_id,
      domain.transactional_outbox_claim_digest(raw_claim_token)
    ) AS renewed
    UNION ALL
    SELECT 4, renewed.result_code
    FROM claim CROSS JOIN LATERAL domain.renew_transactional_outbox_claim(
      tenant_id, event_id, outbox_entry_id,
      delivery_attempt_id + 1, replay_generation, claim_owner_id,
      domain.transactional_outbox_claim_digest(raw_claim_token)
    ) AS renewed
    UNION ALL
    SELECT 5, renewed.result_code
    FROM claim CROSS JOIN LATERAL domain.renew_transactional_outbox_claim(
      tenant_id, event_id, outbox_entry_id,
      delivery_attempt_id, replay_generation + 1, claim_owner_id,
      domain.transactional_outbox_claim_digest(raw_claim_token)
    ) AS renewed
    UNION ALL
    SELECT 6, renewed.result_code
    FROM claim CROSS JOIN LATERAL domain.renew_transactional_outbox_claim(
      tenant_id, event_id, outbox_entry_id,
      delivery_attempt_id, replay_generation, 'worker-other-synthetic',
      domain.transactional_outbox_claim_digest(raw_claim_token)
    ) AS renewed
    UNION ALL
    SELECT 7, renewed.result_code
    FROM claim CROSS JOIN LATERAL domain.renew_transactional_outbox_claim(
      tenant_id, event_id, outbox_entry_id,
      delivery_attempt_id, replay_generation, claim_owner_id,
      decode(repeat('00', 32), 'hex')
    ) AS renewed
  )
  SELECT string_agg(result_code, ',' ORDER BY ordinal) FROM denials;
"
fence_snapshot_after=$(psql_test --tuples-only --no-align --command="
  SELECT md5(to_jsonb(entry)::text || to_jsonb(attempt)::text)
  FROM domain.worker_ops_fence_claim AS claim
  JOIN domain.transactional_outbox AS entry USING (outbox_entry_id)
  JOIN domain.outbox_delivery_attempts AS attempt USING (delivery_attempt_id);
")
test "$fence_snapshot_after" = "$fence_snapshot_before" || postgres_test_fail "denied exact Claim tuple mutated state"

# A near-cap synthetic Attempt proves exact cap clamping without sleeping. The
# second renewal is denied because equality at the cap is not an extension.
insert_pending_fixture tenant-worker-alpha-synthetic event-renew-cap-synthetic 5000 30000 8
psql_test --command="
  DO \$fixture\$
  DECLARE
    v_entry_id bigint;
    v_attempt_id bigint;
    v_now timestamptz := clock_timestamp();
  BEGIN
    SELECT outbox_entry_id INTO v_entry_id
    FROM domain.transactional_outbox
    WHERE event_id = 'event-renew-cap-synthetic';
    INSERT INTO domain.outbox_delivery_attempts (
      tenant_id, event_id, outbox_entry_id, replay_generation,
      total_attempt_number, generation_attempt_number,
      claim_owner_id, claim_token_digest,
      claimed_at, lease_expires_at, absolute_lease_expires_at,
      broker_message_id, policy_id, policy_snapshot_digest,
      effective_lease_ms, effective_retention_days
    ) VALUES (
      'tenant-worker-alpha-synthetic', 'event-renew-cap-synthetic', v_entry_id, 0,
      1, 1, 'worker-renew-cap-synthetic',
      domain.transactional_outbox_claim_digest(decode('$TOKEN_RAW_HEX', 'hex')),
      v_now - interval '20 seconds', v_now + interval '1 second',
      v_now + interval '2 seconds',
      domain.transactional_outbox_message_id(
        'tenant-worker-alpha-synthetic', 'event-renew-cap-synthetic', 'domain-events', 0
      ),
      'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'), 5000, 90
    ) RETURNING delivery_attempt_id INTO v_attempt_id;
    UPDATE domain.transactional_outbox
    SET delivery_state = 'claimed', total_attempt_count = 1,
        generation_attempt_count = 1, next_attempt_at = NULL,
        current_attempt_id = v_attempt_id
    WHERE outbox_entry_id = v_entry_id;
  END
  \$fixture\$;
  CREATE TABLE domain.worker_ops_renew_cap_first AS
  SELECT renewed.*
  FROM domain.transactional_outbox AS entry
  JOIN domain.outbox_delivery_attempts AS attempt
    ON attempt.delivery_attempt_id = entry.current_attempt_id
  CROSS JOIN LATERAL domain.renew_transactional_outbox_claim(
    entry.tenant_id, entry.event_id, entry.outbox_entry_id,
    attempt.delivery_attempt_id, entry.replay_generation,
    attempt.claim_owner_id, decode('$TOKEN_DIGEST_HEX', 'hex')
  ) AS renewed
  WHERE entry.event_id = 'event-renew-cap-synthetic';
"
assert_sql_equals "RenewClaim did not clamp exactly to absolute lifetime" "renewed|true" "
  SELECT renewed.result_code || '|' ||
    (renewed.lease_expires_at = attempt.absolute_lease_expires_at)
  FROM domain.worker_ops_renew_cap_first AS renewed
  JOIN domain.transactional_outbox AS entry
    ON entry.event_id = 'event-renew-cap-synthetic'
  JOIN domain.outbox_delivery_attempts AS attempt
    ON attempt.delivery_attempt_id = entry.current_attempt_id;
"
assert_sql_equals "RenewClaim accepted equality at the absolute cap" "claim-denied|true" "
  SELECT renewed.result_code || '|' || (renewed.lease_expires_at IS NULL)
  FROM domain.transactional_outbox AS entry
  JOIN domain.outbox_delivery_attempts AS attempt
    ON attempt.delivery_attempt_id = entry.current_attempt_id
  CROSS JOIN LATERAL domain.renew_transactional_outbox_claim(
    entry.tenant_id, entry.event_id, entry.outbox_entry_id,
    attempt.delivery_attempt_id, entry.replay_generation,
    attempt.claim_owner_id, decode('$TOKEN_DIGEST_HEX', 'hex')
  ) AS renewed
  WHERE entry.event_id = 'event-renew-cap-synthetic';
"
psql_test --command="
  WITH expired AS (
    UPDATE domain.outbox_delivery_attempts
    SET outcome = 'lease_expired',
        finished_at = clock_timestamp(),
        evidence_not_before = clock_timestamp()
          + effective_retention_days::bigint * 86400 * interval '1 second'
    WHERE event_id = 'event-renew-cap-synthetic'
      AND outcome = 'active'
    RETURNING delivery_attempt_id
  )
  UPDATE domain.transactional_outbox
  SET delivery_state = 'pending',
      next_attempt_at = clock_timestamp() + interval '1 day',
      current_attempt_id = NULL
  WHERE event_id = 'event-renew-cap-synthetic'
    AND current_attempt_id IN (SELECT delivery_attempt_id FROM expired);
"

# Each claim-authority mutation must evaluate database time after waiting for
# the exact Entry lock. Three independent waiters prove Renew, ACK, and Failure
# each enter before expiry, block on that lock, and deny after the lease expires.
insert_pending_fixture tenant-worker-alpha-synthetic event-lock-expiry-renew-synthetic 5000 30000 8
insert_pending_fixture tenant-worker-alpha-synthetic event-lock-expiry-ack-synthetic 5000 30000 8
insert_pending_fixture tenant-worker-alpha-synthetic event-lock-expiry-failure-synthetic 5000 30000 8
psql_test --command="
  DO \$fixture\$
  DECLARE
    v_event_id text;
    v_entry_id bigint;
    v_attempt_id bigint;
    v_claimed_at timestamptz;
  BEGIN
    FOREACH v_event_id IN ARRAY ARRAY[
      'event-lock-expiry-renew-synthetic',
      'event-lock-expiry-ack-synthetic',
      'event-lock-expiry-failure-synthetic'
    ] LOOP
      v_claimed_at := clock_timestamp();
      SELECT outbox_entry_id INTO v_entry_id
      FROM domain.transactional_outbox
      WHERE event_id = v_event_id;
      INSERT INTO domain.outbox_delivery_attempts (
        tenant_id, event_id, outbox_entry_id, replay_generation,
        total_attempt_number, generation_attempt_number,
        claim_owner_id, claim_token_digest,
        claimed_at, lease_expires_at, absolute_lease_expires_at,
        broker_message_id, policy_id, policy_snapshot_digest,
        effective_lease_ms, effective_retention_days
      ) VALUES (
        'tenant-worker-alpha-synthetic', v_event_id, v_entry_id, 0,
        1, 1, 'worker-lock-expiry-synthetic',
        domain.transactional_outbox_claim_digest(decode('$TOKEN_RAW_HEX', 'hex')),
        v_claimed_at, v_claimed_at + interval '2 seconds',
        v_claimed_at + interval '30 seconds',
        domain.transactional_outbox_message_id(
          'tenant-worker-alpha-synthetic', v_event_id, 'domain-events', 0
        ),
        'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'), 5000, 90
      ) RETURNING delivery_attempt_id INTO v_attempt_id;
      UPDATE domain.transactional_outbox
      SET delivery_state = 'claimed', total_attempt_count = 1,
          generation_attempt_count = 1, next_attempt_at = NULL,
          current_attempt_id = v_attempt_id
      WHERE outbox_entry_id = v_entry_id;
    END LOOP;
  END
  \$fixture\$;
"
lock_expiry_snapshot_before=$(psql_test --tuples-only --no-align --command="
  SELECT md5(string_agg(to_jsonb(entry)::text || to_jsonb(attempt)::text, '|' ORDER BY entry.event_id))
  FROM domain.transactional_outbox AS entry
  JOIN domain.outbox_delivery_attempts AS attempt
    ON attempt.delivery_attempt_id = entry.current_attempt_id
  WHERE entry.event_id LIKE 'event-lock-expiry-%-synthetic';
")
lock_expiry_marker="$temp_dir/entry-lock-held"
psql_test \
  --command="BEGIN" \
  --command="SELECT 1 FROM domain.transactional_outbox WHERE event_id LIKE 'event-lock-expiry-%-synthetic' ORDER BY event_id FOR UPDATE" \
  --command="\\! touch '$lock_expiry_marker'" \
  --command="SELECT pg_sleep(GREATEST(0, EXTRACT(EPOCH FROM (MIN(attempt.lease_expires_at) - clock_timestamp())) + 0.25)) FROM domain.outbox_delivery_attempts AS attempt WHERE attempt.event_id LIKE 'event-lock-expiry-%-synthetic'" \
  --command="COMMIT" \
  >"$temp_dir/lock-holder.out" 2>"$temp_dir/lock-holder.err" &
lock_holder_pid=$!
lock_wait_tries=0
while ! test -f "$lock_expiry_marker"; do
  if ! kill -0 "$lock_holder_pid" 2>/dev/null; then
    wait "$lock_holder_pid" || true
    postgres_test_fail "lease-expiry lock holder exited before signaling"
  fi
  lock_wait_tries=$((lock_wait_tries + 1))
  test "$lock_wait_tries" -le 100 || postgres_test_fail "lease-expiry lock holder did not signal"
  sleep 0.02
done
assert_sql_equals "lock-expiry waiters did not start with live leases" "t" "
  SELECT bool_and(clock_timestamp() < attempt.lease_expires_at)
  FROM domain.outbox_delivery_attempts AS attempt
  WHERE attempt.event_id LIKE 'event-lock-expiry-%-synthetic';
"
(
  PGAPPNAME=threadline_lock_expiry_renew
  export PGAPPNAME
  psql_test --tuples-only --no-align --command="
    SELECT renewed.result_code
    FROM domain.transactional_outbox AS entry
    JOIN domain.outbox_delivery_attempts AS attempt
      ON attempt.delivery_attempt_id = entry.current_attempt_id
    CROSS JOIN LATERAL domain.renew_transactional_outbox_claim(
      entry.tenant_id, entry.event_id, entry.outbox_entry_id,
      attempt.delivery_attempt_id, entry.replay_generation,
      attempt.claim_owner_id, decode('$TOKEN_DIGEST_HEX', 'hex')
    ) AS renewed
    WHERE entry.event_id = 'event-lock-expiry-renew-synthetic';
  "
) >"$temp_dir/lock-renew.out" 2>"$temp_dir/lock-renew.err" &
lock_renew_pid=$!
(
  PGAPPNAME=threadline_lock_expiry_ack
  export PGAPPNAME
  psql_test --tuples-only --no-align --command="
    SELECT domain.acknowledge_transactional_outbox_published(
      entry.tenant_id, entry.event_id, entry.outbox_entry_id,
      attempt.delivery_attempt_id, entry.replay_generation,
      attempt.claim_owner_id, decode('$TOKEN_DIGEST_HEX', 'hex'),
      'DOMAIN_EVENTS', 1, false, attempt.broker_message_id
    )
    FROM domain.transactional_outbox AS entry
    JOIN domain.outbox_delivery_attempts AS attempt
      ON attempt.delivery_attempt_id = entry.current_attempt_id
    WHERE entry.event_id = 'event-lock-expiry-ack-synthetic';
  "
) >"$temp_dir/lock-ack.out" 2>"$temp_dir/lock-ack.err" &
lock_ack_pid=$!
(
  PGAPPNAME=threadline_lock_expiry_failure
  export PGAPPNAME
  psql_test --tuples-only --no-align --command="
    SELECT failed.result_code
    FROM domain.transactional_outbox AS entry
    JOIN domain.outbox_delivery_attempts AS attempt
      ON attempt.delivery_attempt_id = entry.current_attempt_id
    CROSS JOIN LATERAL domain.record_transactional_outbox_publish_failure(
      entry.tenant_id, entry.event_id, entry.outbox_entry_id,
      attempt.delivery_attempt_id, entry.replay_generation,
      attempt.claim_owner_id, decode('$TOKEN_DIGEST_HEX', 'hex'),
      'transport-unavailable'
    ) AS failed
    WHERE entry.event_id = 'event-lock-expiry-failure-synthetic';
  "
) >"$temp_dir/lock-failure.out" 2>"$temp_dir/lock-failure.err" &
lock_failure_pid=$!

lock_wait_tries=0
while test "$(psql_test --tuples-only --no-align --command="
  SELECT count(*)
  FROM pg_stat_activity
  WHERE application_name IN (
    'threadline_lock_expiry_renew',
    'threadline_lock_expiry_ack',
    'threadline_lock_expiry_failure'
  )
    AND wait_event_type = 'Lock';
")" != "3"; do
  lock_wait_tries=$((lock_wait_tries + 1))
  test "$lock_wait_tries" -le 200 || postgres_test_fail "claim-authority waiters did not all block on the Entry locks"
  sleep 0.02
done
assert_sql_equals "claim-authority waiters were not all blocked before lease expiry" "t" "
  SELECT bool_and(clock_timestamp() < attempt.lease_expires_at)
  FROM domain.outbox_delivery_attempts AS attempt
  WHERE attempt.event_id LIKE 'event-lock-expiry-%-synthetic';
"
if ! wait "$lock_holder_pid"; then
  lock_holder_error=$(tr '\n' ' ' <"$temp_dir/lock-holder.err")
  postgres_test_fail "lease-expiry lock holder failed: $lock_holder_error"
fi
if ! wait "$lock_renew_pid" || ! wait "$lock_ack_pid" || ! wait "$lock_failure_pid"; then
  lock_waiter_error=$(tr '\n' ' ' <"$temp_dir/lock-renew.err"; tr '\n' ' ' <"$temp_dir/lock-ack.err"; tr '\n' ' ' <"$temp_dir/lock-failure.err")
  postgres_test_fail "claim-authority lock waiter failed: $lock_waiter_error"
fi
test "$(tr -d '[:space:]' <"$temp_dir/lock-renew.out")" = "claim-denied" || postgres_test_fail "Renew retained stale authority across lock wait"
test "$(tr -d '[:space:]' <"$temp_dir/lock-ack.out")" = "claim-denied" || postgres_test_fail "ACK retained stale authority across lock wait"
test "$(tr -d '[:space:]' <"$temp_dir/lock-failure.out")" = "claim-denied" || postgres_test_fail "Failure retained stale authority across lock wait"
lock_expiry_snapshot_after=$(psql_test --tuples-only --no-align --command="
  SELECT md5(string_agg(to_jsonb(entry)::text || to_jsonb(attempt)::text, '|' ORDER BY entry.event_id))
  FROM domain.transactional_outbox AS entry
  JOIN domain.outbox_delivery_attempts AS attempt
    ON attempt.delivery_attempt_id = entry.current_attempt_id
  WHERE entry.event_id LIKE 'event-lock-expiry-%-synthetic';
")
test "$lock_expiry_snapshot_after" = "$lock_expiry_snapshot_before" || postgres_test_fail "expired lock wait mutated state"
psql_test --command="
  DO \$cleanup\$
  DECLARE
    v_now timestamptz := clock_timestamp();
  BEGIN
    UPDATE domain.outbox_delivery_attempts
    SET outcome = 'lease_expired',
        finished_at = v_now,
        evidence_not_before = v_now
          + effective_retention_days::bigint * 86400 * interval '1 second'
    WHERE event_id LIKE 'event-lock-expiry-%-synthetic'
      AND outcome = 'active';
    UPDATE domain.transactional_outbox
    SET delivery_state = 'pending',
        next_attempt_at = v_now + interval '1 day',
        current_attempt_id = NULL
    WHERE event_id LIKE 'event-lock-expiry-%-synthetic';
  END
  \$cleanup\$;
"

# ACK accepts normalized evidence at both numeric and stream-width boundaries,
# closes Entry+Attempt together, and permits only an exact non-mutating repeat.
insert_pending_fixture tenant-worker-alpha-synthetic event-ack-synthetic 120000 900000 8
psql_test --command="
  CREATE TABLE domain.worker_ops_ack_claim AS
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-ack-synthetic', 1);
  CREATE TABLE domain.worker_ops_ack_first AS
  SELECT domain.acknowledge_transactional_outbox_published(
    claim.tenant_id, claim.event_id, claim.outbox_entry_id,
    claim.delivery_attempt_id, claim.replay_generation, claim.claim_owner_id,
    domain.transactional_outbox_claim_digest(claim.raw_claim_token),
    repeat('S', 255), 18446744073709551615, false, claim.broker_message_id
  ) AS result_code
  FROM domain.worker_ops_ack_claim AS claim;
"
assert_sql_equals "first ACK did not atomically deliver exact evidence" "delivered|delivered|delivered|true|true|true|true" "
  SELECT
    acknowledged.result_code || '|' || entry.delivery_state || '|' ||
    attempt.outcome || '|' || (entry.current_attempt_id IS NULL) || '|' ||
    (octet_length(attempt.broker_stream) = 255) || '|' ||
    (attempt.broker_sequence = 18446744073709551615) || '|' ||
    (attempt.broker_duplicate = false)
  FROM domain.worker_ops_ack_first AS acknowledged
  JOIN domain.worker_ops_ack_claim AS claim ON true
  JOIN domain.transactional_outbox AS entry USING (outbox_entry_id)
  JOIN domain.outbox_delivery_attempts AS attempt USING (delivery_attempt_id);
"
assert_sql_equals "delivered Attempt retention timestamp was not exact" "t" "
  SELECT attempt.evidence_not_before =
    attempt.finished_at + attempt.effective_retention_days::bigint * 86400 * interval '1 second'
  FROM domain.worker_ops_ack_claim AS claim
  JOIN domain.outbox_delivery_attempts AS attempt USING (delivery_attempt_id);
"
assert_sql_equals "exact delivered observation was not already-delivered" "already-delivered" "
  SELECT domain.acknowledge_transactional_outbox_published(
    claim.tenant_id, claim.event_id, claim.outbox_entry_id,
    claim.delivery_attempt_id, claim.replay_generation, claim.claim_owner_id,
    domain.transactional_outbox_claim_digest(claim.raw_claim_token),
    repeat('S', 255), 18446744073709551615, false, claim.broker_message_id
  )
  FROM domain.worker_ops_ack_claim AS claim;
"
assert_sql_equals "non-exact delivered observation was not denied" "claim-denied,claim-denied,claim-denied,claim-denied" "
  WITH claim AS (SELECT * FROM domain.worker_ops_ack_claim), denials AS (
    SELECT 1 AS ordinal, domain.acknowledge_transactional_outbox_published(
      tenant_id, event_id, outbox_entry_id, delivery_attempt_id,
      replay_generation, claim_owner_id,
      domain.transactional_outbox_claim_digest(raw_claim_token),
      repeat('T', 255), 18446744073709551615, false, broker_message_id
    ) AS result_code FROM claim
    UNION ALL
    SELECT 2, domain.acknowledge_transactional_outbox_published(
      tenant_id, event_id, outbox_entry_id, delivery_attempt_id,
      replay_generation, claim_owner_id,
      domain.transactional_outbox_claim_digest(raw_claim_token),
      repeat('S', 255), 18446744073709551614, false, broker_message_id
    ) FROM claim
    UNION ALL
    SELECT 3, domain.acknowledge_transactional_outbox_published(
      tenant_id, event_id, outbox_entry_id, delivery_attempt_id,
      replay_generation, claim_owner_id,
      domain.transactional_outbox_claim_digest(raw_claim_token),
      repeat('S', 255), 18446744073709551615, true, broker_message_id
    ) FROM claim
    UNION ALL
    SELECT 4, domain.acknowledge_transactional_outbox_published(
      tenant_id, event_id, outbox_entry_id, delivery_attempt_id,
      replay_generation, claim_owner_id,
      domain.transactional_outbox_claim_digest(raw_claim_token),
      repeat('S', 255), 18446744073709551615, false, repeat('0', 64)
    ) FROM claim
  )
  SELECT string_agg(result_code, ',' ORDER BY ordinal) FROM denials;
"

# Duplicate=true is valid first-transition evidence, not a failure result.
insert_pending_fixture tenant-worker-alpha-synthetic event-ack-duplicate-synthetic 120000 900000 8
psql_test --command="
  CREATE TABLE domain.worker_ops_ack_duplicate_claim AS
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-ack-duplicate-synthetic', 1);
"
assert_sql_equals "Duplicate PubAck was not accepted as delivery evidence" "delivered|already-delivered" "
  WITH first_ack AS MATERIALIZED (
    SELECT domain.acknowledge_transactional_outbox_published(
      tenant_id, event_id, outbox_entry_id, delivery_attempt_id,
      replay_generation, claim_owner_id,
      domain.transactional_outbox_claim_digest(raw_claim_token),
      'DOMAIN_EVENTS', 1, true, broker_message_id
    ) AS result_code
    FROM domain.worker_ops_ack_duplicate_claim
  ), repeated_ack AS (
    SELECT domain.acknowledge_transactional_outbox_published(
      tenant_id, event_id, outbox_entry_id, delivery_attempt_id,
      replay_generation, claim_owner_id,
      domain.transactional_outbox_claim_digest(raw_claim_token),
      'DOMAIN_EVENTS', 1, true, broker_message_id
    ) AS result_code
    FROM domain.worker_ops_ack_duplicate_claim, first_ack
  )
  SELECT first_ack.result_code || '|' || repeated_ack.result_code
  FROM first_ack, repeated_ack;
"

# Malformed PubAck fields are invalid-input before any row lock/write. A
# canonical-but-wrong message ID remains the same secret-safe claim-denied.
insert_pending_fixture tenant-worker-alpha-synthetic event-ack-invalid-synthetic 120000 900000 8
psql_test --command="
  CREATE TABLE domain.worker_ops_ack_invalid_claim AS
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-ack-invalid-synthetic', 1);
"
ack_invalid_snapshot_before=$(psql_test --tuples-only --no-align --command="
  SELECT md5(to_jsonb(entry)::text || to_jsonb(attempt)::text)
  FROM domain.worker_ops_ack_invalid_claim AS claim
  JOIN domain.transactional_outbox AS entry USING (outbox_entry_id)
  JOIN domain.outbox_delivery_attempts AS attempt USING (delivery_attempt_id);
")
expect_invalid_input "empty PubAck stream" "
  SELECT domain.acknowledge_transactional_outbox_published(
    tenant_id, event_id, outbox_entry_id, delivery_attempt_id, replay_generation,
    claim_owner_id, domain.transactional_outbox_claim_digest(raw_claim_token),
    '', 1, false, broker_message_id
  ) FROM domain.worker_ops_ack_invalid_claim;
"
expect_invalid_input "256-octet PubAck stream" "
  SELECT domain.acknowledge_transactional_outbox_published(
    tenant_id, event_id, outbox_entry_id, delivery_attempt_id, replay_generation,
    claim_owner_id, domain.transactional_outbox_claim_digest(raw_claim_token),
    repeat('S', 256), 1, false, broker_message_id
  ) FROM domain.worker_ops_ack_invalid_claim;
"
expect_invalid_input "zero PubAck sequence" "
  SELECT domain.acknowledge_transactional_outbox_published(
    tenant_id, event_id, outbox_entry_id, delivery_attempt_id, replay_generation,
    claim_owner_id, domain.transactional_outbox_claim_digest(raw_claim_token),
    'DOMAIN_EVENTS', 0, false, broker_message_id
  ) FROM domain.worker_ops_ack_invalid_claim;
"
expect_invalid_input "PubAck sequence above uint64" "
  SELECT domain.acknowledge_transactional_outbox_published(
    tenant_id, event_id, outbox_entry_id, delivery_attempt_id, replay_generation,
    claim_owner_id, domain.transactional_outbox_claim_digest(raw_claim_token),
    'DOMAIN_EVENTS', 18446744073709551616, false, broker_message_id
  ) FROM domain.worker_ops_ack_invalid_claim;
"
expect_invalid_input "fractional PubAck sequence" "
  SELECT domain.acknowledge_transactional_outbox_published(
    tenant_id, event_id, outbox_entry_id, delivery_attempt_id, replay_generation,
    claim_owner_id, domain.transactional_outbox_claim_digest(raw_claim_token),
    'DOMAIN_EVENTS', 1.5, false, broker_message_id
  ) FROM domain.worker_ops_ack_invalid_claim;
"
expect_invalid_input "NULL PubAck duplicate flag" "
  SELECT domain.acknowledge_transactional_outbox_published(
    tenant_id, event_id, outbox_entry_id, delivery_attempt_id, replay_generation,
    claim_owner_id, domain.transactional_outbox_claim_digest(raw_claim_token),
    'DOMAIN_EVENTS', 1, NULL, broker_message_id
  ) FROM domain.worker_ops_ack_invalid_claim;
"
expect_invalid_input "noncanonical PubAck message ID" "
  SELECT domain.acknowledge_transactional_outbox_published(
    tenant_id, event_id, outbox_entry_id, delivery_attempt_id, replay_generation,
    claim_owner_id, domain.transactional_outbox_claim_digest(raw_claim_token),
    'DOMAIN_EVENTS', 1, false, repeat('A', 64)
  ) FROM domain.worker_ops_ack_invalid_claim;
"
assert_sql_equals "canonical wrong PubAck message ID leaked claim detail" "claim-denied" "
  SELECT domain.acknowledge_transactional_outbox_published(
    tenant_id, event_id, outbox_entry_id, delivery_attempt_id, replay_generation,
    claim_owner_id, domain.transactional_outbox_claim_digest(raw_claim_token),
    'DOMAIN_EVENTS', 1, false, repeat('0', 64)
  ) FROM domain.worker_ops_ack_invalid_claim;
"
ack_invalid_snapshot_after=$(psql_test --tuples-only --no-align --command="
  SELECT md5(to_jsonb(entry)::text || to_jsonb(attempt)::text)
  FROM domain.worker_ops_ack_invalid_claim AS claim
  JOIN domain.transactional_outbox AS entry USING (outbox_entry_id)
  JOIN domain.outbox_delivery_attempts AS attempt USING (delivery_attempt_id);
")
test "$ack_invalid_snapshot_after" = "$ack_invalid_snapshot_before" || postgres_test_fail "invalid or denied PubAck mutated state"

assert_sql_equals "cross-Tenant ACK did not return claim-denied" "claim-denied" "
  SELECT domain.acknowledge_transactional_outbox_published(
    'tenant-worker-beta-synthetic', event_id, outbox_entry_id,
    delivery_attempt_id, replay_generation, claim_owner_id,
    domain.transactional_outbox_claim_digest(raw_claim_token),
    'DOMAIN_EVENTS', 1, false, broker_message_id
  ) FROM domain.worker_ops_ack_invalid_claim;
"
assert_sql_equals "ACK exact tuple fencing did not collapse every mismatch" "claim-denied,claim-denied,claim-denied,claim-denied,claim-denied,claim-denied,claim-denied" "
  WITH claim AS (
    SELECT * FROM domain.worker_ops_ack_invalid_claim
  ), variants AS (
    SELECT variant.*
    FROM claim
    CROSS JOIN LATERAL (
      VALUES
        (1, 'tenant-worker-beta-synthetic', event_id, outbox_entry_id,
          delivery_attempt_id, replay_generation, claim_owner_id,
          domain.transactional_outbox_claim_digest(raw_claim_token)),
        (2, tenant_id, 'event-order-a-synthetic', outbox_entry_id,
          delivery_attempt_id, replay_generation, claim_owner_id,
          domain.transactional_outbox_claim_digest(raw_claim_token)),
        (3, tenant_id, event_id, outbox_entry_id + 1,
          delivery_attempt_id, replay_generation, claim_owner_id,
          domain.transactional_outbox_claim_digest(raw_claim_token)),
        (4, tenant_id, event_id, outbox_entry_id,
          delivery_attempt_id + 1, replay_generation, claim_owner_id,
          domain.transactional_outbox_claim_digest(raw_claim_token)),
        (5, tenant_id, event_id, outbox_entry_id,
          delivery_attempt_id, replay_generation + 1, claim_owner_id,
          domain.transactional_outbox_claim_digest(raw_claim_token)),
        (6, tenant_id, event_id, outbox_entry_id,
          delivery_attempt_id, replay_generation, 'worker-other-synthetic',
          domain.transactional_outbox_claim_digest(raw_claim_token)),
        (7, tenant_id, event_id, outbox_entry_id,
          delivery_attempt_id, replay_generation, claim_owner_id,
          decode(repeat('00', 32), 'hex'))
    ) AS variant(
      ordinal, tenant_id, event_id, outbox_entry_id, delivery_attempt_id,
      replay_generation, claim_owner_id, candidate_digest
    )
  )
  SELECT string_agg(
    domain.acknowledge_transactional_outbox_published(
      tenant_id, event_id, outbox_entry_id, delivery_attempt_id,
      replay_generation, claim_owner_id, candidate_digest,
      'DOMAIN_EVENTS', 1, false,
      domain.transactional_outbox_message_id(tenant_id, event_id, 'domain-events', replay_generation)
    ),
    ',' ORDER BY ordinal
  )
  FROM variants;
"
assert_sql_equals "expired/replaced ACK did not return claim-denied" "claim-denied" "
  SELECT domain.acknowledge_transactional_outbox_published(
    attempt.tenant_id, attempt.event_id, attempt.outbox_entry_id,
    attempt.delivery_attempt_id, attempt.replay_generation,
    attempt.claim_owner_id, decode('$TOKEN_DIGEST_HEX', 'hex'),
    'DOMAIN_EVENTS', 1, false, attempt.broker_message_id
  )
  FROM domain.outbox_delivery_attempts AS attempt
  WHERE attempt.event_id = 'event-expired-synthetic'
    AND attempt.outcome = 'lease_expired'
    AND attempt.claim_owner_id = 'worker-expired-old-synthetic';
"

# All four failure classes are exercised from independent current claims. The
# assertion pins matching-only counters, exact Attempt outcomes, and DB-time
# schedules derived from the persisted secret digest without exposing it in an
# operation result.
insert_pending_fixture tenant-worker-alpha-synthetic event-failure-transport-synthetic 120000 900000 8
insert_pending_fixture tenant-worker-alpha-synthetic event-failure-unknown-synthetic 120000 900000 8
insert_pending_fixture tenant-worker-alpha-synthetic event-failure-retryable-synthetic 120000 900000 8
insert_pending_fixture tenant-worker-alpha-synthetic event-failure-permanent-synthetic 120000 900000 8
psql_test --command="
  CREATE TABLE domain.worker_ops_failure_claims AS
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-failure-synthetic', 4);
  CREATE TABLE domain.worker_ops_failure_results AS
  SELECT claim.event_id, failed.*
  FROM domain.worker_ops_failure_claims AS claim
  CROSS JOIN LATERAL domain.record_transactional_outbox_publish_failure(
    claim.tenant_id, claim.event_id, claim.outbox_entry_id,
    claim.delivery_attempt_id, claim.replay_generation, claim.claim_owner_id,
    domain.transactional_outbox_claim_digest(claim.raw_claim_token),
    CASE claim.event_id
      WHEN 'event-failure-transport-synthetic' THEN 'transport-unavailable'
      WHEN 'event-failure-unknown-synthetic' THEN 'publish-outcome-unknown'
      WHEN 'event-failure-retryable-synthetic' THEN 'event-retryable'
      WHEN 'event-failure-permanent-synthetic' THEN 'event-permanent'
    END
  ) AS failed;
"
assert_sql_equals "failure classes returned the wrong stable results" "event-failure-permanent-synthetic:parked,event-failure-retryable-synthetic:retry-scheduled,event-failure-transport-synthetic:retry-scheduled,event-failure-unknown-synthetic:retry-scheduled" "
  SELECT string_agg(event_id || ':' || result_code, ',' ORDER BY event_id)
  FROM domain.worker_ops_failure_results;
"
assert_sql_equals "failure classes changed unrelated counters or outcomes" "event-failure-permanent-synthetic|event_permanent|parked|0|0|0|event-permanent|event-failure-retryable-synthetic|event_retryable|pending|0|0|1|event-retryable|event-failure-transport-synthetic|transport_unavailable|pending|1|0|0|transport-unavailable|event-failure-unknown-synthetic|publish_outcome_unknown|pending|0|1|0|publish-outcome-unknown" "
  SELECT string_agg(
    claim.event_id || '|' || attempt.outcome || '|' || entry.delivery_state || '|' ||
    entry.generation_transport_failure_count || '|' ||
    entry.generation_unknown_outcome_count || '|' ||
    entry.generation_failure_count || '|' || entry.last_failure_code,
    '|' ORDER BY claim.event_id
  )
  FROM domain.worker_ops_failure_claims AS claim
  JOIN domain.transactional_outbox AS entry USING (outbox_entry_id)
  JOIN domain.outbox_delivery_attempts AS attempt USING (delivery_attempt_id);
"
assert_sql_equals "scheduled failures did not use exact digest/class ordinals and DB finished time" "3" "
  SELECT count(*)
  FROM domain.worker_ops_failure_claims AS claim
  JOIN domain.transactional_outbox AS entry USING (outbox_entry_id)
  JOIN domain.outbox_delivery_attempts AS attempt USING (delivery_attempt_id)
  WHERE claim.event_id <> 'event-failure-permanent-synthetic'
    AND entry.next_attempt_at = attempt.finished_at
      + domain.transactional_outbox_backoff_delay_ms(
          attempt.claim_token_digest,
          CASE claim.event_id
            WHEN 'event-failure-transport-synthetic' THEN 1::smallint
            WHEN 'event-failure-unknown-synthetic' THEN 2::smallint
            ELSE 3::smallint
          END,
          1,
          CASE claim.event_id
            WHEN 'event-failure-transport-synthetic' THEN 1000
            ELSE 5000
          END,
          CASE claim.event_id
            WHEN 'event-failure-transport-synthetic' THEN 60000
            ELSE 300000
          END
        ) * interval '1 millisecond';
"
assert_sql_equals "permanent failure consumed event budget or scheduled retry" "true|true|true" "
  SELECT
    (result.next_attempt_at IS NULL) || '|' ||
    (entry.next_attempt_at IS NULL) || '|' ||
    (entry.generation_failure_count = 0)
  FROM domain.worker_ops_failure_results AS result
  JOIN domain.worker_ops_failure_claims AS claim USING (event_id)
  JOIN domain.transactional_outbox AS entry USING (outbox_entry_id)
  WHERE result.event_id = 'event-failure-permanent-synthetic';
"
assert_sql_equals "failure retention timestamps were not exact" "t" "
  SELECT bool_and(
    attempt.evidence_not_before = attempt.finished_at
      + attempt.effective_retention_days::bigint * 86400 * interval '1 second'
  )
  FROM domain.worker_ops_failure_claims AS claim
  JOIN domain.outbox_delivery_attempts AS attempt USING (delivery_attempt_id);
"

psql_test --command="
  UPDATE domain.transactional_outbox
  SET next_attempt_at = clock_timestamp() + interval '1 day'
  WHERE event_id IN (
    'event-failure-transport-synthetic',
    'event-failure-unknown-synthetic',
    'event-failure-retryable-synthetic'
  );
"

failure_invalid_snapshot_before=$(psql_test --tuples-only --no-align --command="
  SELECT md5(to_jsonb(entry)::text || to_jsonb(attempt)::text)
  FROM domain.worker_ops_fence_claim AS claim
  JOIN domain.transactional_outbox AS entry USING (outbox_entry_id)
  JOIN domain.outbox_delivery_attempts AS attempt USING (delivery_attempt_id);
")
expect_invalid_input "unknown publish failure code" "
  SELECT failed.*
  FROM domain.worker_ops_fence_claim AS claim
  CROSS JOIN LATERAL domain.record_transactional_outbox_publish_failure(
    claim.tenant_id, claim.event_id, claim.outbox_entry_id,
    claim.delivery_attempt_id, claim.replay_generation, claim.claim_owner_id,
    domain.transactional_outbox_claim_digest(claim.raw_claim_token),
    'raw-broker-error-is-forbidden'
  ) AS failed;
"
assert_sql_equals "RecordFailure exact tuple fencing did not collapse every mismatch" "claim-denied,claim-denied,claim-denied,claim-denied,claim-denied,claim-denied,claim-denied" "
  WITH claim AS (
    SELECT * FROM domain.worker_ops_fence_claim
  ), variants AS (
    SELECT variant.*
    FROM claim
    CROSS JOIN LATERAL (
      VALUES
        (1, 'tenant-worker-beta-synthetic', event_id, outbox_entry_id,
          delivery_attempt_id, replay_generation, claim_owner_id,
          domain.transactional_outbox_claim_digest(raw_claim_token)),
        (2, tenant_id, 'event-order-a-synthetic', outbox_entry_id,
          delivery_attempt_id, replay_generation, claim_owner_id,
          domain.transactional_outbox_claim_digest(raw_claim_token)),
        (3, tenant_id, event_id, outbox_entry_id + 1,
          delivery_attempt_id, replay_generation, claim_owner_id,
          domain.transactional_outbox_claim_digest(raw_claim_token)),
        (4, tenant_id, event_id, outbox_entry_id,
          delivery_attempt_id + 1, replay_generation, claim_owner_id,
          domain.transactional_outbox_claim_digest(raw_claim_token)),
        (5, tenant_id, event_id, outbox_entry_id,
          delivery_attempt_id, replay_generation + 1, claim_owner_id,
          domain.transactional_outbox_claim_digest(raw_claim_token)),
        (6, tenant_id, event_id, outbox_entry_id,
          delivery_attempt_id, replay_generation, 'worker-other-synthetic',
          domain.transactional_outbox_claim_digest(raw_claim_token)),
        (7, tenant_id, event_id, outbox_entry_id,
          delivery_attempt_id, replay_generation, claim_owner_id,
          decode(repeat('00', 32), 'hex'))
    ) AS variant(
      ordinal, tenant_id, event_id, outbox_entry_id, delivery_attempt_id,
      replay_generation, claim_owner_id, candidate_digest
    )
  ), denials AS (
    SELECT variants.ordinal, failed.result_code
    FROM variants
    CROSS JOIN LATERAL domain.record_transactional_outbox_publish_failure(
      tenant_id, event_id, outbox_entry_id, delivery_attempt_id,
      replay_generation, claim_owner_id, candidate_digest,
      'transport-unavailable'
    ) AS failed
  )
  SELECT string_agg(result_code, ',' ORDER BY ordinal) FROM denials;
"
failure_invalid_snapshot_after=$(psql_test --tuples-only --no-align --command="
  SELECT md5(to_jsonb(entry)::text || to_jsonb(attempt)::text)
  FROM domain.worker_ops_fence_claim AS claim
  JOIN domain.transactional_outbox AS entry USING (outbox_entry_id)
  JOIN domain.outbox_delivery_attempts AS attempt USING (delivery_attempt_id);
")
test "$failure_invalid_snapshot_after" = "$failure_invalid_snapshot_before" || postgres_test_fail "invalid failure code mutated state"

# Default event-retryable failures 1..7 reschedule; failure 8 parks exactly.
insert_pending_fixture tenant-worker-alpha-synthetic event-retry-ceiling-synthetic 120000 900000 8
psql_test --command="
  DO \$ceiling\$
  DECLARE
    v_claim record;
    v_failed record;
    v_ordinal integer;
  BEGIN
    FOR v_ordinal IN 1..8 LOOP
      SELECT * INTO STRICT v_claim
      FROM domain.claim_transactional_outbox_batch('worker-ceiling-synthetic', 1);
      SELECT * INTO STRICT v_failed
      FROM domain.record_transactional_outbox_publish_failure(
        v_claim.tenant_id, v_claim.event_id, v_claim.outbox_entry_id,
        v_claim.delivery_attempt_id, v_claim.replay_generation,
        v_claim.claim_owner_id,
        domain.transactional_outbox_claim_digest(v_claim.raw_claim_token),
        'event-retryable'
      );
      IF v_ordinal < 8 AND v_failed.result_code <> 'retry-scheduled' THEN
        RAISE EXCEPTION 'event retry parked below the ceiling';
      END IF;
      IF v_ordinal = 8 AND v_failed.result_code <> 'parked' THEN
        RAISE EXCEPTION 'event retry did not park at the ceiling';
      END IF;
      IF v_ordinal < 8 THEN
        UPDATE domain.transactional_outbox
        SET next_attempt_at = clock_timestamp() - interval '1 millisecond'
        WHERE outbox_entry_id = v_claim.outbox_entry_id;
      END IF;
    END LOOP;
  END
  \$ceiling\$;
"
assert_sql_equals "event retry ceiling did not preserve eight exact attempts" "parked|8|8|8|8|0" "
  SELECT
    entry.delivery_state || '|' || entry.total_attempt_count || '|' ||
    entry.generation_attempt_count || '|' || entry.generation_failure_count || '|' ||
    count(attempt.delivery_attempt_id) || '|' ||
    count(attempt.delivery_attempt_id) FILTER (WHERE attempt.outcome = 'active')
  FROM domain.transactional_outbox AS entry
  JOIN domain.outbox_delivery_attempts AS attempt USING (tenant_id, event_id, outbox_entry_id)
  WHERE entry.event_id = 'event-retry-ceiling-synthetic'
  GROUP BY entry.delivery_state, entry.total_attempt_count,
    entry.generation_attempt_count, entry.generation_failure_count;
"

# Infrastructure classes remain schedulable even with the event counter at its
# ceiling; each increments only its own independent ordinal.
insert_pending_fixture tenant-worker-alpha-synthetic event-ceiling-transport-synthetic 120000 900000 8
insert_pending_fixture tenant-worker-alpha-synthetic event-ceiling-unknown-synthetic 120000 900000 8
psql_test --command="
  UPDATE domain.transactional_outbox
  SET total_attempt_count = 8,
      generation_attempt_count = 8,
      generation_failure_count = 8,
      last_failure_code = 'event-retryable'
  WHERE event_id IN (
    'event-ceiling-transport-synthetic', 'event-ceiling-unknown-synthetic'
  );
  CREATE TABLE domain.worker_ops_infrastructure_ceiling_claims AS
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-infrastructure-ceiling-synthetic', 2);
  CREATE TABLE domain.worker_ops_infrastructure_ceiling_results AS
  SELECT claim.event_id, failed.*
  FROM domain.worker_ops_infrastructure_ceiling_claims AS claim
  CROSS JOIN LATERAL domain.record_transactional_outbox_publish_failure(
    claim.tenant_id, claim.event_id, claim.outbox_entry_id,
    claim.delivery_attempt_id, claim.replay_generation, claim.claim_owner_id,
    domain.transactional_outbox_claim_digest(claim.raw_claim_token),
    CASE claim.event_id
      WHEN 'event-ceiling-transport-synthetic' THEN 'transport-unavailable'
      ELSE 'publish-outcome-unknown'
    END
  ) AS failed;
"
assert_sql_equals "infrastructure failure consumed/obeyed the event ceiling" "event-ceiling-transport-synthetic|retry-scheduled|pending|1|0|8|event-ceiling-unknown-synthetic|retry-scheduled|pending|0|1|8" "
  SELECT string_agg(
    result.event_id || '|' || result.result_code || '|' || entry.delivery_state || '|' ||
    entry.generation_transport_failure_count || '|' ||
    entry.generation_unknown_outcome_count || '|' || entry.generation_failure_count,
    '|' ORDER BY result.event_id
  )
  FROM domain.worker_ops_infrastructure_ceiling_results AS result
  JOIN domain.worker_ops_infrastructure_ceiling_claims AS claim USING (event_id)
  JOIN domain.transactional_outbox AS entry USING (outbox_entry_id);
"

# Concurrent ACK/failure calls serialize on the exact Entry. Exactly one wins;
# the loser observes claim-denied and the final Entry/Attempt pair is coherent.
insert_pending_fixture tenant-worker-alpha-synthetic event-ack-failure-race-synthetic 120000 900000 8
psql_test --command="
  CREATE TABLE domain.worker_ops_ack_failure_race_claim AS
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-ack-failure-race-synthetic', 1);
  CREATE TABLE domain.worker_ops_ack_race_result (result_code text);
  CREATE TABLE domain.worker_ops_failure_race_result (
    result_code text,
    next_attempt_at timestamptz
  );
"
psql_test --command="
  INSERT INTO domain.worker_ops_ack_race_result
  SELECT domain.acknowledge_transactional_outbox_published(
    tenant_id, event_id, outbox_entry_id, delivery_attempt_id,
    replay_generation, claim_owner_id,
    domain.transactional_outbox_claim_digest(raw_claim_token),
    'DOMAIN_EVENTS', 1, false, broker_message_id
  )
  FROM domain.worker_ops_ack_failure_race_claim;
" >"$temp_dir/ack-race.out" 2>"$temp_dir/ack-race.err" &
ack_race_pid=$!
psql_test --command="
  INSERT INTO domain.worker_ops_failure_race_result
  SELECT failed.*
  FROM domain.worker_ops_ack_failure_race_claim AS claim
  CROSS JOIN LATERAL domain.record_transactional_outbox_publish_failure(
    claim.tenant_id, claim.event_id, claim.outbox_entry_id,
    claim.delivery_attempt_id, claim.replay_generation, claim.claim_owner_id,
    domain.transactional_outbox_claim_digest(claim.raw_claim_token),
    'transport-unavailable'
  ) AS failed;
" >"$temp_dir/failure-race.out" 2>"$temp_dir/failure-race.err" &
failure_race_pid=$!
if ! wait "$ack_race_pid"; then
  postgres_test_fail "concurrent ACK race session failed"
fi
if ! wait "$failure_race_pid"; then
  postgres_test_fail "concurrent failure race session failed"
fi
assert_sql_equals "ACK/failure race did not have exactly one winner" "t" "
  SELECT
    (ack.result_code = 'delivered' AND failed.result_code = 'claim-denied')
    OR
    (ack.result_code = 'claim-denied' AND failed.result_code = 'retry-scheduled')
  FROM domain.worker_ops_ack_race_result AS ack,
       domain.worker_ops_failure_race_result AS failed;
"
assert_sql_equals "ACK/failure race left incoherent state" "t" "
  SELECT
    (entry.delivery_state = 'delivered'
      AND attempt.outcome = 'delivered'
      AND entry.current_attempt_id IS NULL
      AND entry.generation_transport_failure_count = 0)
    OR
    (entry.delivery_state = 'pending'
      AND attempt.outcome = 'transport_unavailable'
      AND entry.current_attempt_id IS NULL
      AND entry.generation_transport_failure_count = 1)
  FROM domain.worker_ops_ack_failure_race_claim AS claim
  JOIN domain.transactional_outbox AS entry USING (outbox_entry_id)
  JOIN domain.outbox_delivery_attempts AS attempt USING (delivery_attempt_id);
"

assert_sql_equals "claim lease/absolute ranges did not come from immutable policy" "true|true|true|true" "
  SELECT
    bool_and(lease_expires_at - claimed_at = interval '30 seconds') || '|' ||
    bool_and(absolute_lease_expires_at - claimed_at = interval '300 seconds') || '|' ||
    (SELECT lease_expires_at - claimed_at = interval '5 seconds'
     FROM domain.worker_ops_expiry_replacement) || '|' ||
    (SELECT absolute_lease_expires_at - claimed_at = interval '30 seconds'
     FROM domain.worker_ops_expiry_replacement)
  FROM domain.worker_ops_order_claims;
"

assert_sql_equals "message ID did not vary by destination/generation or remain canonical" "t" "
  SELECT
    domain.transactional_outbox_message_id('tenant', 'event', 'domain-events', 0)
      <> domain.transactional_outbox_message_id('tenant', 'event', 'other-destination', 0)
    AND domain.transactional_outbox_message_id('tenant', 'event', 'domain-events', 0)
      <> domain.transactional_outbox_message_id('tenant', 'event', 'domain-events', 1)
    AND domain.transactional_outbox_message_id('tenant', 'event', 'domain-events', 0)
      ~ '^[0-9a-f]{64}$';
"
expect_invalid_input "negative message ID generation" "
  SELECT domain.transactional_outbox_message_id('tenant', 'event', 'domain-events', -1);
"

# Every operation rejects a non-read-committed transaction before mutation.
insert_pending_fixture tenant-worker-alpha-synthetic event-isolation-synthetic 120000 900000 8
isolation_attempts_before=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.outbox_delivery_attempts;
")
expect_sql_failure_matching "ClaimBatch repeatable-read transaction" "
  BEGIN ISOLATION LEVEL REPEATABLE READ;
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-isolation-synthetic', 1);
  COMMIT;
" 'transactional outbox: persistence-failure'
isolation_attempts_after=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.outbox_delivery_attempts;
")
test "$isolation_attempts_after" = "$isolation_attempts_before" || postgres_test_fail "isolation failure partially claimed an Entry"
psql_test --command="
  UPDATE domain.transactional_outbox
  SET next_attempt_at = clock_timestamp() + interval '1 day'
  WHERE event_id IN (
    'event-isolation-synthetic',
    'event-ceiling-transport-synthetic',
    'event-ceiling-unknown-synthetic',
    'event-ack-failure-race-synthetic'
  ) AND delivery_state = 'pending';
"

isolation_claim_snapshot_before=$(psql_test --tuples-only --no-align --command="
  SELECT md5(to_jsonb(entry)::text || to_jsonb(attempt)::text)
  FROM domain.worker_ops_fence_claim AS claim
  JOIN domain.transactional_outbox AS entry USING (outbox_entry_id)
  JOIN domain.outbox_delivery_attempts AS attempt USING (delivery_attempt_id);
")
expect_sql_failure_matching "RenewClaim repeatable-read transaction" "
  BEGIN ISOLATION LEVEL REPEATABLE READ;
  SELECT renewed.*
  FROM domain.worker_ops_fence_claim AS claim
  CROSS JOIN LATERAL domain.renew_transactional_outbox_claim(
    claim.tenant_id, claim.event_id, claim.outbox_entry_id,
    claim.delivery_attempt_id, claim.replay_generation, claim.claim_owner_id,
    domain.transactional_outbox_claim_digest(claim.raw_claim_token)
  ) AS renewed;
  COMMIT;
" 'transactional outbox: persistence-failure'
expect_sql_failure_matching "ACK repeatable-read transaction" "
  BEGIN ISOLATION LEVEL REPEATABLE READ;
  SELECT domain.acknowledge_transactional_outbox_published(
    tenant_id, event_id, outbox_entry_id, delivery_attempt_id,
    replay_generation, claim_owner_id,
    domain.transactional_outbox_claim_digest(raw_claim_token),
    'DOMAIN_EVENTS', 1, false, broker_message_id
  ) FROM domain.worker_ops_fence_claim;
  COMMIT;
" 'transactional outbox: persistence-failure'
expect_sql_failure_matching "RecordFailure repeatable-read transaction" "
  BEGIN ISOLATION LEVEL REPEATABLE READ;
  SELECT failed.*
  FROM domain.worker_ops_fence_claim AS claim
  CROSS JOIN LATERAL domain.record_transactional_outbox_publish_failure(
    claim.tenant_id, claim.event_id, claim.outbox_entry_id,
    claim.delivery_attempt_id, claim.replay_generation, claim.claim_owner_id,
    domain.transactional_outbox_claim_digest(claim.raw_claim_token),
    'transport-unavailable'
  ) AS failed;
  COMMIT;
" 'transactional outbox: persistence-failure'
isolation_claim_snapshot_after=$(psql_test --tuples-only --no-align --command="
  SELECT md5(to_jsonb(entry)::text || to_jsonb(attempt)::text)
  FROM domain.worker_ops_fence_claim AS claim
  JOIN domain.transactional_outbox AS entry USING (outbox_entry_id)
  JOIN domain.outbox_delivery_attempts AS attempt USING (delivery_attempt_id);
")
test "$isolation_claim_snapshot_after" = "$isolation_claim_snapshot_before" || postgres_test_fail "isolation rejection mutated current Claim"

# Claim counters at MaxInt64 fail atomically before token generation or Attempt
# insertion. The Entry remains the exact pre-call pending row.
insert_pending_fixture tenant-worker-alpha-synthetic event-max-claim-synthetic 120000 900000 8
psql_test --command="
  UPDATE domain.transactional_outbox
  SET total_attempt_count = 9223372036854775807,
      generation_attempt_count = 9223372036854775807
  WHERE event_id = 'event-max-claim-synthetic';
"
max_claim_snapshot_before=$(psql_test --tuples-only --no-align --command="
  SELECT md5(to_jsonb(entry)::text)
  FROM domain.transactional_outbox AS entry
  WHERE event_id = 'event-max-claim-synthetic';
")
max_claim_attempts_before=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.outbox_delivery_attempts;
")
expect_sql_failure_matching "ClaimBatch MaxInt64 increment" "
  SELECT * FROM domain.claim_transactional_outbox_batch('worker-max-claim-synthetic', 1);
" 'transactional outbox: persistence-failure'
max_claim_snapshot_after=$(psql_test --tuples-only --no-align --command="
  SELECT md5(to_jsonb(entry)::text)
  FROM domain.transactional_outbox AS entry
  WHERE event_id = 'event-max-claim-synthetic';
")
max_claim_attempts_after=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.outbox_delivery_attempts;
")
test "$max_claim_snapshot_after" = "$max_claim_snapshot_before" || postgres_test_fail "MaxInt64 ClaimBatch mutated Entry"
test "$max_claim_attempts_after" = "$max_claim_attempts_before" || postgres_test_fail "MaxInt64 ClaimBatch inserted Attempt"
psql_test --command="
  UPDATE domain.transactional_outbox
  SET next_attempt_at = clock_timestamp() + interval '1 day'
  WHERE event_id = 'event-max-claim-synthetic';
"

prepare_max_failure_fixture() {
  max_fixture_event_id=$1
  max_fixture_owner=$2
  max_fixture_counter=$3
  case "$max_fixture_counter" in
    transport)
      max_transport_count=9223372036854775807
      max_unknown_count=0
      max_event_count=0
      ;;
    unknown)
      max_transport_count=0
      max_unknown_count=9223372036854775807
      max_event_count=0
      ;;
    event)
      max_transport_count=0
      max_unknown_count=0
      max_event_count=9223372036854775807
      ;;
    *) postgres_test_fail "test bug: unknown MaxInt64 counter fixture" ;;
  esac
  insert_pending_fixture tenant-worker-alpha-synthetic "$max_fixture_event_id" 120000 900000 8
  psql_test --command="
    DO \$fixture\$
    DECLARE
      v_entry_id bigint;
      v_attempt_id bigint;
      v_now timestamptz := clock_timestamp();
    BEGIN
      SELECT outbox_entry_id INTO v_entry_id
      FROM domain.transactional_outbox
      WHERE event_id = '$max_fixture_event_id';
      INSERT INTO domain.outbox_delivery_attempts (
        tenant_id, event_id, outbox_entry_id, replay_generation,
        total_attempt_number, generation_attempt_number,
        claim_owner_id, claim_token_digest,
        claimed_at, lease_expires_at, absolute_lease_expires_at,
        broker_message_id, policy_id, policy_snapshot_digest,
        effective_lease_ms, effective_retention_days
      ) VALUES (
        'tenant-worker-alpha-synthetic', '$max_fixture_event_id', v_entry_id, 0,
        9223372036854775807, 9223372036854775807,
        '$max_fixture_owner', decode('$TOKEN_DIGEST_HEX', 'hex'),
        v_now, v_now + interval '120 seconds', v_now + interval '900 seconds',
        domain.transactional_outbox_message_id(
          'tenant-worker-alpha-synthetic', '$max_fixture_event_id', 'domain-events', 0
        ),
        'threadline.outbox.policy/v1', decode('$POLICY_DIGEST_HEX', 'hex'),
        120000, 90
      ) RETURNING delivery_attempt_id INTO v_attempt_id;
      UPDATE domain.transactional_outbox
      SET delivery_state = 'claimed',
          total_attempt_count = 9223372036854775807,
          generation_attempt_count = 9223372036854775807,
          generation_transport_failure_count = $max_transport_count,
          generation_unknown_outcome_count = $max_unknown_count,
          generation_failure_count = $max_event_count,
          next_attempt_at = NULL,
          current_attempt_id = v_attempt_id
      WHERE outbox_entry_id = v_entry_id;
    END
    \$fixture\$;
  "
}

prepare_max_failure_fixture event-max-transport-synthetic worker-max-transport-synthetic transport
prepare_max_failure_fixture event-max-unknown-synthetic worker-max-unknown-synthetic unknown
prepare_max_failure_fixture event-max-event-synthetic worker-max-event-synthetic event
max_failure_snapshot_before=$(psql_test --tuples-only --no-align --command="
  SELECT md5(string_agg(to_jsonb(entry)::text || to_jsonb(attempt)::text, '' ORDER BY entry.event_id))
  FROM domain.transactional_outbox AS entry
  JOIN domain.outbox_delivery_attempts AS attempt
    ON attempt.delivery_attempt_id = entry.current_attempt_id
  WHERE entry.event_id LIKE 'event-max-%-synthetic'
    AND entry.delivery_state = 'claimed';
")
expect_sql_failure_matching "transport counter MaxInt64 increment" "
  SELECT failed.*
  FROM domain.transactional_outbox AS entry
  JOIN domain.outbox_delivery_attempts AS attempt
    ON attempt.delivery_attempt_id = entry.current_attempt_id
  CROSS JOIN LATERAL domain.record_transactional_outbox_publish_failure(
    entry.tenant_id, entry.event_id, entry.outbox_entry_id,
    attempt.delivery_attempt_id, entry.replay_generation,
    attempt.claim_owner_id, decode('$TOKEN_DIGEST_HEX', 'hex'),
    'transport-unavailable'
  ) AS failed
  WHERE entry.event_id = 'event-max-transport-synthetic';
" 'transactional outbox: persistence-failure'
expect_sql_failure_matching "unknown counter MaxInt64 increment" "
  SELECT failed.*
  FROM domain.transactional_outbox AS entry
  JOIN domain.outbox_delivery_attempts AS attempt
    ON attempt.delivery_attempt_id = entry.current_attempt_id
  CROSS JOIN LATERAL domain.record_transactional_outbox_publish_failure(
    entry.tenant_id, entry.event_id, entry.outbox_entry_id,
    attempt.delivery_attempt_id, entry.replay_generation,
    attempt.claim_owner_id, decode('$TOKEN_DIGEST_HEX', 'hex'),
    'publish-outcome-unknown'
  ) AS failed
  WHERE entry.event_id = 'event-max-unknown-synthetic';
" 'transactional outbox: persistence-failure'
expect_sql_failure_matching "event counter MaxInt64 increment" "
  SELECT failed.*
  FROM domain.transactional_outbox AS entry
  JOIN domain.outbox_delivery_attempts AS attempt
    ON attempt.delivery_attempt_id = entry.current_attempt_id
  CROSS JOIN LATERAL domain.record_transactional_outbox_publish_failure(
    entry.tenant_id, entry.event_id, entry.outbox_entry_id,
    attempt.delivery_attempt_id, entry.replay_generation,
    attempt.claim_owner_id, decode('$TOKEN_DIGEST_HEX', 'hex'),
    'event-retryable'
  ) AS failed
  WHERE entry.event_id = 'event-max-event-synthetic';
" 'transactional outbox: persistence-failure'
max_failure_snapshot_after=$(psql_test --tuples-only --no-align --command="
  SELECT md5(string_agg(to_jsonb(entry)::text || to_jsonb(attempt)::text, '' ORDER BY entry.event_id))
  FROM domain.transactional_outbox AS entry
  JOIN domain.outbox_delivery_attempts AS attempt
    ON attempt.delivery_attempt_id = entry.current_attempt_id
  WHERE entry.event_id LIKE 'event-max-%-synthetic'
    AND entry.delivery_state = 'claimed';
")
test "$max_failure_snapshot_after" = "$max_failure_snapshot_before" || postgres_test_fail "MaxInt64 RecordFailure partially mutated state"

# SECURITY DEFINER operations pin a trusted search path, do not grant PUBLIC
# execution, and expose neither stored digest through any result type nor raw
# authority through persistent production tables.
assert_sql_equals "Worker operations did not pin the trusted search path" "4" "
  SELECT count(*)
  FROM pg_proc AS procedure
  JOIN pg_namespace AS namespace ON namespace.oid = procedure.pronamespace
  WHERE namespace.nspname = 'domain'
    AND procedure.proname IN (
      'claim_transactional_outbox_batch',
      'renew_transactional_outbox_claim',
      'acknowledge_transactional_outbox_published',
      'record_transactional_outbox_publish_failure'
    )
    AND procedure.prosecdef
    AND 'search_path=pg_catalog, domain' = ANY(procedure.proconfig);
"
assert_sql_equals "Worker operations granted PUBLIC execution" "0" "
  SELECT count(*)
  FROM pg_proc AS procedure
  JOIN pg_namespace AS namespace ON namespace.oid = procedure.pronamespace
  CROSS JOIN LATERAL aclexplode(
    coalesce(procedure.proacl, acldefault('f', procedure.proowner))
  ) AS privilege
  WHERE namespace.nspname = 'domain'
    AND procedure.proname IN (
      'claim_transactional_outbox_batch',
      'renew_transactional_outbox_claim',
      'acknowledge_transactional_outbox_published',
      'record_transactional_outbox_publish_failure'
    )
    AND privilege.grantee = 0
    AND privilege.privilege_type = 'EXECUTE';
"
assert_sql_equals "Worker result types exposed a stored claim-token digest" "0" "
  SELECT count(*)
  FROM information_schema.attributes
  WHERE udt_schema = 'domain'
    AND udt_name IN (
      'transactional_outbox_claim_result',
      'transactional_outbox_renew_result',
      'transactional_outbox_failure_result'
    )
    AND attribute_name = 'claim_token_digest';
"

psql_test --file="$WORKER_OPS_DOWN"

worker_function_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*)
  FROM pg_proc AS procedure
  JOIN pg_namespace AS namespace ON namespace.oid = procedure.pronamespace
  WHERE namespace.nspname = 'domain'
    AND procedure.proname IN (
      'transactional_outbox_claim_digest',
      'transactional_outbox_message_id',
      'transactional_outbox_backoff_delay_ms',
      'claim_transactional_outbox_batch',
      'renew_transactional_outbox_claim',
      'acknowledge_transactional_outbox_published',
      'record_transactional_outbox_publish_failure'
    );
")
test "$worker_function_count" = "0" || postgres_test_fail "000009 down left Worker functions installed"

worker_result_type_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*)
  FROM pg_type AS result_type
  JOIN pg_namespace AS namespace ON namespace.oid = result_type.typnamespace
  WHERE namespace.nspname = 'domain'
    AND result_type.typname IN (
      'transactional_outbox_claim_result',
      'transactional_outbox_renew_result',
      'transactional_outbox_failure_result'
    );
")
test "$worker_result_type_count" = "0" || postgres_test_fail "000009 down left Worker result types installed"

extension_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM pg_extension WHERE extname = 'pgcrypto';
")
test "$extension_count" = "1" || postgres_test_fail "000009 down removed shared pgcrypto"

sentinel_row_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*) FROM domain.worker_ops_down_sentinel
  WHERE marker = 'worker-ops-owned-by-test-synthetic';
")
test "$sentinel_row_count" = "1" || postgres_test_fail "000009 down removed unrelated table data"

sentinel_function_result=$(psql_test --tuples-only --no-align --command="
  SELECT domain.worker_ops_unrelated_sentinel();
")
test "$sentinel_function_result" = "1" || postgres_test_fail "000009 down removed unrelated function"

outbox_table_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*)
  FROM information_schema.tables
  WHERE table_schema = 'domain'
    AND table_name IN (
      'domain_events', 'transactional_outbox', 'outbox_delivery_attempts'
    );
")
test "$outbox_table_count" = "3" || postgres_test_fail "000009 down removed migration 000008 tables"

psql_test --file="$WORKER_OPS_UP"

postgres_test_finish "transactional Outbox Worker operations passed"

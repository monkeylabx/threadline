BEGIN;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_extension
    WHERE extname = 'pgcrypto'
  )
      OR to_regprocedure('public.gen_random_bytes(integer)') IS NULL
      OR to_regprocedure('public.digest(bytea,text)') IS NULL THEN
    RAISE EXCEPTION 'transactional outbox: pgcrypto-precondition-failed'
      USING ERRCODE = 'object_not_in_prerequisite_state';
  END IF;
END;
$$;

CREATE TYPE domain.transactional_outbox_claim_result AS (
  result_code text,
  tenant_id text,
  event_id text,
  outbox_entry_id bigint,
  delivery_attempt_id bigint,
  replay_generation bigint,
  total_attempt_number bigint,
  generation_attempt_number bigint,
  claim_owner_id text,
  raw_claim_token bytea,
  claimed_at timestamp with time zone,
  lease_expires_at timestamp with time zone,
  absolute_lease_expires_at timestamp with time zone,
  broker_message_id text,
  destination text,
  event_type text,
  schema_version integer,
  aggregate_kind text,
  aggregate_id text,
  payload bytea,
  occurred_at timestamp with time zone,
  enqueued_at timestamp with time zone,
  policy_id text,
  policy_snapshot_digest bytea
);

CREATE TYPE domain.transactional_outbox_renew_result AS (
  result_code text,
  lease_expires_at timestamp with time zone
);

CREATE TYPE domain.transactional_outbox_failure_result AS (
  result_code text,
  next_attempt_at timestamp with time zone
);

CREATE FUNCTION domain.transactional_outbox_claim_digest(p_raw_claim_token bytea)
RETURNS bytea
LANGUAGE plpgsql
IMMUTABLE
STRICT
AS $$
BEGIN
  IF octet_length(p_raw_claim_token) <> 32 THEN
    RAISE EXCEPTION 'transactional outbox: invalid-input'
      USING ERRCODE = 'P0001';
  END IF;

  RETURN public.digest(
    convert_to('threadline.outbox.claim-token/v1', 'UTF8')
      || decode('00', 'hex')
      || p_raw_claim_token,
    'sha256'
  );
END;
$$;

CREATE FUNCTION domain.transactional_outbox_message_id(
  p_tenant_id text,
  p_event_id text,
  p_destination text,
  p_replay_generation bigint
)
RETURNS text
LANGUAGE plpgsql
IMMUTABLE
STRICT
AS $$
DECLARE
  v_preimage bytea;
BEGIN
  IF p_tenant_id = ''
      OR p_event_id = ''
      OR p_destination = ''
      OR p_replay_generation < 0 THEN
    RAISE EXCEPTION 'transactional outbox: invalid-input'
      USING ERRCODE = 'P0001';
  END IF;

  v_preimage := convert_to('threadline.outbox.msg-id/v1', 'UTF8')
    || decode('00', 'hex')
    || int4send(octet_length(convert_to(p_tenant_id, 'UTF8')))
    || convert_to(p_tenant_id, 'UTF8')
    || int4send(octet_length(convert_to(p_event_id, 'UTF8')))
    || convert_to(p_event_id, 'UTF8')
    || int4send(octet_length(convert_to(p_destination, 'UTF8')))
    || convert_to(p_destination, 'UTF8')
    || int8send(p_replay_generation);

  RETURN encode(public.digest(v_preimage, 'sha256'), 'hex');
END;
$$;

CREATE FUNCTION domain.transactional_outbox_backoff_delay_ms(
  p_claim_token_digest bytea,
  p_failure_class smallint,
  p_ordinal bigint,
  p_base_ms integer,
  p_cap_ms integer
)
RETURNS bigint
LANGUAGE plpgsql
IMMUTABLE
STRICT
AS $$
DECLARE
  v_shift integer;
  v_window bigint;
  v_jitter_digest bytea;
  v_remainder bigint := 0;
  v_index integer;
BEGIN
  IF octet_length(p_claim_token_digest) <> 32
      OR p_failure_class NOT IN (1, 2, 3)
      OR p_ordinal < 1
      OR p_base_ms < 1
      OR p_cap_ms < p_base_ms
      OR p_cap_ms > 900000 THEN
    RAISE EXCEPTION 'transactional outbox: invalid-input'
      USING ERRCODE = 'P0001';
  END IF;

  v_shift := CASE
    WHEN p_ordinal > 21 THEN 20
    ELSE (p_ordinal - 1)::integer
  END;
  v_window := LEAST(
    p_base_ms::bigint * (1::bigint << v_shift),
    p_cap_ms::bigint
  );
  v_jitter_digest := public.digest(
    convert_to('threadline.outbox.backoff-jitter/v1', 'UTF8')
      || decode('00', 'hex')
      || p_claim_token_digest
      || decode(lpad(to_hex(p_failure_class::integer), 2, '0'), 'hex')
      || int8send(p_ordinal),
    'sha256'
  );

  FOR v_index IN 0..7 LOOP
    v_remainder := (v_remainder * 256 + get_byte(v_jitter_digest, v_index))
      % (v_window + 1);
  END LOOP;

  RETURN v_remainder;
END;
$$;

CREATE FUNCTION domain.claim_transactional_outbox_batch(
  p_claim_owner_id text,
  p_batch_size integer
)
RETURNS SETOF domain.transactional_outbox_claim_result
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
SET search_path = pg_catalog, domain
AS $$
DECLARE
  v_now timestamp with time zone := clock_timestamp();
  v_entry record;
  v_current_attempt record;
  v_raw_claim_token bytea;
  v_claim_token_digest bytea;
  v_delivery_attempt_id bigint;
  v_total_attempt_number bigint;
  v_generation_attempt_number bigint;
  v_broker_message_id text;
BEGIN
  IF current_setting('transaction_isolation') <> 'read committed' THEN
    RAISE EXCEPTION 'transactional outbox: persistence-failure'
      USING ERRCODE = 'P0001';
  END IF;
  IF p_batch_size IS NULL OR p_batch_size NOT BETWEEN 1 AND 256
      OR p_claim_owner_id IS NULL
      OR p_claim_owner_id = ''
      OR p_claim_owner_id IS DISTINCT FROM btrim(p_claim_owner_id)
      OR octet_length(p_claim_owner_id) > 128
      OR p_claim_owner_id ~ '[[:cntrl:]]' THEN
    RAISE EXCEPTION 'transactional outbox: invalid-input'
      USING ERRCODE = 'P0001';
  END IF;

  FOR v_entry IN
    SELECT
      entry.*,
      event.event_type,
      event.schema_version,
      event.aggregate_kind,
      event.aggregate_id,
      event.payload,
      event.occurred_at,
      event.enqueued_at,
      current_attempt.lease_expires_at AS current_lease_expires_at
    FROM domain.transactional_outbox AS entry
    JOIN domain.domain_events AS event
      ON event.tenant_id = entry.tenant_id
     AND event.event_id = entry.event_id
    LEFT JOIN domain.outbox_delivery_attempts AS current_attempt
      ON current_attempt.tenant_id = entry.tenant_id
     AND current_attempt.event_id = entry.event_id
     AND current_attempt.outbox_entry_id = entry.outbox_entry_id
     AND current_attempt.replay_generation = entry.replay_generation
     AND current_attempt.delivery_attempt_id = entry.current_attempt_id
     AND current_attempt.outcome = 'active'
    WHERE (
        entry.delivery_state = 'pending'
        AND entry.next_attempt_at <= v_now
      ) OR (
        entry.delivery_state = 'claimed'
        AND current_attempt.lease_expires_at <= v_now
      )
    ORDER BY
      CASE
        WHEN entry.delivery_state = 'pending' THEN entry.next_attempt_at
        ELSE current_attempt.lease_expires_at
      END,
      event.enqueued_at,
      entry.outbox_entry_id
    FOR UPDATE OF entry SKIP LOCKED
    LIMIT p_batch_size
  LOOP
    IF v_entry.total_attempt_count = 9223372036854775807
        OR v_entry.generation_attempt_count = 9223372036854775807 THEN
      RAISE EXCEPTION 'transactional outbox: persistence-failure'
        USING ERRCODE = 'P0001';
    END IF;

    IF v_entry.delivery_state = 'claimed' THEN
      SELECT attempt.*
      INTO v_current_attempt
      FROM domain.outbox_delivery_attempts AS attempt
      WHERE attempt.tenant_id = v_entry.tenant_id
        AND attempt.event_id = v_entry.event_id
        AND attempt.outbox_entry_id = v_entry.outbox_entry_id
        AND attempt.replay_generation = v_entry.replay_generation
        AND attempt.delivery_attempt_id = v_entry.current_attempt_id
      FOR UPDATE;

      IF NOT FOUND
          OR v_current_attempt.outcome <> 'active'
          OR v_current_attempt.lease_expires_at > v_now THEN
        RAISE EXCEPTION 'transactional outbox: persistence-failure'
          USING ERRCODE = 'P0001';
      END IF;

      UPDATE domain.outbox_delivery_attempts
      SET outcome = 'lease_expired',
          finished_at = v_now,
          evidence_not_before = v_now
            + (v_current_attempt.effective_retention_days::bigint * 86400)
              * INTERVAL '1 second'
      WHERE delivery_attempt_id = v_current_attempt.delivery_attempt_id;
    END IF;

    v_total_attempt_number := v_entry.total_attempt_count + 1;
    v_generation_attempt_number := v_entry.generation_attempt_count + 1;
    v_raw_claim_token := public.gen_random_bytes(32);
    v_claim_token_digest := domain.transactional_outbox_claim_digest(v_raw_claim_token);
    v_broker_message_id := domain.transactional_outbox_message_id(
      v_entry.tenant_id,
      v_entry.event_id,
      v_entry.destination,
      v_entry.replay_generation
    );

    INSERT INTO domain.outbox_delivery_attempts (
      tenant_id,
      event_id,
      outbox_entry_id,
      replay_generation,
      total_attempt_number,
      generation_attempt_number,
      claim_owner_id,
      claim_token_digest,
      claimed_at,
      lease_expires_at,
      absolute_lease_expires_at,
      broker_message_id,
      policy_id,
      policy_snapshot_digest,
      effective_lease_ms,
      effective_retention_days
    ) VALUES (
      v_entry.tenant_id,
      v_entry.event_id,
      v_entry.outbox_entry_id,
      v_entry.replay_generation,
      v_total_attempt_number,
      v_generation_attempt_number,
      p_claim_owner_id,
      v_claim_token_digest,
      v_now,
      v_now + v_entry.effective_lease_ms::bigint * INTERVAL '1 millisecond',
      v_now + v_entry.effective_absolute_lifetime_ms::bigint * INTERVAL '1 millisecond',
      v_broker_message_id,
      v_entry.policy_id,
      v_entry.policy_snapshot_digest,
      v_entry.effective_lease_ms,
      v_entry.effective_retention_days
    )
    RETURNING domain.outbox_delivery_attempts.delivery_attempt_id
    INTO v_delivery_attempt_id;

    UPDATE domain.transactional_outbox
    SET delivery_state = 'claimed',
        total_attempt_count = v_total_attempt_number,
        generation_attempt_count = v_generation_attempt_number,
        next_attempt_at = NULL,
        current_attempt_id = v_delivery_attempt_id
    WHERE transactional_outbox.outbox_entry_id = v_entry.outbox_entry_id;

    RETURN NEXT ROW(
      'claimed',
      v_entry.tenant_id,
      v_entry.event_id,
      v_entry.outbox_entry_id,
      v_delivery_attempt_id,
      v_entry.replay_generation,
      v_total_attempt_number,
      v_generation_attempt_number,
      p_claim_owner_id,
      v_raw_claim_token,
      v_now,
      v_now + v_entry.effective_lease_ms::bigint * INTERVAL '1 millisecond',
      v_now
        + v_entry.effective_absolute_lifetime_ms::bigint * INTERVAL '1 millisecond',
      v_broker_message_id,
      v_entry.destination,
      v_entry.event_type,
      v_entry.schema_version,
      v_entry.aggregate_kind,
      v_entry.aggregate_id,
      v_entry.payload,
      v_entry.occurred_at,
      v_entry.enqueued_at,
      v_entry.policy_id,
      v_entry.policy_snapshot_digest
    )::domain.transactional_outbox_claim_result;
  END LOOP;
END;
$$;

CREATE FUNCTION domain.renew_transactional_outbox_claim(
  p_tenant_id text,
  p_event_id text,
  p_outbox_entry_id bigint,
  p_delivery_attempt_id bigint,
  p_replay_generation bigint,
  p_claim_owner_id text,
  p_candidate_digest bytea
)
RETURNS SETOF domain.transactional_outbox_renew_result
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
SET search_path = pg_catalog, domain
AS $$
DECLARE
  v_now timestamp with time zone;
  v_entry record;
  v_attempt record;
  v_new_lease_expires_at timestamp with time zone;
BEGIN
  IF current_setting('transaction_isolation') <> 'read committed' THEN
    RAISE EXCEPTION 'transactional outbox: persistence-failure'
      USING ERRCODE = 'P0001';
  END IF;

  SELECT entry.*
  INTO v_entry
  FROM domain.transactional_outbox AS entry
  WHERE entry.tenant_id = p_tenant_id
    AND entry.event_id = p_event_id
    AND entry.outbox_entry_id = p_outbox_entry_id
  FOR UPDATE;

  IF NOT FOUND
      OR v_entry.delivery_state <> 'claimed'
      OR v_entry.replay_generation <> p_replay_generation
      OR v_entry.current_attempt_id <> p_delivery_attempt_id THEN
    RETURN NEXT ROW(
      'claim-denied', NULL::timestamp with time zone
    )::domain.transactional_outbox_renew_result;
    RETURN;
  END IF;

  SELECT attempt.*
  INTO v_attempt
  FROM domain.outbox_delivery_attempts AS attempt
  WHERE attempt.tenant_id = p_tenant_id
    AND attempt.event_id = p_event_id
    AND attempt.outbox_entry_id = p_outbox_entry_id
    AND attempt.delivery_attempt_id = p_delivery_attempt_id
    AND attempt.replay_generation = p_replay_generation
  FOR UPDATE;

  IF NOT FOUND
      OR v_attempt.outcome <> 'active'
      OR v_attempt.claim_owner_id IS DISTINCT FROM p_claim_owner_id
      OR v_attempt.claim_token_digest IS DISTINCT FROM p_candidate_digest THEN
    RETURN NEXT ROW(
      'claim-denied', NULL::timestamp with time zone
    )::domain.transactional_outbox_renew_result;
    RETURN;
  END IF;

  -- A caller may have waited on either row lock. Lease authority is evaluated
  -- against database time only after both exact rows are locked.
  v_now := clock_timestamp();
  IF v_now >= v_attempt.lease_expires_at
      OR v_now >= v_attempt.absolute_lease_expires_at THEN
    RETURN NEXT ROW(
      'claim-denied', NULL::timestamp with time zone
    )::domain.transactional_outbox_renew_result;
    RETURN;
  END IF;

  v_new_lease_expires_at := LEAST(
    v_now + v_attempt.effective_lease_ms::bigint * INTERVAL '1 millisecond',
    v_attempt.absolute_lease_expires_at
  );
  IF v_new_lease_expires_at <= v_attempt.lease_expires_at THEN
    RETURN NEXT ROW(
      'claim-denied', NULL::timestamp with time zone
    )::domain.transactional_outbox_renew_result;
    RETURN;
  END IF;

  UPDATE domain.outbox_delivery_attempts
  SET lease_expires_at = v_new_lease_expires_at
  WHERE outbox_delivery_attempts.delivery_attempt_id = p_delivery_attempt_id;

  RETURN NEXT ROW(
    'renewed', v_new_lease_expires_at
  )::domain.transactional_outbox_renew_result;
END;
$$;

CREATE FUNCTION domain.acknowledge_transactional_outbox_published(
  p_tenant_id text,
  p_event_id text,
  p_outbox_entry_id bigint,
  p_delivery_attempt_id bigint,
  p_replay_generation bigint,
  p_claim_owner_id text,
  p_candidate_digest bytea,
  p_broker_stream text,
  p_broker_sequence numeric,
  p_broker_duplicate boolean,
  p_broker_message_id text
)
RETURNS text
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
SET search_path = pg_catalog, domain
AS $$
DECLARE
  v_now timestamp with time zone;
  v_entry record;
  v_attempt record;
BEGIN
  IF current_setting('transaction_isolation') <> 'read committed' THEN
    RAISE EXCEPTION 'transactional outbox: persistence-failure'
      USING ERRCODE = 'P0001';
  END IF;
  IF p_broker_stream IS NULL
      OR p_broker_stream = ''
      OR p_broker_stream IS DISTINCT FROM btrim(p_broker_stream)
      OR octet_length(p_broker_stream) > 255
      OR p_broker_stream ~ '[[:cntrl:]]'
      OR p_broker_sequence IS NULL
      OR p_broker_sequence <> trunc(p_broker_sequence)
      OR p_broker_sequence NOT BETWEEN 1 AND 18446744073709551615
      OR p_broker_duplicate IS NULL
      OR p_broker_message_id IS NULL
      OR p_broker_message_id !~ '^[0-9a-f]{64}$' THEN
    RAISE EXCEPTION 'transactional outbox: invalid-input'
      USING ERRCODE = 'P0001';
  END IF;

  SELECT entry.*
  INTO v_entry
  FROM domain.transactional_outbox AS entry
  WHERE entry.tenant_id = p_tenant_id
    AND entry.event_id = p_event_id
    AND entry.outbox_entry_id = p_outbox_entry_id
  FOR UPDATE;

  IF NOT FOUND OR v_entry.replay_generation <> p_replay_generation THEN
    RETURN 'claim-denied';
  END IF;

  SELECT attempt.*
  INTO v_attempt
  FROM domain.outbox_delivery_attempts AS attempt
  WHERE attempt.tenant_id = p_tenant_id
    AND attempt.event_id = p_event_id
    AND attempt.outbox_entry_id = p_outbox_entry_id
    AND attempt.delivery_attempt_id = p_delivery_attempt_id
    AND attempt.replay_generation = p_replay_generation
  FOR UPDATE;

  IF NOT FOUND
      OR v_attempt.claim_owner_id IS DISTINCT FROM p_claim_owner_id
      OR v_attempt.claim_token_digest IS DISTINCT FROM p_candidate_digest
      OR v_attempt.broker_message_id IS DISTINCT FROM p_broker_message_id THEN
    RETURN 'claim-denied';
  END IF;

  IF v_attempt.outcome = 'delivered'
      AND v_entry.delivery_state = 'delivered'
      AND v_attempt.broker_stream IS NOT DISTINCT FROM p_broker_stream
      AND v_attempt.broker_sequence IS NOT DISTINCT FROM p_broker_sequence
      AND v_attempt.broker_duplicate IS NOT DISTINCT FROM p_broker_duplicate THEN
    RETURN 'already-delivered';
  END IF;

  -- Do not let time spent waiting for either exact row lock extend authority.
  v_now := clock_timestamp();
  IF v_entry.delivery_state <> 'claimed'
      OR v_entry.current_attempt_id <> p_delivery_attempt_id
      OR v_attempt.outcome <> 'active'
      OR v_now >= v_attempt.lease_expires_at THEN
    RETURN 'claim-denied';
  END IF;

  UPDATE domain.outbox_delivery_attempts
  SET outcome = 'delivered',
      finished_at = v_now,
      broker_stream = p_broker_stream,
      broker_sequence = p_broker_sequence,
      broker_duplicate = p_broker_duplicate,
      evidence_not_before = v_now
        + (v_attempt.effective_retention_days::bigint * 86400)
          * INTERVAL '1 second'
  WHERE outbox_delivery_attempts.delivery_attempt_id = p_delivery_attempt_id;

  UPDATE domain.transactional_outbox
  SET delivery_state = 'delivered',
      current_attempt_id = NULL,
      last_failure_code = NULL
  WHERE transactional_outbox.outbox_entry_id = p_outbox_entry_id;

  RETURN 'delivered';
END;
$$;

CREATE FUNCTION domain.record_transactional_outbox_publish_failure(
  p_tenant_id text,
  p_event_id text,
  p_outbox_entry_id bigint,
  p_delivery_attempt_id bigint,
  p_replay_generation bigint,
  p_claim_owner_id text,
  p_candidate_digest bytea,
  p_failure_code text
)
RETURNS SETOF domain.transactional_outbox_failure_result
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
SET search_path = pg_catalog, domain
AS $$
DECLARE
  v_now timestamp with time zone;
  v_entry record;
  v_attempt record;
  v_failure_outcome text;
  v_failure_class smallint;
  v_failure_ordinal bigint;
  v_delay_ms bigint;
  v_scheduled_at timestamp with time zone;
  v_base_ms integer;
  v_cap_ms integer;
BEGIN
  IF current_setting('transaction_isolation') <> 'read committed' THEN
    RAISE EXCEPTION 'transactional outbox: persistence-failure'
      USING ERRCODE = 'P0001';
  END IF;
  IF p_failure_code IS NULL OR p_failure_code NOT IN (
    'transport-unavailable',
    'publish-outcome-unknown',
    'event-retryable',
    'event-permanent'
  ) THEN
    RAISE EXCEPTION 'transactional outbox: invalid-input'
      USING ERRCODE = 'P0001';
  END IF;

  SELECT entry.*
  INTO v_entry
  FROM domain.transactional_outbox AS entry
  WHERE entry.tenant_id = p_tenant_id
    AND entry.event_id = p_event_id
    AND entry.outbox_entry_id = p_outbox_entry_id
  FOR UPDATE;

  IF NOT FOUND
      OR v_entry.delivery_state <> 'claimed'
      OR v_entry.replay_generation <> p_replay_generation
      OR v_entry.current_attempt_id <> p_delivery_attempt_id THEN
    RETURN NEXT ROW(
      'claim-denied', NULL::timestamp with time zone
    )::domain.transactional_outbox_failure_result;
    RETURN;
  END IF;

  SELECT attempt.*
  INTO v_attempt
  FROM domain.outbox_delivery_attempts AS attempt
  WHERE attempt.tenant_id = p_tenant_id
    AND attempt.event_id = p_event_id
    AND attempt.outbox_entry_id = p_outbox_entry_id
    AND attempt.delivery_attempt_id = p_delivery_attempt_id
    AND attempt.replay_generation = p_replay_generation
  FOR UPDATE;

  IF NOT FOUND
      OR v_attempt.outcome <> 'active'
      OR v_attempt.claim_owner_id IS DISTINCT FROM p_claim_owner_id
      OR v_attempt.claim_token_digest IS DISTINCT FROM p_candidate_digest THEN
    RETURN NEXT ROW(
      'claim-denied', NULL::timestamp with time zone
    )::domain.transactional_outbox_failure_result;
    RETURN;
  END IF;

  -- Do not let time spent waiting for either exact row lock extend authority.
  v_now := clock_timestamp();
  IF v_now >= v_attempt.lease_expires_at THEN
    RETURN NEXT ROW(
      'claim-denied', NULL::timestamp with time zone
    )::domain.transactional_outbox_failure_result;
    RETURN;
  END IF;

  v_failure_outcome := replace(p_failure_code, '-', '_');
  IF p_failure_code = 'transport-unavailable' THEN
    IF v_entry.generation_transport_failure_count = 9223372036854775807 THEN
      RAISE EXCEPTION 'transactional outbox: persistence-failure'
        USING ERRCODE = 'P0001';
    END IF;
    v_failure_class := 1;
    v_failure_ordinal := v_entry.generation_transport_failure_count + 1;
    v_base_ms := v_entry.effective_transport_base_ms;
    v_cap_ms := v_entry.effective_transport_cap_ms;
  ELSIF p_failure_code = 'publish-outcome-unknown' THEN
    IF v_entry.generation_unknown_outcome_count = 9223372036854775807 THEN
      RAISE EXCEPTION 'transactional outbox: persistence-failure'
        USING ERRCODE = 'P0001';
    END IF;
    v_failure_class := 2;
    v_failure_ordinal := v_entry.generation_unknown_outcome_count + 1;
    v_base_ms := v_entry.effective_unknown_base_ms;
    v_cap_ms := v_entry.effective_unknown_cap_ms;
  ELSIF p_failure_code = 'event-retryable' THEN
    IF v_entry.generation_failure_count = 9223372036854775807 THEN
      RAISE EXCEPTION 'transactional outbox: persistence-failure'
        USING ERRCODE = 'P0001';
    END IF;
    v_failure_class := 3;
    v_failure_ordinal := v_entry.generation_failure_count + 1;
    v_base_ms := v_entry.effective_event_base_ms;
    v_cap_ms := v_entry.effective_event_cap_ms;
  END IF;

  IF p_failure_code <> 'event-permanent'
      AND NOT (
        p_failure_code = 'event-retryable'
        AND v_failure_ordinal >= v_entry.effective_event_retry_ceiling
      ) THEN
    v_delay_ms := domain.transactional_outbox_backoff_delay_ms(
      v_attempt.claim_token_digest,
      v_failure_class,
      v_failure_ordinal,
      v_base_ms,
      v_cap_ms
    );
    v_scheduled_at := v_now + v_delay_ms * INTERVAL '1 millisecond';
  END IF;

  UPDATE domain.outbox_delivery_attempts
  SET outcome = v_failure_outcome,
      finished_at = v_now,
      failure_code = p_failure_code,
      evidence_not_before = v_now
        + (v_attempt.effective_retention_days::bigint * 86400)
          * INTERVAL '1 second'
  WHERE outbox_delivery_attempts.delivery_attempt_id = p_delivery_attempt_id;

  IF p_failure_code = 'event-permanent'
      OR (
        p_failure_code = 'event-retryable'
        AND v_failure_ordinal >= v_entry.effective_event_retry_ceiling
      ) THEN
    UPDATE domain.transactional_outbox
    SET delivery_state = 'parked',
        generation_failure_count = CASE
          WHEN p_failure_code = 'event-retryable' THEN v_failure_ordinal
          ELSE generation_failure_count
        END,
        next_attempt_at = NULL,
        current_attempt_id = NULL,
        last_failure_code = p_failure_code,
        parked_at = v_now
    WHERE transactional_outbox.outbox_entry_id = p_outbox_entry_id;
    RETURN NEXT ROW(
      'parked', NULL::timestamp with time zone
    )::domain.transactional_outbox_failure_result;
  ELSE
    UPDATE domain.transactional_outbox
    SET delivery_state = 'pending',
        generation_transport_failure_count = CASE
          WHEN p_failure_code = 'transport-unavailable' THEN v_failure_ordinal
          ELSE generation_transport_failure_count
        END,
        generation_unknown_outcome_count = CASE
          WHEN p_failure_code = 'publish-outcome-unknown' THEN v_failure_ordinal
          ELSE generation_unknown_outcome_count
        END,
        generation_failure_count = CASE
          WHEN p_failure_code = 'event-retryable' THEN v_failure_ordinal
          ELSE generation_failure_count
        END,
        next_attempt_at = v_scheduled_at,
        current_attempt_id = NULL,
        last_failure_code = p_failure_code,
        parked_at = NULL
    WHERE transactional_outbox.outbox_entry_id = p_outbox_entry_id;
    RETURN NEXT ROW(
      'retry-scheduled', v_scheduled_at
    )::domain.transactional_outbox_failure_result;
  END IF;
END;
$$;

REVOKE ALL ON FUNCTION domain.claim_transactional_outbox_batch(text, integer)
FROM PUBLIC;
REVOKE ALL ON FUNCTION domain.renew_transactional_outbox_claim(
  text, text, bigint, bigint, bigint, text, bytea
) FROM PUBLIC;
REVOKE ALL ON FUNCTION domain.acknowledge_transactional_outbox_published(
  text, text, bigint, bigint, bigint, text, bytea, text, numeric, boolean, text
) FROM PUBLIC;
REVOKE ALL ON FUNCTION domain.record_transactional_outbox_publish_failure(
  text, text, bigint, bigint, bigint, text, bytea, text
) FROM PUBLIC;
REVOKE ALL ON FUNCTION domain.transactional_outbox_claim_digest(bytea)
FROM PUBLIC;
REVOKE ALL ON FUNCTION domain.transactional_outbox_message_id(
  text, text, text, bigint
) FROM PUBLIC;
REVOKE ALL ON FUNCTION domain.transactional_outbox_backoff_delay_ms(
  bytea, smallint, bigint, integer, integer
) FROM PUBLIC;
REVOKE ALL ON TYPE domain.transactional_outbox_claim_result FROM PUBLIC;
REVOKE ALL ON TYPE domain.transactional_outbox_renew_result FROM PUBLIC;
REVOKE ALL ON TYPE domain.transactional_outbox_failure_result FROM PUBLIC;

COMMIT;

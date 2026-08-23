BEGIN;

CREATE TABLE domain.domain_events (
  tenant_id text NOT NULL,
  event_id text NOT NULL,
  event_type text NOT NULL,
  schema_version integer NOT NULL,
  aggregate_kind text NOT NULL,
  aggregate_id text NOT NULL,
  payload bytea NOT NULL,
  occurred_at timestamp with time zone NOT NULL,
  enqueued_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, event_id),
  CONSTRAINT domain_events_organization_fk
    FOREIGN KEY (tenant_id)
      REFERENCES domain.organizations (tenant_id),
  CONSTRAINT domain_events_tenant_id_not_blank
    CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
  CONSTRAINT domain_events_event_id_not_blank
    CHECK (event_id <> '' AND event_id = btrim(event_id)),
  CONSTRAINT domain_events_event_type_canonical
    CHECK (
      event_type <> ''
      AND event_type = btrim(event_type)
      AND event_type = lower(event_type)
      AND strpos(event_type, '.') > 1
      AND right(event_type, 1) <> '.'
    ),
  CONSTRAINT domain_events_schema_version_positive
    CHECK (schema_version > 0),
  CONSTRAINT domain_events_aggregate_kind_not_blank
    CHECK (aggregate_kind <> '' AND aggregate_kind = btrim(aggregate_kind)),
  CONSTRAINT domain_events_aggregate_id_not_blank
    CHECK (aggregate_id <> '' AND aggregate_id = btrim(aggregate_id)),
  CONSTRAINT domain_events_payload_size
    CHECK (octet_length(payload) <= 262144)
);

CREATE TABLE domain.transactional_outbox (
  outbox_entry_id bigint GENERATED ALWAYS AS IDENTITY,
  tenant_id text NOT NULL,
  event_id text NOT NULL,
  destination text NOT NULL,
  delivery_state text NOT NULL DEFAULT 'pending',
  total_attempt_count bigint NOT NULL DEFAULT 0,
  replay_generation bigint NOT NULL DEFAULT 0,
  generation_attempt_count bigint NOT NULL DEFAULT 0,
  generation_transport_failure_count bigint NOT NULL DEFAULT 0,
  generation_unknown_outcome_count bigint NOT NULL DEFAULT 0,
  generation_failure_count bigint NOT NULL DEFAULT 0,
  next_attempt_at timestamp with time zone,
  current_attempt_id bigint,
  last_failure_code text,
  parked_at timestamp with time zone,
  policy_id text NOT NULL,
  policy_snapshot_digest bytea NOT NULL,
  effective_lease_ms integer NOT NULL,
  effective_absolute_lifetime_ms integer NOT NULL,
  effective_event_retry_ceiling integer NOT NULL,
  effective_transport_base_ms integer NOT NULL,
  effective_transport_cap_ms integer NOT NULL,
  effective_unknown_base_ms integer NOT NULL,
  effective_unknown_cap_ms integer NOT NULL,
  effective_event_base_ms integer NOT NULL,
  effective_event_cap_ms integer NOT NULL,
  effective_retention_days integer NOT NULL,
  PRIMARY KEY (outbox_entry_id),
  CONSTRAINT transactional_outbox_exact_entry_unique
    UNIQUE (tenant_id, event_id, outbox_entry_id),
  CONSTRAINT transactional_outbox_event_destination_unique
    UNIQUE (tenant_id, event_id, destination),
  CONSTRAINT transactional_outbox_current_reference_unique
    UNIQUE (tenant_id, event_id, outbox_entry_id, replay_generation),
  CONSTRAINT transactional_outbox_exact_policy_snapshot_unique
    UNIQUE (
      tenant_id,
      event_id,
      outbox_entry_id,
      replay_generation,
      policy_id,
      policy_snapshot_digest,
      effective_lease_ms,
      effective_retention_days
    ),
  CONSTRAINT transactional_outbox_event_fk
    FOREIGN KEY (tenant_id, event_id)
      REFERENCES domain.domain_events (tenant_id, event_id),
  CONSTRAINT transactional_outbox_destination_known
    CHECK (destination = 'domain-events'),
  CONSTRAINT transactional_outbox_delivery_state_known
    CHECK (delivery_state IN ('pending', 'claimed', 'delivered', 'parked')),
  CONSTRAINT transactional_outbox_counters_non_negative
    CHECK (
      total_attempt_count >= 0
      AND replay_generation >= 0
      AND generation_attempt_count >= 0
      AND generation_transport_failure_count >= 0
      AND generation_unknown_outcome_count >= 0
      AND generation_failure_count >= 0
    ),
  CONSTRAINT transactional_outbox_generation_counts_bounded
    CHECK (
      generation_attempt_count <= total_attempt_count
      AND generation_transport_failure_count
        + generation_unknown_outcome_count
        + generation_failure_count <= generation_attempt_count
    ),
  CONSTRAINT transactional_outbox_failure_code_known
    CHECK (
      last_failure_code IS NULL
      OR last_failure_code IN (
        'transport-unavailable',
        'publish-outcome-unknown',
        'event-retryable',
        'event-permanent'
      )
    ),
  CONSTRAINT transactional_outbox_policy_id_known
    CHECK (policy_id = 'threadline.outbox.policy/v1'),
  CONSTRAINT transactional_outbox_policy_digest_size
    CHECK (octet_length(policy_snapshot_digest) = 32),
  CONSTRAINT transactional_outbox_effective_lease_range
    CHECK (effective_lease_ms BETWEEN 5000 AND 120000),
  CONSTRAINT transactional_outbox_effective_absolute_lifetime_range
    CHECK (
      effective_absolute_lifetime_ms BETWEEN 30000 AND 900000
      AND effective_absolute_lifetime_ms >= effective_lease_ms * 2
    ),
  CONSTRAINT transactional_outbox_effective_retry_ceiling_range
    CHECK (effective_event_retry_ceiling BETWEEN 1 AND 20),
  CONSTRAINT transactional_outbox_effective_transport_backoff_range
    CHECK (
      effective_transport_base_ms BETWEEN 100 AND 10000
      AND effective_transport_cap_ms BETWEEN 1000 AND 300000
      AND effective_transport_cap_ms >= effective_transport_base_ms
    ),
  CONSTRAINT transactional_outbox_effective_unknown_backoff_range
    CHECK (
      effective_unknown_base_ms BETWEEN 500 AND 30000
      AND effective_unknown_cap_ms BETWEEN 5000 AND 900000
      AND effective_unknown_cap_ms >= effective_unknown_base_ms
    ),
  CONSTRAINT transactional_outbox_effective_event_backoff_range
    CHECK (
      effective_event_base_ms BETWEEN 500 AND 30000
      AND effective_event_cap_ms BETWEEN 5000 AND 900000
      AND effective_event_cap_ms >= effective_event_base_ms
    ),
  CONSTRAINT transactional_outbox_effective_retention_range
    CHECK (effective_retention_days BETWEEN 30 AND 365),
  CONSTRAINT transactional_outbox_state_fields_consistent
    CHECK (
      (
        delivery_state = 'pending'
        AND next_attempt_at IS NOT NULL
        AND current_attempt_id IS NULL
        AND (
          last_failure_code IS NULL
          OR last_failure_code IN (
            'transport-unavailable',
            'publish-outcome-unknown',
            'event-retryable'
          )
        )
        AND parked_at IS NULL
      )
      OR
      (
        delivery_state = 'claimed'
        AND next_attempt_at IS NULL
        AND current_attempt_id IS NOT NULL
        AND (
          last_failure_code IS NULL
          OR last_failure_code IN (
            'transport-unavailable',
            'publish-outcome-unknown',
            'event-retryable'
          )
        )
        AND parked_at IS NULL
      )
      OR
      (
        delivery_state = 'delivered'
        AND next_attempt_at IS NULL
        AND current_attempt_id IS NULL
        AND last_failure_code IS NULL
        AND parked_at IS NULL
      )
      OR
      (
        delivery_state = 'parked'
        AND next_attempt_at IS NULL
        AND current_attempt_id IS NULL
        AND last_failure_code IN ('event-retryable', 'event-permanent')
        AND parked_at IS NOT NULL
      )
    )
);

CREATE TABLE domain.outbox_delivery_attempts (
  delivery_attempt_id bigint GENERATED ALWAYS AS IDENTITY,
  tenant_id text NOT NULL,
  event_id text NOT NULL,
  outbox_entry_id bigint NOT NULL,
  replay_generation bigint NOT NULL,
  total_attempt_number bigint NOT NULL,
  generation_attempt_number bigint NOT NULL,
  claim_owner_id text NOT NULL,
  claim_token_digest bytea NOT NULL,
  claimed_at timestamp with time zone NOT NULL,
  lease_expires_at timestamp with time zone NOT NULL,
  absolute_lease_expires_at timestamp with time zone NOT NULL,
  outcome text NOT NULL DEFAULT 'active',
  finished_at timestamp with time zone,
  failure_code text,
  broker_stream text,
  broker_sequence numeric(20, 0),
  broker_duplicate boolean,
  broker_message_id text NOT NULL,
  policy_id text NOT NULL,
  policy_snapshot_digest bytea NOT NULL,
  effective_lease_ms integer NOT NULL,
  effective_retention_days integer NOT NULL,
  evidence_not_before timestamp with time zone,
  PRIMARY KEY (delivery_attempt_id),
  CONSTRAINT outbox_delivery_attempts_current_reference_unique
    UNIQUE (
      tenant_id,
      event_id,
      outbox_entry_id,
      replay_generation,
      delivery_attempt_id
    ),
  CONSTRAINT outbox_delivery_attempts_lifetime_ordinal_unique
    UNIQUE (tenant_id, event_id, outbox_entry_id, total_attempt_number),
  CONSTRAINT outbox_delivery_attempts_generation_ordinal_unique
    UNIQUE (
      tenant_id,
      event_id,
      outbox_entry_id,
      replay_generation,
      generation_attempt_number
    ),
  CONSTRAINT outbox_delivery_attempts_exact_entry_policy_fk
    FOREIGN KEY (
      tenant_id,
      event_id,
      outbox_entry_id,
      replay_generation,
      policy_id,
      policy_snapshot_digest,
      effective_lease_ms,
      effective_retention_days
    ) REFERENCES domain.transactional_outbox (
      tenant_id,
      event_id,
      outbox_entry_id,
      replay_generation,
      policy_id,
      policy_snapshot_digest,
      effective_lease_ms,
      effective_retention_days
    ),
  CONSTRAINT outbox_delivery_attempts_replay_generation_non_negative
    CHECK (replay_generation >= 0),
  CONSTRAINT outbox_delivery_attempts_ordinals_positive
    CHECK (total_attempt_number > 0 AND generation_attempt_number > 0),
  CONSTRAINT outbox_delivery_attempts_claim_owner_not_blank
    CHECK (claim_owner_id <> '' AND claim_owner_id = btrim(claim_owner_id)),
  CONSTRAINT outbox_delivery_attempts_claim_digest_size
    CHECK (octet_length(claim_token_digest) = 32),
  CONSTRAINT outbox_delivery_attempts_lease_order
    CHECK (
      claimed_at < lease_expires_at
      AND lease_expires_at <= absolute_lease_expires_at
    ),
  CONSTRAINT outbox_delivery_attempts_outcome_known
    CHECK (
      outcome IN (
        'active',
        'delivered',
        'transport_unavailable',
        'publish_outcome_unknown',
        'event_retryable',
        'event_permanent',
        'lease_expired'
      )
    ),
  CONSTRAINT outbox_delivery_attempts_failure_code_known
    CHECK (
      failure_code IS NULL
      OR failure_code IN (
        'transport-unavailable',
        'publish-outcome-unknown',
        'event-retryable',
        'event-permanent'
      )
    ),
  CONSTRAINT outbox_delivery_attempts_broker_stream_not_blank
    CHECK (
      broker_stream IS NULL
      OR (broker_stream <> '' AND broker_stream = btrim(broker_stream))
    ),
  CONSTRAINT outbox_delivery_attempts_broker_sequence_range
    CHECK (
      broker_sequence IS NULL
      OR broker_sequence BETWEEN 1 AND 18446744073709551615
    ),
  CONSTRAINT outbox_delivery_attempts_broker_message_id_canonical
    CHECK (broker_message_id ~ '^[0-9a-f]{64}$'),
  CONSTRAINT outbox_delivery_attempts_policy_id_known
    CHECK (policy_id = 'threadline.outbox.policy/v1'),
  CONSTRAINT outbox_delivery_attempts_policy_digest_size
    CHECK (octet_length(policy_snapshot_digest) = 32),
  CONSTRAINT outbox_delivery_attempts_effective_lease_range
    CHECK (effective_lease_ms BETWEEN 5000 AND 120000),
  CONSTRAINT outbox_delivery_attempts_effective_retention_range
    CHECK (effective_retention_days BETWEEN 30 AND 365),
  CONSTRAINT outbox_delivery_attempts_finished_after_claim
    CHECK (finished_at IS NULL OR finished_at >= claimed_at),
  CONSTRAINT outbox_delivery_attempts_evidence_cutoff_exact
    CHECK (
      evidence_not_before IS NULL
      OR evidence_not_before = finished_at
        + (effective_retention_days::bigint * 86400) * INTERVAL '1 second'
    ),
  CONSTRAINT outbox_delivery_attempts_terminal_fields_consistent
    CHECK (
      (
        outcome = 'active'
        AND finished_at IS NULL
        AND failure_code IS NULL
        AND broker_stream IS NULL
        AND broker_sequence IS NULL
        AND broker_duplicate IS NULL
        AND evidence_not_before IS NULL
      )
      OR
      (
        outcome = 'delivered'
        AND finished_at IS NOT NULL
        AND failure_code IS NULL
        AND broker_stream IS NOT NULL
        AND broker_sequence IS NOT NULL
        AND broker_duplicate IS NOT NULL
        AND evidence_not_before IS NOT NULL
      )
      OR
      (
        outcome = 'transport_unavailable'
        AND finished_at IS NOT NULL
        AND failure_code = 'transport-unavailable'
        AND broker_stream IS NULL
        AND broker_sequence IS NULL
        AND broker_duplicate IS NULL
        AND evidence_not_before IS NOT NULL
      )
      OR
      (
        outcome = 'publish_outcome_unknown'
        AND finished_at IS NOT NULL
        AND failure_code = 'publish-outcome-unknown'
        AND broker_stream IS NULL
        AND broker_sequence IS NULL
        AND broker_duplicate IS NULL
        AND evidence_not_before IS NOT NULL
      )
      OR
      (
        outcome = 'event_retryable'
        AND finished_at IS NOT NULL
        AND failure_code = 'event-retryable'
        AND broker_stream IS NULL
        AND broker_sequence IS NULL
        AND broker_duplicate IS NULL
        AND evidence_not_before IS NOT NULL
      )
      OR
      (
        outcome = 'event_permanent'
        AND finished_at IS NOT NULL
        AND failure_code = 'event-permanent'
        AND broker_stream IS NULL
        AND broker_sequence IS NULL
        AND broker_duplicate IS NULL
        AND evidence_not_before IS NOT NULL
      )
      OR
      (
        outcome = 'lease_expired'
        AND finished_at IS NOT NULL
        AND failure_code IS NULL
        AND broker_stream IS NULL
        AND broker_sequence IS NULL
        AND broker_duplicate IS NULL
        AND evidence_not_before IS NOT NULL
      )
    )
);

ALTER TABLE domain.transactional_outbox
  ADD CONSTRAINT transactional_outbox_current_attempt_fk
  FOREIGN KEY (
    tenant_id,
    event_id,
    outbox_entry_id,
    replay_generation,
    current_attempt_id
  ) REFERENCES domain.outbox_delivery_attempts (
    tenant_id,
    event_id,
    outbox_entry_id,
    replay_generation,
    delivery_attempt_id
  )
  DEFERRABLE INITIALLY DEFERRED;

CREATE UNIQUE INDEX outbox_delivery_attempts_one_active_per_entry
ON domain.outbox_delivery_attempts (tenant_id, event_id, outbox_entry_id)
WHERE outcome = 'active';

CREATE INDEX transactional_outbox_pending_eligibility
ON domain.transactional_outbox (next_attempt_at, outbox_entry_id)
WHERE delivery_state = 'pending';

CREATE INDEX outbox_delivery_attempts_active_expiry
ON domain.outbox_delivery_attempts (lease_expires_at, outbox_entry_id)
WHERE outcome = 'active';

CREATE FUNCTION domain.enforce_domain_event_immutability()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'Domain Events are immutable and cannot be deleted'
    USING ERRCODE = 'check_violation';
END;
$$;

CREATE TRIGGER domain_events_immutable_guard
BEFORE UPDATE OR DELETE ON domain.domain_events
FOR EACH ROW
EXECUTE FUNCTION domain.enforce_domain_event_immutability();

CREATE FUNCTION domain.enforce_transactional_outbox_lifecycle()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    IF NEW.delivery_state <> 'pending'
        OR NEW.total_attempt_count <> 0
        OR NEW.replay_generation <> 0
        OR NEW.generation_attempt_count <> 0
        OR NEW.generation_transport_failure_count <> 0
        OR NEW.generation_unknown_outcome_count <> 0
        OR NEW.generation_failure_count <> 0
        OR NEW.last_failure_code IS NOT NULL THEN
      RAISE EXCEPTION 'Transactional Outbox Entries must start as a fresh pending generation'
        USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
  END IF;

  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'Transactional Outbox Entries cannot be deleted'
      USING ERRCODE = 'check_violation';
  END IF;

  IF OLD.outbox_entry_id IS DISTINCT FROM NEW.outbox_entry_id
      OR OLD.tenant_id IS DISTINCT FROM NEW.tenant_id
      OR OLD.event_id IS DISTINCT FROM NEW.event_id
      OR OLD.destination IS DISTINCT FROM NEW.destination
      OR OLD.replay_generation IS DISTINCT FROM NEW.replay_generation
      OR OLD.policy_id IS DISTINCT FROM NEW.policy_id
      OR OLD.policy_snapshot_digest IS DISTINCT FROM NEW.policy_snapshot_digest
      OR OLD.effective_lease_ms IS DISTINCT FROM NEW.effective_lease_ms
      OR OLD.effective_absolute_lifetime_ms IS DISTINCT FROM NEW.effective_absolute_lifetime_ms
      OR OLD.effective_event_retry_ceiling IS DISTINCT FROM NEW.effective_event_retry_ceiling
      OR OLD.effective_transport_base_ms IS DISTINCT FROM NEW.effective_transport_base_ms
      OR OLD.effective_transport_cap_ms IS DISTINCT FROM NEW.effective_transport_cap_ms
      OR OLD.effective_unknown_base_ms IS DISTINCT FROM NEW.effective_unknown_base_ms
      OR OLD.effective_unknown_cap_ms IS DISTINCT FROM NEW.effective_unknown_cap_ms
      OR OLD.effective_event_base_ms IS DISTINCT FROM NEW.effective_event_base_ms
      OR OLD.effective_event_cap_ms IS DISTINCT FROM NEW.effective_event_cap_ms
      OR OLD.effective_retention_days IS DISTINCT FROM NEW.effective_retention_days THEN
    RAISE EXCEPTION 'Transactional Outbox immutable facts cannot be changed'
      USING ERRCODE = 'check_violation';
  END IF;

  IF NEW.total_attempt_count < OLD.total_attempt_count
      OR NEW.generation_attempt_count < OLD.generation_attempt_count
      OR NEW.generation_transport_failure_count < OLD.generation_transport_failure_count
      OR NEW.generation_unknown_outcome_count < OLD.generation_unknown_outcome_count
      OR NEW.generation_failure_count < OLD.generation_failure_count THEN
    RAISE EXCEPTION 'Transactional Outbox counters cannot decrease'
      USING ERRCODE = 'check_violation';
  END IF;

  IF OLD.delivery_state IN ('delivered', 'parked') THEN
    RAISE EXCEPTION 'Terminal Transactional Outbox Entries cannot be changed'
      USING ERRCODE = 'check_violation';
  END IF;

  IF OLD.delivery_state = 'pending'
      AND NEW.delivery_state NOT IN ('pending', 'claimed') THEN
    RAISE EXCEPTION 'Invalid Transactional Outbox state transition'
      USING ERRCODE = 'check_violation';
  END IF;

  IF OLD.delivery_state = 'claimed'
      AND NEW.delivery_state NOT IN ('pending', 'claimed', 'delivered', 'parked') THEN
    RAISE EXCEPTION 'Invalid Transactional Outbox state transition'
      USING ERRCODE = 'check_violation';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER transactional_outbox_lifecycle_guard
BEFORE INSERT OR UPDATE OR DELETE ON domain.transactional_outbox
FOR EACH ROW
EXECUTE FUNCTION domain.enforce_transactional_outbox_lifecycle();

CREATE FUNCTION domain.enforce_outbox_delivery_attempt_lifecycle()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    IF NEW.outcome <> 'active' THEN
      RAISE EXCEPTION 'Outbox Delivery Attempts must start active'
        USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
  END IF;

  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'Outbox Delivery Attempts cannot be deleted'
      USING ERRCODE = 'check_violation';
  END IF;

  IF OLD.delivery_attempt_id IS DISTINCT FROM NEW.delivery_attempt_id
      OR OLD.tenant_id IS DISTINCT FROM NEW.tenant_id
      OR OLD.event_id IS DISTINCT FROM NEW.event_id
      OR OLD.outbox_entry_id IS DISTINCT FROM NEW.outbox_entry_id
      OR OLD.replay_generation IS DISTINCT FROM NEW.replay_generation
      OR OLD.total_attempt_number IS DISTINCT FROM NEW.total_attempt_number
      OR OLD.generation_attempt_number IS DISTINCT FROM NEW.generation_attempt_number
      OR OLD.claim_owner_id IS DISTINCT FROM NEW.claim_owner_id
      OR OLD.claim_token_digest IS DISTINCT FROM NEW.claim_token_digest
      OR OLD.claimed_at IS DISTINCT FROM NEW.claimed_at
      OR OLD.absolute_lease_expires_at IS DISTINCT FROM NEW.absolute_lease_expires_at
      OR OLD.broker_message_id IS DISTINCT FROM NEW.broker_message_id
      OR OLD.policy_id IS DISTINCT FROM NEW.policy_id
      OR OLD.policy_snapshot_digest IS DISTINCT FROM NEW.policy_snapshot_digest
      OR OLD.effective_lease_ms IS DISTINCT FROM NEW.effective_lease_ms
      OR OLD.effective_retention_days IS DISTINCT FROM NEW.effective_retention_days THEN
    RAISE EXCEPTION 'Outbox Delivery Attempt immutable facts cannot be changed'
      USING ERRCODE = 'check_violation';
  END IF;

  IF OLD.outcome <> 'active' THEN
    RAISE EXCEPTION 'Terminal Outbox Delivery Attempts cannot be changed'
      USING ERRCODE = 'check_violation';
  END IF;

  IF NEW.outcome = 'active' THEN
    IF NEW.lease_expires_at <= OLD.lease_expires_at THEN
      RAISE EXCEPTION 'Outbox Delivery Attempt renewal must increase the lease'
        USING ERRCODE = 'check_violation';
    END IF;
  ELSIF NEW.lease_expires_at IS DISTINCT FROM OLD.lease_expires_at THEN
    RAISE EXCEPTION 'Terminalizing an Outbox Delivery Attempt cannot change its lease'
      USING ERRCODE = 'check_violation';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER outbox_delivery_attempts_lifecycle_guard
BEFORE INSERT OR UPDATE OR DELETE ON domain.outbox_delivery_attempts
FOR EACH ROW
EXECUTE FUNCTION domain.enforce_outbox_delivery_attempt_lifecycle();

CREATE FUNCTION domain.enforce_outbox_current_attempt_consistency()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  checked_tenant_id text;
  checked_event_id text;
  checked_entry_id bigint;
  entry_state text;
  entry_generation bigint;
  entry_total_attempt_count bigint;
  entry_generation_attempt_count bigint;
  entry_current_attempt_id bigint;
  active_attempt_count bigint;
  exact_current_attempt_count bigint;
BEGIN
  IF TG_OP = 'DELETE' THEN
    checked_tenant_id := OLD.tenant_id;
    checked_event_id := OLD.event_id;
    checked_entry_id := OLD.outbox_entry_id;
  ELSE
    checked_tenant_id := NEW.tenant_id;
    checked_event_id := NEW.event_id;
    checked_entry_id := NEW.outbox_entry_id;
  END IF;

  SELECT
    delivery_state,
    replay_generation,
    total_attempt_count,
    generation_attempt_count,
    current_attempt_id
  INTO
    entry_state,
    entry_generation,
    entry_total_attempt_count,
    entry_generation_attempt_count,
    entry_current_attempt_id
  FROM domain.transactional_outbox
  WHERE tenant_id = checked_tenant_id
    AND event_id = checked_event_id
    AND outbox_entry_id = checked_entry_id;

  IF NOT FOUND THEN
    RETURN NULL;
  END IF;

  SELECT
    count(*) FILTER (WHERE outcome = 'active'),
    count(*) FILTER (
      WHERE outcome = 'active'
        AND replay_generation = entry_generation
        AND total_attempt_number = entry_total_attempt_count
        AND generation_attempt_number = entry_generation_attempt_count
        AND delivery_attempt_id = entry_current_attempt_id
    )
  INTO active_attempt_count, exact_current_attempt_count
  FROM domain.outbox_delivery_attempts
  WHERE tenant_id = checked_tenant_id
    AND event_id = checked_event_id
    AND outbox_entry_id = checked_entry_id;

  IF entry_state = 'claimed' THEN
    IF active_attempt_count <> 1 OR exact_current_attempt_count <> 1 THEN
      RAISE EXCEPTION 'Claimed Outbox Entry must reference its sole active Attempt'
        USING ERRCODE = 'check_violation';
    END IF;
  ELSIF active_attempt_count <> 0 THEN
    RAISE EXCEPTION 'Non-claimed Outbox Entry cannot have an active Attempt'
      USING ERRCODE = 'check_violation';
  END IF;

  RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER transactional_outbox_current_attempt_consistency
AFTER INSERT OR UPDATE ON domain.transactional_outbox
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION domain.enforce_outbox_current_attempt_consistency();

CREATE CONSTRAINT TRIGGER outbox_delivery_attempts_current_consistency
AFTER INSERT OR UPDATE OR DELETE ON domain.outbox_delivery_attempts
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION domain.enforce_outbox_current_attempt_consistency();

COMMIT;

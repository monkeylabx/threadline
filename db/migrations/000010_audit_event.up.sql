BEGIN;

CREATE FUNCTION domain.audit_identifier_is_canonical(identifier text)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
  SELECT
    octet_length(identifier) BETWEEN 1 AND 255
    AND NOT (
      ascii(NULLIF(left(identifier, 1), '')) IN (
        9, 10, 11, 12, 13, 32, 160, 5760, 8232, 8233, 8239, 8287, 12288, 65279
      )
      OR ascii(NULLIF(left(identifier, 1), '')) BETWEEN 8192 AND 8202
      OR ascii(NULLIF(right(identifier, 1), '')) IN (
        9, 10, 11, 12, 13, 32, 160, 5760, 8232, 8233, 8239, 8287, 12288, 65279
      )
      OR ascii(NULLIF(right(identifier, 1), '')) BETWEEN 8192 AND 8202
    )
    AND identifier !~ '[*?[:cntrl:]]'
$$;

CREATE TABLE domain.audit_events (
  tenant_id text NOT NULL,
  audit_event_id text NOT NULL,
  contract_version smallint NOT NULL,
  tenant_sequence bigint NOT NULL,
  recorded_at timestamp with time zone NOT NULL,
  principal_actor_type smallint NOT NULL,
  principal_actor_id text NOT NULL,
  action text NOT NULL,
  outcome text NOT NULL,
  reason text NOT NULL,
  target_type text NOT NULL,
  target_id text NOT NULL,
  target_version bigint,
  policy_version text NOT NULL,
  request_id text NOT NULL,
  approval_id text,
  recovery_case_id text,
  evidence_digest bytea,
  previous_event_hash bytea NOT NULL,
  event_hash bytea NOT NULL,
  PRIMARY KEY (tenant_id, audit_event_id),
  CONSTRAINT audit_events_tenant_sequence_unique
    UNIQUE (tenant_id, tenant_sequence),
  CONSTRAINT audit_events_exact_head_reference_unique
    UNIQUE (tenant_id, audit_event_id, tenant_sequence, event_hash),
  CONSTRAINT audit_events_organization_fk
    FOREIGN KEY (tenant_id)
      REFERENCES domain.organizations (tenant_id),
  CONSTRAINT audit_events_contract_version_v1
    CHECK (contract_version = 1),
  CONSTRAINT audit_events_tenant_sequence_positive
    CHECK (tenant_sequence > 0),
  CONSTRAINT audit_events_principal_actor_type_known
    CHECK (principal_actor_type IN (1, 2, 3)),
  CONSTRAINT audit_events_action_known
    CHECK (
      action IN (
        'channel.archive',
        'capability_grant.issue',
        'capability_grant.revoke',
        'retention.expire',
        'retention.legal_hold.apply',
        'retention.legal_hold.release',
        'recovery.request',
        'recovery.decision',
        'recovery.commit',
        'outbox.replay.request'
      )
    ),
  CONSTRAINT audit_events_outcome_known
    CHECK (outcome IN ('succeeded', 'denied', 'failed')),
  CONSTRAINT audit_events_reason_known
    CHECK (
      reason IN (
        'authorized',
        'authorization_denied',
        'evidence_invalid',
        'policy_denied',
        'retention_expired',
        'state_conflict',
        'invalid_input',
        'internal_failure'
      )
    ),
  CONSTRAINT audit_events_target_type_known
    CHECK (
      target_type IN (
        'channel',
        'capability_grant',
        'retention_subject',
        'recovery_case',
        'outbox_entry'
      )
    ),
  CONSTRAINT audit_events_target_version_positive
    CHECK (target_version IS NULL OR target_version > 0),
  CONSTRAINT audit_events_digest_sizes
    CHECK (
      (evidence_digest IS NULL OR octet_length(evidence_digest) = 32)
      AND octet_length(previous_event_hash) = 32
      AND octet_length(event_hash) = 32
    ),
  CONSTRAINT audit_events_action_target_consistent
    CHECK (
      (action = 'channel.archive' AND target_type = 'channel')
      OR
      (action IN ('capability_grant.issue', 'capability_grant.revoke')
        AND target_type = 'capability_grant')
      OR
      (action IN ('retention.expire', 'retention.legal_hold.apply', 'retention.legal_hold.release')
        AND target_type = 'retention_subject')
      OR
      (action IN ('recovery.request', 'recovery.decision', 'recovery.commit')
        AND target_type = 'recovery_case'
        AND recovery_case_id = target_id)
      OR
      (action = 'outbox.replay.request'
        AND target_type = 'outbox_entry'
        AND target_version IS NOT NULL
        AND (
          outcome <> 'succeeded'
          OR (approval_id IS NOT NULL AND evidence_digest IS NOT NULL)
        ))
    ),
  CONSTRAINT audit_events_recovery_reference_consistent
    CHECK (
      (
        action IN ('recovery.request', 'recovery.decision', 'recovery.commit')
        AND recovery_case_id = target_id
      )
      OR
      (
        action NOT IN ('recovery.request', 'recovery.decision', 'recovery.commit')
        AND recovery_case_id IS NULL
      )
    ),
  CONSTRAINT audit_events_identifiers_canonical
    CHECK (
      domain.audit_identifier_is_canonical(tenant_id)
      AND domain.audit_identifier_is_canonical(audit_event_id)
      AND domain.audit_identifier_is_canonical(principal_actor_id)
      AND domain.audit_identifier_is_canonical(target_id)
      AND domain.audit_identifier_is_canonical(policy_version)
      AND domain.audit_identifier_is_canonical(request_id)
      AND (approval_id IS NULL OR domain.audit_identifier_is_canonical(approval_id))
      AND (recovery_case_id IS NULL OR domain.audit_identifier_is_canonical(recovery_case_id))
    )
);

CREATE INDEX audit_events_tenant_request_id_idx
ON domain.audit_events (tenant_id, request_id);

CREATE TABLE domain.audit_tenant_heads (
  tenant_id text NOT NULL,
  last_sequence bigint NOT NULL DEFAULT 0,
  last_audit_event_id text,
  last_event_hash bytea NOT NULL DEFAULT decode(repeat('00', 32), 'hex'),
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id),
  CONSTRAINT audit_tenant_heads_organization_fk
    FOREIGN KEY (tenant_id)
      REFERENCES domain.organizations (tenant_id),
  CONSTRAINT audit_tenant_heads_exact_event_fk
    FOREIGN KEY (tenant_id, last_audit_event_id, last_sequence, last_event_hash)
      REFERENCES domain.audit_events (
        tenant_id,
        audit_event_id,
        tenant_sequence,
        event_hash
      )
      DEFERRABLE INITIALLY DEFERRED,
  CONSTRAINT audit_tenant_heads_sequence_non_negative
    CHECK (last_sequence >= 0),
  CONSTRAINT audit_tenant_heads_hash_size
    CHECK (octet_length(last_event_hash) = 32),
  CONSTRAINT audit_tenant_heads_genesis_consistent
    CHECK (
      (
        last_sequence = 0
        AND last_audit_event_id IS NULL
        AND last_event_hash = decode(repeat('00', 32), 'hex')
      )
      OR
      (last_sequence > 0 AND last_audit_event_id IS NOT NULL)
    )
);

CREATE FUNCTION domain.prepare_audit_event_append()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  current_sequence bigint;
  current_hash bytea;
  current_recorded_at timestamp with time zone;
BEGIN
  SELECT head.last_sequence, head.last_event_hash
  INTO current_sequence, current_hash
  FROM domain.audit_tenant_heads AS head
  WHERE head.tenant_id = NEW.tenant_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'Audit append requires a tenant head'
      USING ERRCODE = 'foreign_key_violation';
  END IF;
  IF current_sequence = 9223372036854775807
      OR NEW.tenant_sequence <> current_sequence + 1
      OR NEW.previous_event_hash IS DISTINCT FROM current_hash THEN
    RAISE EXCEPTION 'Audit append slot is stale or non-contiguous'
      USING ERRCODE = 'serialization_failure';
  END IF;
  IF NEW.recorded_at IS DISTINCT FROM CURRENT_TIMESTAMP THEN
    RAISE EXCEPTION 'Audit append recording time is not owned by this transaction'
      USING ERRCODE = 'serialization_failure';
  END IF;
  NEW.recorded_at := CURRENT_TIMESTAMP;
  IF current_sequence > 0 THEN
    SELECT recorded_at
    INTO current_recorded_at
    FROM domain.audit_events
    WHERE tenant_id = NEW.tenant_id
      AND tenant_sequence = current_sequence;
    IF NOT FOUND OR NEW.recorded_at < current_recorded_at THEN
      RAISE EXCEPTION 'Audit recording time is non-monotonic'
        USING ERRCODE = 'check_violation';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER audit_events_append_guard
BEFORE INSERT ON domain.audit_events
FOR EACH ROW
EXECUTE FUNCTION domain.prepare_audit_event_append();

CREATE FUNCTION domain.reject_audit_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'Audit Events are append-only'
    USING ERRCODE = 'check_violation';
END;
$$;

CREATE TRIGGER audit_events_immutable_guard
BEFORE UPDATE OR DELETE ON domain.audit_events
FOR EACH ROW
EXECUTE FUNCTION domain.reject_audit_event_mutation();

CREATE TRIGGER audit_events_truncate_guard
BEFORE TRUNCATE ON domain.audit_events
FOR EACH STATEMENT
EXECUTE FUNCTION domain.reject_audit_event_mutation();

CREATE FUNCTION domain.enforce_audit_tenant_head_advance()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'DELETE'
      OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
      OR NEW.last_sequence <> OLD.last_sequence + 1
      OR NEW.last_audit_event_id IS NULL
      OR NEW.last_event_hash IS NULL THEN
    RAISE EXCEPTION 'Audit Tenant head must advance exactly once'
      USING ERRCODE = 'check_violation';
  END IF;
  NEW.updated_at := CURRENT_TIMESTAMP;
  RETURN NEW;
END;
$$;

CREATE TRIGGER audit_tenant_heads_advance_guard
BEFORE UPDATE OR DELETE ON domain.audit_tenant_heads
FOR EACH ROW
EXECUTE FUNCTION domain.enforce_audit_tenant_head_advance();

CREATE TRIGGER audit_tenant_heads_truncate_guard
BEFORE TRUNCATE ON domain.audit_tenant_heads
FOR EACH STATEMENT
EXECUTE FUNCTION domain.reject_audit_event_mutation();

REVOKE TRUNCATE ON domain.audit_events, domain.audit_tenant_heads FROM PUBLIC;

CREATE FUNCTION domain.require_audit_event_covered_by_head()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  PERFORM 1
  FROM domain.audit_tenant_heads AS head
  WHERE head.tenant_id = NEW.tenant_id
    AND head.last_sequence >= NEW.tenant_sequence;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'Audit Event must be covered by the Tenant head before commit'
      USING ERRCODE = 'check_violation';
  END IF;
  RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER audit_events_covered_by_head_at_commit
AFTER INSERT ON domain.audit_events
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION domain.require_audit_event_covered_by_head();

COMMIT;

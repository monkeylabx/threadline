-- name: EnsureAuditTenantHead :exec
INSERT INTO domain.audit_tenant_heads (tenant_id)
VALUES (sqlc.arg(tenant_id))
ON CONFLICT (tenant_id) DO NOTHING;

-- name: LockAuditAppendSlot :one
SELECT
  tenant_id,
  (last_sequence + 1)::bigint AS tenant_sequence,
  CURRENT_TIMESTAMP::timestamptz AS recorded_at,
  txid_current()::bigint AS slot_transaction_id,
  last_audit_event_id AS previous_audit_event_id,
  last_event_hash AS previous_event_hash
FROM domain.audit_tenant_heads
WHERE tenant_id = sqlc.arg(tenant_id)
  AND last_sequence < 9223372036854775807
FOR UPDATE;

-- name: AppendAuditEventAndAdvanceHead :one
WITH slot_guard AS (
  SELECT 1
  WHERE sqlc.arg(slot_transaction_id)::bigint = txid_current()::bigint
    AND sqlc.arg(recorded_at)::timestamptz = CURRENT_TIMESTAMP
),
inserted_event AS (
  INSERT INTO domain.audit_events (
    tenant_id,
    audit_event_id,
    contract_version,
    tenant_sequence,
    recorded_at,
    principal_actor_type,
    principal_actor_id,
    action,
    outcome,
    reason,
    target_type,
    target_id,
    target_version,
    policy_version,
    request_id,
    approval_id,
    recovery_case_id,
    evidence_digest,
    previous_event_hash,
    event_hash
  )
  SELECT
    sqlc.arg(tenant_id),
    sqlc.arg(audit_event_id),
    1,
    sqlc.arg(tenant_sequence),
    sqlc.arg(recorded_at)::timestamptz,
    sqlc.arg(principal_actor_type),
    sqlc.arg(principal_actor_id),
    sqlc.arg(action),
    sqlc.arg(outcome),
    sqlc.arg(reason),
    sqlc.arg(target_type),
    sqlc.arg(target_id),
    sqlc.narg(target_version)::bigint,
    sqlc.arg(policy_version),
    sqlc.arg(request_id),
    sqlc.narg(approval_id)::text,
    sqlc.narg(recovery_case_id)::text,
    sqlc.narg(evidence_digest)::bytea,
    sqlc.arg(previous_event_hash),
    sqlc.arg(event_hash)
  FROM slot_guard
  RETURNING *
),
advanced_head AS (
  UPDATE domain.audit_tenant_heads AS head
  SET
    last_sequence = event.tenant_sequence,
    last_audit_event_id = event.audit_event_id,
    last_event_hash = event.event_hash
  FROM inserted_event AS event
  WHERE head.tenant_id = event.tenant_id
    AND head.last_sequence = event.tenant_sequence - 1
    AND head.last_audit_event_id IS NOT DISTINCT FROM sqlc.narg(previous_audit_event_id)::text
    AND head.last_event_hash = event.previous_event_hash
  RETURNING head.tenant_id
)
SELECT event.*
FROM inserted_event AS event
JOIN advanced_head AS head USING (tenant_id);

-- name: ObserveAuditEventIdempotency :one
SELECT
  event.*,
  (
    event.contract_version = 1
    AND event.principal_actor_type = sqlc.arg(principal_actor_type)
    AND event.principal_actor_id = sqlc.arg(principal_actor_id)
    AND event.action = sqlc.arg(action)
    AND event.outcome = sqlc.arg(outcome)
    AND event.reason = sqlc.arg(reason)
    AND event.target_type = sqlc.arg(target_type)
    AND event.target_id = sqlc.arg(target_id)
    AND event.target_version IS NOT DISTINCT FROM sqlc.narg(target_version)::bigint
    AND event.policy_version = sqlc.arg(policy_version)
    AND event.request_id = sqlc.arg(request_id)
    AND event.approval_id IS NOT DISTINCT FROM sqlc.narg(approval_id)::text
    AND event.recovery_case_id IS NOT DISTINCT FROM sqlc.narg(recovery_case_id)::text
    AND event.evidence_digest IS NOT DISTINCT FROM sqlc.narg(evidence_digest)::bytea
  )::boolean AS exact_match
FROM domain.audit_events AS event
WHERE event.tenant_id = sqlc.arg(tenant_id)
  AND event.audit_event_id = sqlc.arg(audit_event_id);

-- name: GetAuditTenantHead :one
SELECT tenant_id, last_sequence, last_audit_event_id, last_event_hash, updated_at
FROM domain.audit_tenant_heads
WHERE tenant_id = sqlc.arg(tenant_id);

-- name: GetAuditEvent :one
SELECT *
FROM domain.audit_events
WHERE tenant_id = sqlc.arg(tenant_id)
  AND audit_event_id = sqlc.arg(audit_event_id);

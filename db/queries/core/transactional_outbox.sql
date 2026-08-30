-- name: GetOutboxTransactionIsolation :one
SELECT current_setting('transaction_isolation')::text;

-- name: TryInsertDomainEventAndInitialEntry :one
WITH inserted_event AS (
  INSERT INTO domain.domain_events (
    tenant_id,
    event_id,
    event_type,
    schema_version,
    aggregate_kind,
    aggregate_id,
    payload,
    occurred_at
  )
  VALUES (
    sqlc.arg(tenant_id),
    sqlc.arg(event_id),
    sqlc.arg(event_type),
    sqlc.arg(schema_version),
    sqlc.arg(aggregate_kind),
    sqlc.arg(aggregate_id),
    sqlc.arg(payload),
    sqlc.arg(occurred_at)
  )
  ON CONFLICT (tenant_id, event_id) DO NOTHING
  RETURNING tenant_id, event_id, enqueued_at
),
inserted_entry AS (
  INSERT INTO domain.transactional_outbox (
    tenant_id,
    event_id,
    destination,
    next_attempt_at,
    policy_id,
    policy_snapshot_digest,
    effective_lease_ms,
    effective_absolute_lifetime_ms,
    effective_event_retry_ceiling,
    effective_transport_base_ms,
    effective_transport_cap_ms,
    effective_unknown_base_ms,
    effective_unknown_cap_ms,
    effective_event_base_ms,
    effective_event_cap_ms,
    effective_retention_days
  )
  SELECT
    tenant_id,
    event_id,
    'domain-events',
    CURRENT_TIMESTAMP,
    sqlc.arg(policy_id),
    sqlc.arg(policy_snapshot_digest),
    sqlc.arg(effective_lease_ms),
    sqlc.arg(effective_absolute_lifetime_ms),
    sqlc.arg(effective_event_retry_ceiling),
    sqlc.arg(effective_transport_base_ms),
    sqlc.arg(effective_transport_cap_ms),
    sqlc.arg(effective_unknown_base_ms),
    sqlc.arg(effective_unknown_cap_ms),
    sqlc.arg(effective_event_base_ms),
    sqlc.arg(effective_event_cap_ms),
    sqlc.arg(effective_retention_days)
  FROM inserted_event
  RETURNING tenant_id, event_id, outbox_entry_id
)
SELECT
  inserted_event.event_id,
  inserted_entry.outbox_entry_id,
  inserted_event.enqueued_at
FROM inserted_event
JOIN inserted_entry USING (tenant_id, event_id);

-- name: ObserveExactDomainEventAndInitialDestination :one
SELECT
  event.event_id,
  entry.outbox_entry_id,
  event.enqueued_at,
  (
    event.event_type = sqlc.arg(event_type)
    AND event.schema_version = sqlc.arg(schema_version)
    AND event.aggregate_kind = sqlc.arg(aggregate_kind)
    AND event.aggregate_id = sqlc.arg(aggregate_id)
    AND event.payload = sqlc.arg(payload)
    AND event.occurred_at = sqlc.arg(occurred_at)
    AND entry.destination = 'domain-events'
    AND (
      SELECT count(*)
      FROM domain.transactional_outbox AS destinations
      WHERE destinations.tenant_id = event.tenant_id
        AND destinations.event_id = event.event_id
    ) = 1
  )::boolean AS exact_match
FROM domain.domain_events AS event
JOIN domain.transactional_outbox AS entry
  ON entry.tenant_id = event.tenant_id
 AND entry.event_id = event.event_id
 AND entry.destination = 'domain-events'
WHERE event.tenant_id = sqlc.arg(tenant_id)
  AND event.event_id = sqlc.arg(event_id);

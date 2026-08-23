-- name: ClaimTransactionalOutboxBatch :many
WITH claimed AS (
  SELECT domain.claim_transactional_outbox_batch(
    sqlc.arg(claim_owner_id)::text,
    sqlc.arg(batch_size)::integer
  )::domain.transactional_outbox_claim_result AS value
)
SELECT
  ((claimed.value).result_code)::text AS result_code,
  ((claimed.value).tenant_id)::text AS tenant_id,
  ((claimed.value).event_id)::text AS event_id,
  ((claimed.value).outbox_entry_id)::bigint AS outbox_entry_id,
  ((claimed.value).delivery_attempt_id)::bigint AS delivery_attempt_id,
  ((claimed.value).replay_generation)::bigint AS replay_generation,
  ((claimed.value).total_attempt_number)::bigint AS total_attempt_number,
  ((claimed.value).generation_attempt_number)::bigint AS generation_attempt_number,
  ((claimed.value).claim_owner_id)::text AS claim_owner_id,
  ((claimed.value).raw_claim_token)::bytea AS raw_claim_token,
  ((claimed.value).claimed_at)::timestamp with time zone AS claimed_at,
  ((claimed.value).lease_expires_at)::timestamp with time zone AS lease_expires_at,
  ((claimed.value).absolute_lease_expires_at)::timestamp with time zone AS absolute_lease_expires_at,
  ((claimed.value).broker_message_id)::text AS broker_message_id,
  ((claimed.value).destination)::text AS destination,
  ((claimed.value).event_type)::text AS event_type,
  ((claimed.value).schema_version)::integer AS schema_version,
  ((claimed.value).aggregate_kind)::text AS aggregate_kind,
  ((claimed.value).aggregate_id)::text AS aggregate_id,
  ((claimed.value).payload)::bytea AS payload,
  ((claimed.value).occurred_at)::timestamp with time zone AS occurred_at,
  ((claimed.value).enqueued_at)::timestamp with time zone AS enqueued_at,
  ((claimed.value).policy_id)::text AS policy_id,
  ((claimed.value).policy_snapshot_digest)::bytea AS policy_snapshot_digest
FROM claimed;

-- name: RenewTransactionalOutboxClaim :one
WITH renewed AS (
  SELECT domain.renew_transactional_outbox_claim(
    sqlc.arg(tenant_id)::text,
    sqlc.arg(event_id)::text,
    sqlc.arg(outbox_entry_id)::bigint,
    sqlc.arg(delivery_attempt_id)::bigint,
    sqlc.arg(replay_generation)::bigint,
    sqlc.arg(claim_owner_id)::text,
    sqlc.arg(candidate_digest)::bytea
  )::domain.transactional_outbox_renew_result AS value
)
SELECT
  ((renewed.value).result_code)::text AS result_code,
  ((renewed.value).lease_expires_at)::timestamp with time zone AS lease_expires_at
FROM renewed;

-- name: AcknowledgeTransactionalOutboxPublished :one
SELECT domain.acknowledge_transactional_outbox_published(
  sqlc.arg(tenant_id)::text,
  sqlc.arg(event_id)::text,
  sqlc.arg(outbox_entry_id)::bigint,
  sqlc.arg(delivery_attempt_id)::bigint,
  sqlc.arg(replay_generation)::bigint,
  sqlc.arg(claim_owner_id)::text,
  sqlc.arg(candidate_digest)::bytea,
  sqlc.arg(broker_stream)::text,
  sqlc.arg(broker_sequence)::numeric,
  sqlc.arg(broker_duplicate)::boolean,
  sqlc.arg(broker_message_id)::text
) AS result_code;

-- name: RecordTransactionalOutboxPublishFailure :one
WITH failed AS (
  SELECT domain.record_transactional_outbox_publish_failure(
    sqlc.arg(tenant_id)::text,
    sqlc.arg(event_id)::text,
    sqlc.arg(outbox_entry_id)::bigint,
    sqlc.arg(delivery_attempt_id)::bigint,
    sqlc.arg(replay_generation)::bigint,
    sqlc.arg(claim_owner_id)::text,
    sqlc.arg(candidate_digest)::bytea,
    sqlc.arg(failure_code)::text
  )::domain.transactional_outbox_failure_result AS value
)
SELECT
  ((failed.value).result_code)::text AS result_code,
  ((failed.value).next_attempt_at)::timestamp with time zone AS next_attempt_at
FROM failed;

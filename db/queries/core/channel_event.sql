-- name: LockConversationSequenceHead :one
SELECT tenant_id, conversation_kind, conversation_id, last_seq
FROM domain.conversation_sequence_heads
WHERE tenant_id = sqlc.arg(tenant_id)
  AND conversation_kind = sqlc.arg(conversation_kind)
  AND conversation_id = sqlc.arg(conversation_id)
FOR UPDATE;

-- name: InsertConversationSequenceHead :one
INSERT INTO domain.conversation_sequence_heads (tenant_id, conversation_kind, conversation_id)
VALUES (sqlc.arg(tenant_id), sqlc.arg(conversation_kind), sqlc.arg(conversation_id))
RETURNING tenant_id, conversation_kind, conversation_id, last_seq;

-- name: AdvanceConversationSequenceHead :one
UPDATE domain.conversation_sequence_heads
SET last_seq = last_seq + 1,
    updated_at = CURRENT_TIMESTAMP
WHERE tenant_id = sqlc.arg(tenant_id)
  AND conversation_kind = sqlc.arg(conversation_kind)
  AND conversation_id = sqlc.arg(conversation_id)
  AND last_seq = sqlc.arg(expected_last_seq)
RETURNING last_seq;

-- name: InsertChannelEvent :one
INSERT INTO domain.channel_events (
  tenant_id,
  event_id,
  channel_id,
  dm_id,
  channel_seq,
  sender_actor_type,
  sender_actor_id,
  sender_device_id,
  idempotency_key,
  category,
  envelope,
  content_hash
)
VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(event_id),
  sqlc.narg(channel_id),
  sqlc.narg(dm_id),
  sqlc.arg(channel_seq),
  sqlc.arg(sender_actor_type),
  sqlc.arg(sender_actor_id),
  sqlc.arg(sender_device_id),
  sqlc.arg(idempotency_key),
  sqlc.arg(category),
  sqlc.arg(envelope),
  sqlc.arg(content_hash)
)
RETURNING tenant_id, event_id, channel_seq, committed_at;

-- name: InsertSendCommand :one
INSERT INTO domain.send_commands (
  tenant_id,
  sender_device_id,
  idempotency_key,
  event_id,
  envelope_digest
)
VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(sender_device_id),
  sqlc.arg(idempotency_key),
  sqlc.arg(event_id),
  sqlc.arg(envelope_digest)
)
RETURNING tenant_id, sender_device_id, idempotency_key, event_id, bound_at;

-- name: GetSendCommandBinding :one
SELECT
  commands.tenant_id,
  commands.sender_device_id,
  commands.idempotency_key,
  commands.event_id,
  commands.envelope_digest,
  events.channel_id,
  events.dm_id,
  events.channel_seq,
  events.committed_at,
  events.envelope
FROM domain.send_commands AS commands
JOIN domain.channel_events AS events
  ON events.tenant_id = commands.tenant_id
  AND events.event_id = commands.event_id
WHERE commands.tenant_id = sqlc.arg(tenant_id)
  AND commands.sender_device_id = sqlc.arg(sender_device_id)
  AND commands.idempotency_key = sqlc.arg(idempotency_key);

-- name: GetChannelEvent :one
SELECT tenant_id, event_id, channel_id, dm_id, channel_seq, sender_actor_type, sender_actor_id,
  sender_device_id, idempotency_key, category, envelope, content_hash, committed_at
FROM domain.channel_events
WHERE tenant_id = sqlc.arg(tenant_id)
  AND event_id = sqlc.arg(event_id);

-- name: ListChannelEventsOldestFirst :many
SELECT tenant_id, event_id, channel_id, dm_id, channel_seq, sender_actor_type, sender_actor_id,
  sender_device_id, idempotency_key, category, envelope, content_hash, committed_at
FROM domain.channel_events
WHERE tenant_id = sqlc.arg(tenant_id)
  AND channel_id = sqlc.arg(channel_id)
  AND channel_seq > sqlc.arg(after_seq)
ORDER BY channel_seq ASC
LIMIT sqlc.arg(page_limit);

-- name: ListChannelEventsNewestFirst :many
SELECT tenant_id, event_id, channel_id, dm_id, channel_seq, sender_actor_type, sender_actor_id,
  sender_device_id, idempotency_key, category, envelope, content_hash, committed_at
FROM domain.channel_events
WHERE tenant_id = sqlc.arg(tenant_id)
  AND channel_id = sqlc.arg(channel_id)
  AND channel_seq < sqlc.arg(before_seq)
ORDER BY channel_seq DESC
LIMIT sqlc.arg(page_limit);

-- name: GetConversationSequenceHead :one
SELECT tenant_id, conversation_kind, conversation_id, last_seq
FROM domain.conversation_sequence_heads
WHERE tenant_id = sqlc.arg(tenant_id)
  AND conversation_kind = sqlc.arg(conversation_kind)
  AND conversation_id = sqlc.arg(conversation_id);

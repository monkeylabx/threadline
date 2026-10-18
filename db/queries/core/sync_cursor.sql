-- name: GetSyncCursorForUpdate :one
SELECT tenant_id, device_id, conversation_kind, conversation_id, actor_type, actor_id, last_applied_seq, updated_at
FROM domain.sync_cursors
WHERE tenant_id = sqlc.arg(tenant_id)
  AND device_id = sqlc.arg(device_id)
  AND conversation_kind = sqlc.arg(conversation_kind)
  AND conversation_id = sqlc.arg(conversation_id)
FOR UPDATE;

-- name: InsertSyncCursor :one
INSERT INTO domain.sync_cursors (
  tenant_id, device_id, conversation_kind, conversation_id, actor_type, actor_id, last_applied_seq
)
VALUES (
  sqlc.arg(tenant_id), sqlc.arg(device_id), sqlc.arg(conversation_kind), sqlc.arg(conversation_id),
  sqlc.arg(actor_type), sqlc.arg(actor_id), sqlc.arg(last_applied_seq)
)
RETURNING tenant_id, device_id, conversation_kind, conversation_id, actor_type, actor_id, last_applied_seq, updated_at;

-- name: AdvanceSyncCursor :one
UPDATE domain.sync_cursors
SET last_applied_seq = sqlc.arg(last_applied_seq),
    updated_at = CURRENT_TIMESTAMP
WHERE tenant_id = sqlc.arg(tenant_id)
  AND device_id = sqlc.arg(device_id)
  AND conversation_kind = sqlc.arg(conversation_kind)
  AND conversation_id = sqlc.arg(conversation_id)
  AND last_applied_seq <= sqlc.arg(last_applied_seq)
RETURNING tenant_id, device_id, conversation_kind, conversation_id, actor_type, actor_id, last_applied_seq, updated_at;

-- name: ListDeviceSyncCursors :many
SELECT tenant_id, device_id, conversation_kind, conversation_id, actor_type, actor_id, last_applied_seq, updated_at
FROM domain.sync_cursors
WHERE tenant_id = sqlc.arg(tenant_id)
  AND device_id = sqlc.arg(device_id)
ORDER BY conversation_kind, conversation_id;

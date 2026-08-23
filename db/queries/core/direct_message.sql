-- name: CreateDirectMessage :one
INSERT INTO domain.direct_messages (
  tenant_id,
  dm_id,
  e2ee_group_id
) VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(dm_id),
  sqlc.arg(e2ee_group_id)
)
RETURNING tenant_id, dm_id, e2ee_group_id, participants_sealed, created_at;

-- name: FinalizeDirectMessageParticipants :one
UPDATE domain.direct_messages
SET participants_sealed = TRUE
WHERE tenant_id = sqlc.arg(tenant_id)
  AND dm_id = sqlc.arg(dm_id)
  AND NOT participants_sealed
RETURNING tenant_id, dm_id, e2ee_group_id, participants_sealed, created_at;

-- name: GetDirectMessage :one
SELECT tenant_id, dm_id, e2ee_group_id, participants_sealed, created_at
FROM domain.direct_messages
WHERE tenant_id = sqlc.arg(tenant_id)
  AND dm_id = sqlc.arg(dm_id);

-- name: AddDirectMessageParticipant :one
INSERT INTO domain.direct_message_participants (
  tenant_id,
  dm_id,
  actor_type,
  actor_id
) VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(dm_id),
  sqlc.arg(actor_type),
  sqlc.arg(actor_id)
)
RETURNING tenant_id, dm_id, actor_type, actor_id;

-- name: ListDirectMessageParticipants :many
SELECT tenant_id, dm_id, actor_type, actor_id
FROM domain.direct_message_participants
WHERE tenant_id = sqlc.arg(tenant_id)
  AND dm_id = sqlc.arg(dm_id)
ORDER BY actor_type, actor_id;

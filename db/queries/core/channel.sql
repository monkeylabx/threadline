-- name: CreateChannel :one
INSERT INTO domain.channels (
  tenant_id,
  channel_id,
  space_id,
  name,
  visibility,
  state,
  e2ee_group_id
) VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(channel_id),
  sqlc.arg(space_id),
  sqlc.arg(name),
  sqlc.arg(visibility),
  sqlc.arg(state),
  sqlc.arg(e2ee_group_id)
)
RETURNING tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id, created_at;

-- name: GetChannel :one
SELECT tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id, created_at
FROM domain.channels
WHERE tenant_id = sqlc.arg(tenant_id)
  AND channel_id = sqlc.arg(channel_id);

-- name: UpdateChannelNameState :one
UPDATE domain.channels
SET
  name = sqlc.arg(name),
  state = sqlc.arg(state)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND channel_id = sqlc.arg(channel_id)
RETURNING tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id, created_at;

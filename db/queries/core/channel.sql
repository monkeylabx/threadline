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

-- name: ArchiveActiveChannel :one
UPDATE domain.channels
SET state = 2
WHERE tenant_id = sqlc.arg(tenant_id)
  AND channel_id = sqlc.arg(channel_id)
  AND state = 1
RETURNING tenant_id, channel_id, state;

-- name: UpdateChannelNameState :one
UPDATE domain.channels
SET
  name = sqlc.arg(name),
  state = sqlc.arg(state)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND channel_id = sqlc.arg(channel_id)
RETURNING tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id, created_at;

-- name: ListActorActiveChannels :many
SELECT channels.tenant_id, channels.channel_id, channels.space_id, channels.name, channels.visibility,
  channels.state, channels.e2ee_group_id, channels.created_at
FROM domain.channel_memberships AS memberships
JOIN domain.channels AS channels
  ON channels.tenant_id = memberships.tenant_id
  AND channels.channel_id = memberships.channel_id
WHERE memberships.tenant_id = sqlc.arg(tenant_id)
  AND memberships.actor_type = sqlc.arg(actor_type)
  AND memberships.actor_id = sqlc.arg(actor_id)
  AND memberships.left_at IS NULL
  AND (sqlc.narg(space_id)::text IS NULL OR channels.space_id = sqlc.narg(space_id)::text)
  AND channels.channel_id > sqlc.arg(after_channel_id)
ORDER BY channels.channel_id ASC
LIMIT sqlc.arg(page_limit);

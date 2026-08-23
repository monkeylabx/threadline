-- name: CreateActiveChannelMembership :one
INSERT INTO domain.channel_memberships (
  tenant_id,
  channel_id,
  actor_type,
  actor_id,
  role
) VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(channel_id),
  sqlc.arg(actor_type),
  sqlc.arg(actor_id),
  sqlc.arg(role)
)
RETURNING interval_id, tenant_id, channel_id, actor_type, actor_id, role, joined_at, left_at;

-- name: GetActiveChannelMembership :one
SELECT interval_id, tenant_id, channel_id, actor_type, actor_id, role, joined_at, left_at
FROM domain.channel_memberships
WHERE tenant_id = sqlc.arg(tenant_id)
  AND channel_id = sqlc.arg(channel_id)
  AND actor_type = sqlc.arg(actor_type)
  AND actor_id = sqlc.arg(actor_id)
  AND left_at IS NULL;

-- name: EndActiveChannelMembership :one
UPDATE domain.channel_memberships
SET left_at = CURRENT_TIMESTAMP
WHERE tenant_id = sqlc.arg(tenant_id)
  AND channel_id = sqlc.arg(channel_id)
  AND actor_type = sqlc.arg(actor_type)
  AND actor_id = sqlc.arg(actor_id)
  AND left_at IS NULL
RETURNING interval_id, tenant_id, channel_id, actor_type, actor_id, role, joined_at, left_at;

-- name: ListActiveChannelMemberships :many
SELECT interval_id, tenant_id, channel_id, actor_type, actor_id, role, joined_at, left_at
FROM domain.channel_memberships
WHERE tenant_id = sqlc.arg(tenant_id)
  AND channel_id = sqlc.arg(channel_id)
  AND left_at IS NULL
ORDER BY actor_type, actor_id;

-- name: ListChannelMembershipHistory :many
SELECT interval_id, tenant_id, channel_id, actor_type, actor_id, role, joined_at, left_at
FROM domain.channel_memberships
WHERE tenant_id = sqlc.arg(tenant_id)
  AND channel_id = sqlc.arg(channel_id)
ORDER BY joined_at, interval_id;

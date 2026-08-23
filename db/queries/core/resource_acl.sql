-- name: LockSpaceForACLReplacement :one
SELECT space_id
FROM domain.spaces
WHERE tenant_id = sqlc.arg(tenant_id)
  AND space_id = sqlc.arg(resource_id)
FOR NO KEY UPDATE;

-- name: LockChannelForACLReplacement :one
SELECT channel_id
FROM domain.channels
WHERE tenant_id = sqlc.arg(tenant_id)
  AND channel_id = sqlc.arg(resource_id)
FOR NO KEY UPDATE;

-- name: CreateResourceACLSnapshot :one
INSERT INTO domain.resource_acl_snapshots (
  tenant_id,
  resource_kind,
  resource_id,
  space_id,
  channel_id,
  default_effect
) VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(resource_kind),
  sqlc.arg(resource_id),
  sqlc.narg(space_id),
  sqlc.narg(channel_id),
  sqlc.arg(default_effect)
)
RETURNING acl_version, tenant_id, resource_kind, resource_id, space_id, channel_id,
  default_effect, entries_sealed, created_at;

-- name: CreateResourceACLEntry :exec
INSERT INTO domain.resource_acl_entries (
  tenant_id,
  acl_version,
  entry_ordinal,
  actor_type,
  actor_id,
  action,
  effect
) VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(acl_version),
  sqlc.arg(entry_ordinal),
  sqlc.arg(actor_type),
  sqlc.arg(actor_id),
  sqlc.arg(action),
  sqlc.arg(effect)
);

-- name: SealResourceACLSnapshot :one
UPDATE domain.resource_acl_snapshots
SET entries_sealed = TRUE
WHERE tenant_id = sqlc.arg(tenant_id)
  AND acl_version = sqlc.arg(acl_version)
  AND NOT entries_sealed
RETURNING acl_version;

-- name: SetCurrentResourceACL :exec
INSERT INTO domain.resource_acl_heads (
  tenant_id,
  resource_kind,
  resource_id,
  space_id,
  channel_id,
  current_acl_version
) VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(resource_kind),
  sqlc.arg(resource_id),
  sqlc.narg(space_id),
  sqlc.narg(channel_id),
  sqlc.arg(acl_version)
)
ON CONFLICT (tenant_id, resource_kind, resource_id) DO UPDATE
SET current_acl_version = EXCLUDED.current_acl_version;

-- name: GetCurrentResourceACLSnapshot :one
SELECT
  snapshot.acl_version,
  snapshot.default_effect
FROM domain.resource_acl_heads AS head
JOIN domain.resource_acl_snapshots AS snapshot
  ON snapshot.tenant_id = head.tenant_id
 AND snapshot.acl_version = head.current_acl_version
 AND snapshot.resource_kind = head.resource_kind
 AND snapshot.resource_id = head.resource_id
WHERE head.tenant_id = sqlc.arg(tenant_id)
  AND head.resource_kind = sqlc.arg(resource_kind)
  AND head.resource_id = sqlc.arg(resource_id)
  AND snapshot.entries_sealed;

-- name: ListResourceACLEntries :many
SELECT actor_type, actor_id, action, effect
FROM domain.resource_acl_entries
WHERE tenant_id = sqlc.arg(tenant_id)
  AND acl_version = sqlc.arg(acl_version)
ORDER BY entry_ordinal;

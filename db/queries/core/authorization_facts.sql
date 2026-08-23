-- name: GetAuthorizationTransactionIsolation :one
SELECT current_setting('transaction_isolation')::text AS transaction_isolation;

-- name: LockAuthorizationSpace :one
SELECT tenant_id, space_id
FROM domain.spaces
WHERE tenant_id = sqlc.arg(tenant_id)
  AND space_id = sqlc.arg(resource_id)
FOR UPDATE;

-- name: LockAuthorizationChannel :one
SELECT tenant_id, channel_id, visibility, state
FROM domain.channels
WHERE tenant_id = sqlc.arg(tenant_id)
  AND channel_id = sqlc.arg(resource_id)
FOR UPDATE;

-- name: LockAuthorizationOrganization :one
SELECT tenant_id, state, policy_version
FROM domain.organizations
WHERE tenant_id = sqlc.arg(tenant_id)
FOR SHARE;

-- name: LockAuthorizationMember :one
SELECT tenant_id, actor_type, actor_id, role, state
FROM domain.members
WHERE tenant_id = sqlc.arg(tenant_id)
  AND actor_type = sqlc.arg(actor_type)
  AND actor_id = sqlc.arg(actor_id)
FOR SHARE;

-- name: LockActiveAuthorizationChannelMembership :one
SELECT interval_id, tenant_id, channel_id, actor_type, actor_id, role
FROM domain.channel_memberships
WHERE tenant_id = sqlc.arg(tenant_id)
  AND channel_id = sqlc.arg(channel_id)
  AND actor_type = sqlc.arg(actor_type)
  AND actor_id = sqlc.arg(actor_id)
  AND left_at IS NULL
FOR SHARE;

-- name: LockCurrentAuthorizationACL :one
SELECT current_acl_version
FROM domain.resource_acl_heads
WHERE tenant_id = sqlc.arg(tenant_id)
  AND resource_kind = sqlc.arg(resource_kind)
  AND resource_id = sqlc.arg(resource_id)
FOR SHARE;

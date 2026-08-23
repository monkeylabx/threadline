-- name: CreateMember :one
INSERT INTO domain.members (
  tenant_id,
  actor_type,
  actor_id,
  display_name,
  role,
  state,
  org_unit_path
) VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(actor_type),
  sqlc.arg(actor_id),
  sqlc.arg(display_name),
  sqlc.arg(role),
  sqlc.arg(state),
  sqlc.narg(org_unit_path)
)
RETURNING tenant_id, actor_type, actor_id, display_name, role, state, org_unit_path, joined_at;

-- name: GetMember :one
SELECT tenant_id, actor_type, actor_id, display_name, role, state, org_unit_path, joined_at
FROM domain.members
WHERE tenant_id = sqlc.arg(tenant_id)
  AND actor_type = sqlc.arg(actor_type)
  AND actor_id = sqlc.arg(actor_id);

-- name: UpdateMemberRoleState :one
UPDATE domain.members
SET
  role = sqlc.arg(role),
  state = sqlc.arg(state)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND actor_type = sqlc.arg(actor_type)
  AND actor_id = sqlc.arg(actor_id)
RETURNING tenant_id, actor_type, actor_id, display_name, role, state, org_unit_path, joined_at;

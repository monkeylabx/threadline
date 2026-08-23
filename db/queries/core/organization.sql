-- name: CreateOrganization :one
INSERT INTO domain.organizations (
  tenant_id,
  display_name,
  state,
  policy_version
) VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(display_name),
  sqlc.arg(state),
  sqlc.arg(policy_version)
)
RETURNING tenant_id, display_name, state, policy_version, created_at;

-- name: GetOrganization :one
SELECT tenant_id, display_name, state, policy_version, created_at
FROM domain.organizations
WHERE tenant_id = sqlc.arg(tenant_id);

-- name: UpdateOrganizationStatePolicy :one
UPDATE domain.organizations
SET
  state = sqlc.arg(state),
  policy_version = sqlc.arg(policy_version)
WHERE tenant_id = sqlc.arg(tenant_id)
RETURNING tenant_id, display_name, state, policy_version, created_at;

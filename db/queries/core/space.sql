-- name: CreateSpace :one
INSERT INTO domain.spaces (
  tenant_id,
  space_id,
  display_name,
  discoverable
) VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(space_id),
  sqlc.arg(display_name),
  sqlc.arg(discoverable)
)
RETURNING tenant_id, space_id, display_name, discoverable, created_at;

-- name: GetSpace :one
SELECT tenant_id, space_id, display_name, discoverable, created_at
FROM domain.spaces
WHERE tenant_id = sqlc.arg(tenant_id)
  AND space_id = sqlc.arg(space_id);

-- name: UpdateSpaceDirectoryMetadata :one
UPDATE domain.spaces
SET
  display_name = sqlc.arg(display_name),
  discoverable = sqlc.arg(discoverable)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND space_id = sqlc.arg(space_id)
RETURNING tenant_id, space_id, display_name, discoverable, created_at;

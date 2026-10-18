-- Slice A two-instance dev seed (ADR-0008). Idempotent: safe to re-run.
-- One tenant, two human Members on two Devices, one Space, one Channel, both
-- Members joined, and a sealed allow-by-default ACL so channel.read and
-- channel.publish evaluate to allow on current facts. Dev stack only.
BEGIN;

INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
VALUES ('tenant-dev', '北辰科技（dev）', 1, 'policy-dev-v1')
ON CONFLICT (tenant_id) DO NOTHING;

INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state)
VALUES
  ('tenant-dev', 1, 'actor-lin-jia', '林嘉', 4, 2),
  ('tenant-dev', 1, 'actor-meng-qing', '孟青', 4, 2)
ON CONFLICT DO NOTHING;

INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable)
VALUES ('tenant-dev', 'space-dev', '研发', TRUE)
ON CONFLICT DO NOTHING;

INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
VALUES ('tenant-dev', 'channel-product', 'space-dev', '产品研发', 2, 1, 'group-channel-product-dev')
ON CONFLICT DO NOTHING;

INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
SELECT 'tenant-dev', 'channel-product', 1, actor_id, 3
FROM (VALUES ('actor-lin-jia'), ('actor-meng-qing')) AS seed (actor_id)
WHERE NOT EXISTS (
  SELECT 1 FROM domain.channel_memberships AS existing
  WHERE existing.tenant_id = 'tenant-dev' AND existing.channel_id = 'channel-product'
    AND existing.actor_type = 1 AND existing.actor_id = seed.actor_id AND existing.left_at IS NULL
);

-- ACL: one sealed snapshot with default allow, installed as the current head.
DO $$
DECLARE
  version bigint;
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM domain.resource_acl_heads
    WHERE tenant_id = 'tenant-dev' AND resource_kind = 2 AND resource_id = 'channel-product'
  ) THEN
    INSERT INTO domain.resource_acl_snapshots (tenant_id, resource_kind, resource_id, channel_id, default_effect)
    VALUES ('tenant-dev', 2, 'channel-product', 'channel-product', 1)
    RETURNING acl_version INTO version;
    UPDATE domain.resource_acl_snapshots SET entries_sealed = TRUE
    WHERE tenant_id = 'tenant-dev' AND acl_version = version;
    INSERT INTO domain.resource_acl_heads (tenant_id, resource_kind, resource_id, channel_id, current_acl_version)
    VALUES ('tenant-dev', 2, 'channel-product', 'channel-product', version);
  END IF;
END $$;

COMMIT;

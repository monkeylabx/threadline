BEGIN;

DROP TABLE domain.resource_acl_heads;
DROP TABLE domain.resource_acl_entries;
DROP TABLE domain.resource_acl_snapshots;
DROP FUNCTION domain.enforce_resource_acl_head_lifecycle();
DROP FUNCTION domain.enforce_resource_acl_entry_lifecycle();
DROP FUNCTION domain.require_resource_acl_snapshot_sealed();
DROP FUNCTION domain.enforce_resource_acl_snapshot_lifecycle();

COMMIT;

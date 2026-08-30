BEGIN;

CREATE TABLE domain.resource_acl_snapshots (
  acl_version bigint GENERATED ALWAYS AS IDENTITY,
  tenant_id text NOT NULL,
  resource_kind smallint NOT NULL,
  resource_id text NOT NULL,
  space_id text,
  channel_id text,
  default_effect smallint NOT NULL,
  entries_sealed boolean NOT NULL DEFAULT FALSE,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (acl_version),
  CONSTRAINT resource_acl_snapshots_tenant_version_unique
    UNIQUE (tenant_id, acl_version),
  CONSTRAINT resource_acl_snapshots_exact_resource_unique
    UNIQUE (tenant_id, acl_version, resource_kind, resource_id),
  CONSTRAINT resource_acl_snapshots_space_fk
    FOREIGN KEY (tenant_id, space_id)
      REFERENCES domain.spaces (tenant_id, space_id),
  CONSTRAINT resource_acl_snapshots_channel_fk
    FOREIGN KEY (tenant_id, channel_id)
      REFERENCES domain.channels (tenant_id, channel_id),
  CONSTRAINT resource_acl_snapshots_tenant_not_blank
    CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
  CONSTRAINT resource_acl_snapshots_resource_kind_known
    CHECK (resource_kind IN (1, 2)),
  CONSTRAINT resource_acl_snapshots_resource_id_not_blank
    CHECK (resource_id <> '' AND resource_id = btrim(resource_id)),
  CONSTRAINT resource_acl_snapshots_default_effect_known
    CHECK (default_effect IN (1, 2)),
  CONSTRAINT resource_acl_snapshots_exactly_one_typed_resource
    CHECK (
      (resource_kind = 1 AND space_id IS NOT NULL AND space_id = resource_id AND channel_id IS NULL)
      OR
      (resource_kind = 2 AND channel_id IS NOT NULL AND channel_id = resource_id AND space_id IS NULL)
    )
);

CREATE TABLE domain.resource_acl_entries (
  tenant_id text NOT NULL,
  acl_version bigint NOT NULL,
  entry_ordinal integer NOT NULL,
  actor_type smallint NOT NULL,
  actor_id text NOT NULL,
  action smallint NOT NULL,
  effect smallint NOT NULL,
  PRIMARY KEY (tenant_id, acl_version, entry_ordinal),
  CONSTRAINT resource_acl_entries_exact_unique
    UNIQUE (tenant_id, acl_version, actor_type, actor_id, action, effect),
  CONSTRAINT resource_acl_entries_snapshot_fk
    FOREIGN KEY (tenant_id, acl_version)
      REFERENCES domain.resource_acl_snapshots (tenant_id, acl_version),
  CONSTRAINT resource_acl_entries_member_fk
    FOREIGN KEY (tenant_id, actor_type, actor_id)
      REFERENCES domain.members (tenant_id, actor_type, actor_id),
  CONSTRAINT resource_acl_entries_ordinal_positive
    CHECK (entry_ordinal > 0),
  CONSTRAINT resource_acl_entries_actor_type_known
    CHECK (actor_type IN (1, 2, 3)),
  CONSTRAINT resource_acl_entries_actor_id_not_blank
    CHECK (actor_id <> '' AND actor_id = btrim(actor_id)),
  CONSTRAINT resource_acl_entries_action_known
    CHECK (action BETWEEN 1 AND 11),
  CONSTRAINT resource_acl_entries_effect_known
    CHECK (effect IN (1, 2))
);

CREATE TABLE domain.resource_acl_heads (
  tenant_id text NOT NULL,
  resource_kind smallint NOT NULL,
  resource_id text NOT NULL,
  space_id text,
  channel_id text,
  current_acl_version bigint NOT NULL,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, resource_kind, resource_id),
  CONSTRAINT resource_acl_heads_space_fk
    FOREIGN KEY (tenant_id, space_id)
      REFERENCES domain.spaces (tenant_id, space_id),
  CONSTRAINT resource_acl_heads_channel_fk
    FOREIGN KEY (tenant_id, channel_id)
      REFERENCES domain.channels (tenant_id, channel_id),
  CONSTRAINT resource_acl_heads_exact_snapshot_fk
    FOREIGN KEY (tenant_id, current_acl_version, resource_kind, resource_id)
      REFERENCES domain.resource_acl_snapshots (tenant_id, acl_version, resource_kind, resource_id),
  CONSTRAINT resource_acl_heads_resource_kind_known
    CHECK (resource_kind IN (1, 2)),
  CONSTRAINT resource_acl_heads_resource_id_not_blank
    CHECK (resource_id <> '' AND resource_id = btrim(resource_id)),
  CONSTRAINT resource_acl_heads_exactly_one_typed_resource
    CHECK (
      (resource_kind = 1 AND space_id IS NOT NULL AND space_id = resource_id AND channel_id IS NULL)
      OR
      (resource_kind = 2 AND channel_id IS NOT NULL AND channel_id = resource_id AND space_id IS NULL)
    )
);

CREATE FUNCTION domain.enforce_resource_acl_snapshot_lifecycle()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'Resource ACL snapshots cannot be deleted'
      USING ERRCODE = 'check_violation';
  END IF;
  IF OLD.entries_sealed
      OR NOT NEW.entries_sealed
      OR OLD.acl_version IS DISTINCT FROM NEW.acl_version
      OR OLD.tenant_id IS DISTINCT FROM NEW.tenant_id
      OR OLD.resource_kind IS DISTINCT FROM NEW.resource_kind
      OR OLD.resource_id IS DISTINCT FROM NEW.resource_id
      OR OLD.space_id IS DISTINCT FROM NEW.space_id
      OR OLD.channel_id IS DISTINCT FROM NEW.channel_id
      OR OLD.default_effect IS DISTINCT FROM NEW.default_effect
      OR OLD.created_at IS DISTINCT FROM NEW.created_at THEN
    RAISE EXCEPTION 'Resource ACL snapshot facts are immutable'
      USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER resource_acl_snapshots_lifecycle_guard
BEFORE UPDATE OR DELETE ON domain.resource_acl_snapshots
FOR EACH ROW
EXECUTE FUNCTION domain.enforce_resource_acl_snapshot_lifecycle();

CREATE FUNCTION domain.require_resource_acl_snapshot_sealed()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  PERFORM 1
  FROM domain.resource_acl_snapshots
  WHERE tenant_id = NEW.tenant_id
    AND acl_version = NEW.acl_version
    AND entries_sealed;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'Resource ACL snapshot must be sealed before commit'
      USING ERRCODE = 'check_violation';
  END IF;
  RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER resource_acl_snapshots_sealed_at_commit
AFTER INSERT ON domain.resource_acl_snapshots
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION domain.require_resource_acl_snapshot_sealed();

CREATE FUNCTION domain.enforce_resource_acl_entry_lifecycle()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP <> 'INSERT' THEN
    RAISE EXCEPTION 'Resource ACL entries are immutable'
      USING ERRCODE = 'check_violation';
  END IF;
  PERFORM 1
  FROM domain.resource_acl_snapshots
  WHERE tenant_id = NEW.tenant_id
    AND acl_version = NEW.acl_version
    AND NOT entries_sealed
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'Resource ACL entries require an unsealed snapshot'
      USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER resource_acl_entries_lifecycle_guard
BEFORE INSERT OR UPDATE OR DELETE ON domain.resource_acl_entries
FOR EACH ROW
EXECUTE FUNCTION domain.enforce_resource_acl_entry_lifecycle();

CREATE FUNCTION domain.enforce_resource_acl_head_lifecycle()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'Resource ACL current heads cannot be deleted'
      USING ERRCODE = 'check_violation';
  END IF;
  IF TG_OP = 'UPDATE' AND (
      OLD.tenant_id IS DISTINCT FROM NEW.tenant_id
      OR OLD.resource_kind IS DISTINCT FROM NEW.resource_kind
      OR OLD.resource_id IS DISTINCT FROM NEW.resource_id
      OR OLD.space_id IS DISTINCT FROM NEW.space_id
      OR OLD.channel_id IS DISTINCT FROM NEW.channel_id
    ) THEN
    RAISE EXCEPTION 'Resource ACL head identity cannot be changed'
      USING ERRCODE = 'check_violation';
  END IF;
  PERFORM 1
  FROM domain.resource_acl_snapshots
  WHERE tenant_id = NEW.tenant_id
    AND acl_version = NEW.current_acl_version
    AND resource_kind = NEW.resource_kind
    AND resource_id = NEW.resource_id
    AND entries_sealed
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'Resource ACL head requires a sealed exact snapshot'
      USING ERRCODE = 'check_violation';
  END IF;
  NEW.updated_at := CURRENT_TIMESTAMP;
  RETURN NEW;
END;
$$;

CREATE TRIGGER resource_acl_heads_lifecycle_guard
BEFORE INSERT OR UPDATE OR DELETE ON domain.resource_acl_heads
FOR EACH ROW
EXECUTE FUNCTION domain.enforce_resource_acl_head_lifecycle();

COMMIT;

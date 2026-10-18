CREATE TABLE domain.sync_cursors (
  tenant_id text NOT NULL,
  device_id text NOT NULL,
  conversation_kind smallint NOT NULL,
  conversation_id text NOT NULL,
  actor_type smallint NOT NULL,
  actor_id text NOT NULL,
  last_applied_seq bigint NOT NULL DEFAULT 0,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, device_id, conversation_kind, conversation_id),
  CONSTRAINT sync_cursors_organization_fk
    FOREIGN KEY (tenant_id)
      REFERENCES domain.organizations (tenant_id),
  CONSTRAINT sync_cursors_member_fk
    FOREIGN KEY (tenant_id, actor_type, actor_id)
      REFERENCES domain.members (tenant_id, actor_type, actor_id),
  CONSTRAINT sync_cursors_tenant_id_not_blank
    CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
  CONSTRAINT sync_cursors_device_id_not_blank
    CHECK (device_id <> '' AND device_id = btrim(device_id)),
  CONSTRAINT sync_cursors_kind_known
    CHECK (conversation_kind IN (1, 2)),
  CONSTRAINT sync_cursors_conversation_id_not_blank
    CHECK (conversation_id <> '' AND conversation_id = btrim(conversation_id)),
  CONSTRAINT sync_cursors_actor_id_not_blank
    CHECK (actor_id <> '' AND actor_id = btrim(actor_id)),
  CONSTRAINT sync_cursors_last_applied_seq_non_negative
    CHECK (last_applied_seq >= 0)
);

CREATE FUNCTION domain.enforce_sync_cursor_lifecycle()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'Sync cursors cannot be deleted'
      USING ERRCODE = 'check_violation';
  END IF;
  IF NEW.tenant_id <> OLD.tenant_id
      OR NEW.device_id <> OLD.device_id
      OR NEW.conversation_kind <> OLD.conversation_kind
      OR NEW.conversation_id <> OLD.conversation_id
      OR NEW.actor_type <> OLD.actor_type
      OR NEW.actor_id <> OLD.actor_id THEN
    RAISE EXCEPTION 'Sync cursor identity is immutable'
      USING ERRCODE = 'check_violation';
  END IF;
  IF NEW.last_applied_seq < OLD.last_applied_seq THEN
    RAISE EXCEPTION 'Sync cursors never regress'
      USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER sync_cursors_lifecycle_guard
BEFORE UPDATE OR DELETE ON domain.sync_cursors
FOR EACH ROW
EXECUTE FUNCTION domain.enforce_sync_cursor_lifecycle();

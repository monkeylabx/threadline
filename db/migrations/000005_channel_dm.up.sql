BEGIN;

CREATE TABLE domain.channels (
  tenant_id text NOT NULL,
  channel_id text NOT NULL,
  space_id text NOT NULL,
  name text NOT NULL,
  visibility smallint NOT NULL,
  state smallint NOT NULL,
  e2ee_group_id text NOT NULL,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, channel_id),
  CONSTRAINT channels_space_fk
    FOREIGN KEY (tenant_id, space_id) REFERENCES domain.spaces (tenant_id, space_id),
  CONSTRAINT channels_e2ee_group_unique
    UNIQUE (tenant_id, e2ee_group_id),
  CONSTRAINT channels_tenant_id_not_blank
    CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
  CONSTRAINT channels_channel_id_not_blank
    CHECK (channel_id <> '' AND channel_id = btrim(channel_id)),
  CONSTRAINT channels_space_id_not_blank
    CHECK (space_id <> '' AND space_id = btrim(space_id)),
  CONSTRAINT channels_name_not_blank
    CHECK (name <> '' AND name = btrim(name)),
  CONSTRAINT channels_visibility_known
    CHECK (visibility IN (1, 2)),
  CONSTRAINT channels_state_known
    CHECK (state IN (1, 2, 3)),
  CONSTRAINT channels_e2ee_group_id_not_blank
    CHECK (e2ee_group_id <> '' AND e2ee_group_id = btrim(e2ee_group_id))
);

CREATE TABLE domain.direct_messages (
  tenant_id text NOT NULL,
  dm_id text NOT NULL,
  e2ee_group_id text NOT NULL,
  participants_sealed boolean NOT NULL DEFAULT FALSE,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, dm_id),
  CONSTRAINT direct_messages_organization_fk
    FOREIGN KEY (tenant_id) REFERENCES domain.organizations (tenant_id),
  CONSTRAINT direct_messages_e2ee_group_unique
    UNIQUE (tenant_id, e2ee_group_id),
  CONSTRAINT direct_messages_tenant_id_not_blank
    CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
  CONSTRAINT direct_messages_dm_id_not_blank
    CHECK (dm_id <> '' AND dm_id = btrim(dm_id)),
  CONSTRAINT direct_messages_e2ee_group_id_not_blank
    CHECK (e2ee_group_id <> '' AND e2ee_group_id = btrim(e2ee_group_id))
);

CREATE TABLE domain.direct_message_participants (
  tenant_id text NOT NULL,
  dm_id text NOT NULL,
  actor_type smallint NOT NULL,
  actor_id text NOT NULL,
  PRIMARY KEY (tenant_id, dm_id, actor_type, actor_id),
  CONSTRAINT direct_message_participants_dm_fk
    FOREIGN KEY (tenant_id, dm_id)
      REFERENCES domain.direct_messages (tenant_id, dm_id),
  CONSTRAINT direct_message_participants_member_fk
    FOREIGN KEY (tenant_id, actor_type, actor_id)
      REFERENCES domain.members (tenant_id, actor_type, actor_id)
);

CREATE FUNCTION domain.reject_initially_sealed_direct_message()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.participants_sealed THEN
    RAISE EXCEPTION 'Direct Message participants must be finalized after participant creation'
      USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER direct_messages_initially_unsealed
BEFORE INSERT ON domain.direct_messages
FOR EACH ROW
EXECUTE FUNCTION domain.reject_initially_sealed_direct_message();

CREATE FUNCTION domain.reject_direct_message_participant_reopen()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF OLD.participants_sealed AND NOT NEW.participants_sealed THEN
    RAISE EXCEPTION 'Direct Message participants cannot be reopened'
      USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER direct_messages_participants_cannot_reopen
BEFORE UPDATE OF participants_sealed ON domain.direct_messages
FOR EACH ROW
EXECUTE FUNCTION domain.reject_direct_message_participant_reopen();

CREATE FUNCTION domain.require_direct_message_participants_sealed()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  participants_are_sealed boolean;
BEGIN
  SELECT participants_sealed
  INTO participants_are_sealed
  FROM domain.direct_messages
  WHERE tenant_id = NEW.tenant_id
    AND dm_id = NEW.dm_id;

  IF FOUND AND NOT participants_are_sealed THEN
    RAISE EXCEPTION 'Direct Message participants must be finalized before commit'
      USING ERRCODE = 'check_violation';
  END IF;
  RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER direct_messages_participants_sealed_at_commit
AFTER INSERT ON domain.direct_messages
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION domain.require_direct_message_participants_sealed();

CREATE FUNCTION domain.enforce_direct_message_participants_append_only()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP <> 'INSERT' THEN
    RAISE EXCEPTION 'Direct Message participants are append-only during creation'
      USING ERRCODE = 'check_violation';
  END IF;

  PERFORM 1
  FROM domain.direct_messages
  WHERE tenant_id = NEW.tenant_id
    AND dm_id = NEW.dm_id
    AND NOT participants_sealed
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'Direct Message participants are already finalized'
      USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER direct_message_participants_append_only_during_creation
BEFORE INSERT OR UPDATE OR DELETE ON domain.direct_message_participants
FOR EACH ROW
EXECUTE FUNCTION domain.enforce_direct_message_participants_append_only();

COMMIT;

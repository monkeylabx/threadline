CREATE TABLE domain.conversation_sequence_heads (
  tenant_id text NOT NULL,
  conversation_kind smallint NOT NULL,
  conversation_id text NOT NULL,
  last_seq bigint NOT NULL DEFAULT 0,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, conversation_kind, conversation_id),
  CONSTRAINT conversation_sequence_heads_organization_fk
    FOREIGN KEY (tenant_id)
      REFERENCES domain.organizations (tenant_id),
  CONSTRAINT conversation_sequence_heads_tenant_id_not_blank
    CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
  CONSTRAINT conversation_sequence_heads_kind_known
    CHECK (conversation_kind IN (1, 2)),
  CONSTRAINT conversation_sequence_heads_conversation_id_not_blank
    CHECK (conversation_id <> '' AND conversation_id = btrim(conversation_id)),
  CONSTRAINT conversation_sequence_heads_last_seq_non_negative
    CHECK (last_seq >= 0)
);

CREATE FUNCTION domain.enforce_conversation_sequence_head_lifecycle()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'Conversation sequence heads cannot be deleted'
      USING ERRCODE = 'check_violation';
  END IF;
  IF NEW.tenant_id <> OLD.tenant_id
      OR NEW.conversation_kind <> OLD.conversation_kind
      OR NEW.conversation_id <> OLD.conversation_id THEN
    RAISE EXCEPTION 'Conversation sequence head identity is immutable'
      USING ERRCODE = 'check_violation';
  END IF;
  IF NEW.last_seq <> OLD.last_seq + 1 THEN
    RAISE EXCEPTION 'Conversation sequence heads advance by exactly one'
      USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER conversation_sequence_heads_lifecycle_guard
BEFORE UPDATE OR DELETE ON domain.conversation_sequence_heads
FOR EACH ROW
EXECUTE FUNCTION domain.enforce_conversation_sequence_head_lifecycle();

CREATE TABLE domain.channel_events (
  tenant_id text NOT NULL,
  event_id text NOT NULL,
  channel_id text,
  dm_id text,
  channel_seq bigint NOT NULL,
  sender_actor_type smallint NOT NULL,
  sender_actor_id text NOT NULL,
  sender_device_id text NOT NULL,
  idempotency_key text NOT NULL,
  category smallint NOT NULL,
  envelope bytea NOT NULL,
  content_hash bytea NOT NULL,
  committed_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, event_id),
  CONSTRAINT channel_events_organization_fk
    FOREIGN KEY (tenant_id)
      REFERENCES domain.organizations (tenant_id),
  CONSTRAINT channel_events_channel_fk
    FOREIGN KEY (tenant_id, channel_id)
      REFERENCES domain.channels (tenant_id, channel_id),
  CONSTRAINT channel_events_direct_message_fk
    FOREIGN KEY (tenant_id, dm_id)
      REFERENCES domain.direct_messages (tenant_id, dm_id),
  CONSTRAINT channel_events_sender_member_fk
    FOREIGN KEY (tenant_id, sender_actor_type, sender_actor_id)
      REFERENCES domain.members (tenant_id, actor_type, actor_id),
  CONSTRAINT channel_events_exactly_one_conversation
    CHECK ((channel_id IS NULL) <> (dm_id IS NULL)),
  CONSTRAINT channel_events_channel_seq_unique
    UNIQUE (tenant_id, channel_id, channel_seq),
  CONSTRAINT channel_events_dm_seq_unique
    UNIQUE (tenant_id, dm_id, channel_seq),
  CONSTRAINT channel_events_command_binding_unique
    UNIQUE (tenant_id, event_id, sender_device_id, idempotency_key),
  CONSTRAINT channel_events_tenant_id_not_blank
    CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
  CONSTRAINT channel_events_event_id_not_blank
    CHECK (event_id <> '' AND event_id = btrim(event_id)),
  CONSTRAINT channel_events_channel_id_not_blank
    CHECK (channel_id IS NULL OR (channel_id <> '' AND channel_id = btrim(channel_id))),
  CONSTRAINT channel_events_dm_id_not_blank
    CHECK (dm_id IS NULL OR (dm_id <> '' AND dm_id = btrim(dm_id))),
  CONSTRAINT channel_events_channel_seq_positive
    CHECK (channel_seq > 0),
  CONSTRAINT channel_events_sender_actor_id_not_blank
    CHECK (sender_actor_id <> '' AND sender_actor_id = btrim(sender_actor_id)),
  CONSTRAINT channel_events_sender_device_id_not_blank
    CHECK (sender_device_id <> '' AND sender_device_id = btrim(sender_device_id)),
  CONSTRAINT channel_events_idempotency_key_not_blank
    CHECK (idempotency_key <> '' AND idempotency_key = btrim(idempotency_key)),
  CONSTRAINT channel_events_category_known
    CHECK (category IN (1, 2, 3, 4, 5)),
  CONSTRAINT channel_events_envelope_size
    CHECK (octet_length(envelope) BETWEEN 1 AND 262144),
  CONSTRAINT channel_events_content_hash_size
    CHECK (octet_length(content_hash) = 32)
);

CREATE FUNCTION domain.enforce_channel_event_immutability()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'Channel events are immutable and cannot be deleted'
    USING ERRCODE = 'check_violation';
END;
$$;

CREATE TRIGGER channel_events_immutable_guard
BEFORE UPDATE OR DELETE ON domain.channel_events
FOR EACH ROW
EXECUTE FUNCTION domain.enforce_channel_event_immutability();

CREATE TABLE domain.send_commands (
  tenant_id text NOT NULL,
  sender_device_id text NOT NULL,
  idempotency_key text NOT NULL,
  event_id text NOT NULL,
  envelope_digest bytea NOT NULL,
  bound_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, sender_device_id, idempotency_key),
  CONSTRAINT send_commands_event_unique
    UNIQUE (tenant_id, event_id),
  CONSTRAINT send_commands_exact_event_fk
    FOREIGN KEY (tenant_id, event_id, sender_device_id, idempotency_key)
      REFERENCES domain.channel_events (tenant_id, event_id, sender_device_id, idempotency_key),
  CONSTRAINT send_commands_tenant_id_not_blank
    CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
  CONSTRAINT send_commands_sender_device_id_not_blank
    CHECK (sender_device_id <> '' AND sender_device_id = btrim(sender_device_id)),
  CONSTRAINT send_commands_idempotency_key_not_blank
    CHECK (idempotency_key <> '' AND idempotency_key = btrim(idempotency_key)),
  CONSTRAINT send_commands_event_id_not_blank
    CHECK (event_id <> '' AND event_id = btrim(event_id)),
  CONSTRAINT send_commands_envelope_digest_size
    CHECK (octet_length(envelope_digest) = 32)
);

CREATE FUNCTION domain.enforce_send_command_immutability()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'Send command bindings are immutable and cannot be deleted'
    USING ERRCODE = 'check_violation';
END;
$$;

CREATE TRIGGER send_commands_immutable_guard
BEFORE UPDATE OR DELETE ON domain.send_commands
FOR EACH ROW
EXECUTE FUNCTION domain.enforce_send_command_immutability();

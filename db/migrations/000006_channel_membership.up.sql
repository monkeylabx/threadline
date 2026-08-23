BEGIN;

CREATE TABLE domain.channel_memberships (
  interval_id bigint GENERATED ALWAYS AS IDENTITY,
  tenant_id text NOT NULL,
  channel_id text NOT NULL,
  actor_type smallint NOT NULL,
  actor_id text NOT NULL,
  role smallint NOT NULL,
  joined_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  left_at timestamp with time zone,
  PRIMARY KEY (tenant_id, interval_id),
  CONSTRAINT channel_memberships_channel_fk
    FOREIGN KEY (tenant_id, channel_id)
      REFERENCES domain.channels (tenant_id, channel_id),
  CONSTRAINT channel_memberships_member_fk
    FOREIGN KEY (tenant_id, actor_type, actor_id)
      REFERENCES domain.members (tenant_id, actor_type, actor_id),
  CONSTRAINT channel_memberships_role_known
    CHECK (role IN (1, 2, 3, 4)),
  CONSTRAINT channel_memberships_left_not_before_joined
    CHECK (left_at IS NULL OR left_at >= joined_at)
);

CREATE UNIQUE INDEX channel_memberships_one_active_actor
ON domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id)
WHERE left_at IS NULL;

CREATE FUNCTION domain.require_active_member_for_channel_membership()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.left_at IS NOT NULL THEN
    RAISE EXCEPTION 'Channel Membership must be created active'
      USING ERRCODE = 'check_violation';
  END IF;

  PERFORM 1
  FROM domain.members
  WHERE tenant_id = NEW.tenant_id
    AND actor_type = NEW.actor_type
    AND actor_id = NEW.actor_id
    AND state = 2
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'Channel Membership requires an active tenant Member'
      USING ERRCODE = 'check_violation';
  END IF;

  NEW.joined_at := CURRENT_TIMESTAMP;
  RETURN NEW;
END;
$$;

CREATE TRIGGER channel_memberships_require_active_member
BEFORE INSERT ON domain.channel_memberships
FOR EACH ROW
EXECUTE FUNCTION domain.require_active_member_for_channel_membership();

CREATE FUNCTION domain.enforce_channel_membership_interval_lifecycle()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'Channel Membership intervals cannot be deleted'
      USING ERRCODE = 'check_violation';
  END IF;

  IF OLD.interval_id IS DISTINCT FROM NEW.interval_id
      OR OLD.tenant_id IS DISTINCT FROM NEW.tenant_id
      OR OLD.channel_id IS DISTINCT FROM NEW.channel_id
      OR OLD.actor_type IS DISTINCT FROM NEW.actor_type
      OR OLD.actor_id IS DISTINCT FROM NEW.actor_id
      OR OLD.role IS DISTINCT FROM NEW.role
      OR OLD.joined_at IS DISTINCT FROM NEW.joined_at THEN
    RAISE EXCEPTION 'Channel Membership interval identity and facts are immutable'
      USING ERRCODE = 'check_violation';
  END IF;

  IF OLD.left_at IS NOT NULL OR NEW.left_at IS NULL THEN
    RAISE EXCEPTION 'Channel Membership can only transition from active to departed once'
      USING ERRCODE = 'check_violation';
  END IF;

  NEW.left_at := CURRENT_TIMESTAMP;
  RETURN NEW;
END;
$$;

CREATE TRIGGER channel_memberships_interval_lifecycle_guard
BEFORE UPDATE OR DELETE ON domain.channel_memberships
FOR EACH ROW
EXECUTE FUNCTION domain.enforce_channel_membership_interval_lifecycle();

COMMIT;

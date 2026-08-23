BEGIN;

DROP TABLE domain.channel_memberships;
DROP FUNCTION domain.enforce_channel_membership_interval_lifecycle();
DROP FUNCTION domain.require_active_member_for_channel_membership();

COMMIT;

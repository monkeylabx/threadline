BEGIN;

DROP TABLE domain.direct_message_participants;
DROP TABLE domain.direct_messages;
DROP TABLE domain.channels;
DROP FUNCTION domain.enforce_direct_message_participants_append_only();
DROP FUNCTION domain.require_direct_message_participants_sealed();
DROP FUNCTION domain.enforce_direct_message_lifecycle_update();
DROP FUNCTION domain.reject_initially_sealed_direct_message();

COMMIT;

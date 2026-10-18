BEGIN;

DROP TABLE domain.send_commands;
DROP TABLE domain.channel_events;
DROP TABLE domain.conversation_sequence_heads;
DROP FUNCTION domain.enforce_send_command_immutability();
DROP FUNCTION domain.enforce_channel_event_immutability();
DROP FUNCTION domain.enforce_conversation_sequence_head_lifecycle();

COMMIT;

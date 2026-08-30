BEGIN;

ALTER TABLE domain.transactional_outbox
  DROP CONSTRAINT transactional_outbox_current_attempt_fk;

DROP TABLE domain.outbox_delivery_attempts;
DROP TABLE domain.transactional_outbox;
DROP TABLE domain.domain_events;
DROP FUNCTION domain.enforce_outbox_current_attempt_consistency();
DROP FUNCTION domain.enforce_outbox_delivery_attempt_lifecycle();
DROP FUNCTION domain.enforce_transactional_outbox_lifecycle();
DROP FUNCTION domain.enforce_domain_event_immutability();

COMMIT;

BEGIN;

DROP FUNCTION domain.record_transactional_outbox_publish_failure(
  text, text, bigint, bigint, bigint, text, bytea, text
);
DROP FUNCTION domain.acknowledge_transactional_outbox_published(
  text, text, bigint, bigint, bigint, text, bytea, text, numeric, boolean, text
);
DROP FUNCTION domain.renew_transactional_outbox_claim(
  text, text, bigint, bigint, bigint, text, bytea
);
DROP FUNCTION domain.claim_transactional_outbox_batch(text, integer);
DROP FUNCTION domain.transactional_outbox_backoff_delay_ms(
  bytea, smallint, bigint, integer, integer
);
DROP FUNCTION domain.transactional_outbox_message_id(text, text, text, bigint);
DROP FUNCTION domain.transactional_outbox_claim_digest(bytea);
DROP TYPE domain.transactional_outbox_failure_result;
DROP TYPE domain.transactional_outbox_renew_result;
DROP TYPE domain.transactional_outbox_claim_result;

COMMIT;

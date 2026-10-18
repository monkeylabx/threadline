CREATE TABLE outbox_events_migration_0002 (
    tenant_id TEXT NOT NULL,
    conversation_kind INTEGER NOT NULL,
    conversation_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    envelope_bytes BLOB NOT NULL,
    enqueue_sequence INTEGER NOT NULL
) STRICT;

INSERT INTO outbox_events_migration_0002
SELECT tenant_id, conversation_kind, conversation_id, event_id, idempotency_key, envelope_bytes,
       row_number() OVER (ORDER BY tenant_id, conversation_kind, conversation_id, event_id)
FROM outbox_events;

DROP TABLE outbox_events;

CREATE TABLE outbox_events (
    tenant_id TEXT NOT NULL CHECK (length(tenant_id) > 0),
    conversation_kind INTEGER NOT NULL CHECK (conversation_kind IN (1, 2)),
    conversation_id TEXT NOT NULL CHECK (length(conversation_id) > 0),
    event_id TEXT NOT NULL CHECK (length(event_id) > 0),
    idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) > 0),
    envelope_bytes BLOB NOT NULL CHECK (length(envelope_bytes) > 0),
    enqueue_sequence INTEGER NOT NULL CHECK (enqueue_sequence > 0),
    PRIMARY KEY (tenant_id, conversation_kind, conversation_id, event_id),
    UNIQUE (tenant_id, conversation_kind, conversation_id, idempotency_key),
    UNIQUE (enqueue_sequence)
) STRICT, WITHOUT ROWID;

INSERT INTO outbox_events
SELECT tenant_id, conversation_kind, conversation_id, event_id, idempotency_key, envelope_bytes, enqueue_sequence
FROM outbox_events_migration_0002;

DROP TABLE outbox_events_migration_0002;

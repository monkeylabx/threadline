CREATE TABLE threadline_schema_migrations (
    version INTEGER PRIMARY KEY CHECK (version > 0),
    sha256 BLOB NOT NULL CHECK (length(sha256) = 32)
) STRICT;

CREATE TABLE opaque_events (
    tenant_id TEXT NOT NULL CHECK (length(tenant_id) > 0),
    conversation_kind INTEGER NOT NULL CHECK (conversation_kind IN (1, 2)),
    conversation_id TEXT NOT NULL CHECK (length(conversation_id) > 0),
    event_id TEXT NOT NULL CHECK (length(event_id) > 0),
    committed_sequence BLOB NOT NULL CHECK (length(committed_sequence) = 8),
    envelope_bytes BLOB NOT NULL CHECK (length(envelope_bytes) > 0),
    PRIMARY KEY (tenant_id, conversation_kind, conversation_id, event_id),
    UNIQUE (tenant_id, conversation_kind, conversation_id, committed_sequence)
) STRICT, WITHOUT ROWID;

CREATE TABLE outbox_events (
    tenant_id TEXT NOT NULL CHECK (length(tenant_id) > 0),
    conversation_kind INTEGER NOT NULL CHECK (conversation_kind IN (1, 2)),
    conversation_id TEXT NOT NULL CHECK (length(conversation_id) > 0),
    event_id TEXT NOT NULL CHECK (length(event_id) > 0),
    idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) > 0),
    envelope_bytes BLOB NOT NULL CHECK (length(envelope_bytes) > 0),
    PRIMARY KEY (tenant_id, conversation_kind, conversation_id, event_id),
    UNIQUE (tenant_id, conversation_kind, conversation_id, idempotency_key)
) STRICT, WITHOUT ROWID;

CREATE TABLE sync_cursors (
    tenant_id TEXT NOT NULL CHECK (length(tenant_id) > 0),
    device_id TEXT NOT NULL CHECK (length(device_id) > 0),
    conversation_kind INTEGER NOT NULL CHECK (conversation_kind IN (1, 2)),
    conversation_id TEXT NOT NULL CHECK (length(conversation_id) > 0),
    last_applied_sequence BLOB NOT NULL CHECK (length(last_applied_sequence) = 8),
    cursor_bytes BLOB NOT NULL CHECK (length(cursor_bytes) > 0),
    PRIMARY KEY (tenant_id, device_id, conversation_kind, conversation_id)
) STRICT, WITHOUT ROWID;

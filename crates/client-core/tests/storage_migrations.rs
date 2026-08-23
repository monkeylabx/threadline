use std::sync::{
    atomic::{AtomicBool, Ordering},
    Arc,
};

use rusqlite::{
    hooks::{AuthAction, AuthContext, Authorization},
    Connection,
};
use threadline_client_core::storage::{
    migrate, CommittedEvent, ConversationRef, CursorUpdate, PendingOutboxEvent, Storage,
    StorageError, APPLICATION_ID, LATEST_SCHEMA_VERSION,
};

#[test]
fn empty_database_migrates_from_zero_to_one() {
    let mut connection = Connection::open_in_memory().expect("open synthetic database");

    let outcome = migrate(&mut connection).expect("apply migration 0001");

    assert_eq!(outcome.from_version, 0);
    assert_eq!(outcome.to_version, 1);
    assert_eq!(outcome.applied_migrations, 1);
    assert_eq!(LATEST_SCHEMA_VERSION, 1);
    assert_eq!(pragma_i64(&connection, "application_id"), APPLICATION_ID);
    assert_eq!(pragma_i64(&connection, "user_version"), 1);

    let ledger: (i64, i64) = connection
        .query_row(
            "SELECT version, length(sha256) FROM threadline_schema_migrations",
            [],
            |row| Ok((row.get(0)?, row.get(1)?)),
        )
        .expect("read migration ledger");
    assert_eq!(ledger, (1, 32));
}

#[test]
fn repeated_open_is_idempotent() {
    let mut connection = Connection::open_in_memory().expect("open synthetic database");
    migrate(&mut connection).expect("apply migration 0001");

    let outcome = migrate(&mut connection).expect("verify existing schema");

    assert_eq!(outcome.from_version, 1);
    assert_eq!(outcome.to_version, 1);
    assert_eq!(outcome.applied_migrations, 0);
    assert_eq!(
        connection
            .query_row(
                "SELECT count(*) FROM threadline_schema_migrations",
                [],
                |row| row.get::<_, i64>(0),
            )
            .expect("count migration ledger"),
        1
    );
}

#[test]
fn migration_ledger_rejects_unexpected_rows() {
    let mut connection = Connection::open_in_memory().expect("open synthetic database");
    migrate(&mut connection).expect("apply migration 0001");
    connection
        .execute(
            "INSERT INTO threadline_schema_migrations (version, sha256) VALUES (2, zeroblob(32))",
            [],
        )
        .expect("inject unexpected ledger row");

    assert_eq!(
        migrate(&mut connection),
        Err(StorageError::MigrationLedgerInvalid)
    );
}

#[test]
fn checksum_drift_fails_closed_without_rewriting_the_ledger() {
    let mut connection = Connection::open_in_memory().expect("open synthetic database");
    migrate(&mut connection).expect("apply migration 0001");
    let drifted = [0xA5_u8; 32];
    connection
        .execute(
            "UPDATE threadline_schema_migrations SET sha256 = ?1 WHERE version = 1",
            [drifted.as_slice()],
        )
        .expect("inject synthetic checksum drift");

    let error = migrate(&mut connection).expect_err("reject checksum drift");

    assert_eq!(error, StorageError::MigrationLedgerInvalid);
    let stored: Vec<u8> = connection
        .query_row(
            "SELECT sha256 FROM threadline_schema_migrations WHERE version = 1",
            [],
            |row| row.get(0),
        )
        .expect("read drifted checksum");
    assert_eq!(stored, drifted);
    assert_eq!(pragma_i64(&connection, "user_version"), 1);
}

#[test]
fn newer_schema_fails_closed_without_changing_version() {
    let mut connection = Connection::open_in_memory().expect("open synthetic database");
    connection
        .pragma_update(None, "application_id", APPLICATION_ID)
        .expect("set Threadline application id");
    connection
        .pragma_update(None, "user_version", 2)
        .expect("set future version");

    let error = migrate(&mut connection).expect_err("reject future schema");

    assert_eq!(error, StorageError::NewerSchema);
    assert_eq!(pragma_i64(&connection, "user_version"), 2);
    assert_eq!(user_object_count(&connection), 0);
}

#[test]
fn migration_failure_leaves_the_database_at_version_zero() {
    let mut connection = Connection::open_in_memory().expect("open synthetic database");
    let first_table_was_created = Arc::new(AtomicBool::new(false));
    let observed_first_table = Arc::clone(&first_table_was_created);
    connection
        .authorizer(Some(move |context: AuthContext<'_>| match context.action {
            AuthAction::CreateTable {
                table_name: "threadline_schema_migrations",
            } => {
                observed_first_table.store(true, Ordering::SeqCst);
                Authorization::Allow
            }
            AuthAction::CreateTable {
                table_name: "opaque_events",
            } => Authorization::Deny,
            _ => Authorization::Allow,
        }))
        .expect("inject a deterministic mid-migration failure");

    let error = migrate(&mut connection).expect_err("migration must fail");
    connection
        .authorizer(None::<fn(AuthContext<'_>) -> Authorization>)
        .expect("remove failure injection");

    assert_eq!(error, StorageError::Database);
    assert!(first_table_was_created.load(Ordering::SeqCst));
    assert_eq!(pragma_i64(&connection, "application_id"), 0);
    assert_eq!(pragma_i64(&connection, "user_version"), 0);
    assert_eq!(user_object_count(&connection), 0);
}

#[test]
fn schema_contains_only_scoped_opaque_state() {
    let mut connection = Connection::open_in_memory().expect("open synthetic database");
    migrate(&mut connection).expect("apply migration 0001");

    let mut statement = connection
        .prepare(
            "SELECT name, sql FROM sqlite_schema
             WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
             ORDER BY name",
        )
        .expect("inspect schema");
    let tables = statement
        .query_map([], |row| {
            Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?))
        })
        .expect("query tables")
        .collect::<Result<Vec<_>, _>>()
        .expect("read tables");

    assert_eq!(
        tables
            .iter()
            .map(|(name, _)| name.as_str())
            .collect::<Vec<_>>(),
        [
            "opaque_events",
            "outbox_events",
            "sync_cursors",
            "threadline_schema_migrations",
        ]
    );
    let schema = tables
        .iter()
        .map(|(_, sql)| sql.as_str())
        .collect::<Vec<_>>()
        .join("\n")
        .to_ascii_lowercase();
    for forbidden in [
        "plaintext",
        "virtual table",
        "fts",
        "private_key",
        "mls",
        "epoch",
        "history",
        "recovery",
    ] {
        assert!(
            !schema.contains(forbidden),
            "forbidden schema term: {forbidden}"
        );
    }
    for table in ["opaque_events", "outbox_events", "sync_cursors"] {
        let columns = table_columns(&connection, table);
        for required in ["tenant_id", "conversation_kind", "conversation_id"] {
            assert!(columns.iter().any(|column| column == required));
        }
    }
}

#[test]
fn committed_event_preserves_the_golden_envelope_byte_for_byte() {
    let mut connection = Connection::open_in_memory().expect("open synthetic database");
    let conversation = ConversationRef::channel("channel-golden").expect("valid conversation");
    let golden = decode_hex(include_str!(
        "../../../proto/golden/v1/channel-event-envelope.golden.hex"
    ));
    let mut storage = Storage::attach(&mut connection).expect("attach migrated storage");

    storage
        .insert_committed_event(CommittedEvent {
            tenant_id: "tenant-golden",
            conversation,
            event_id: "evt-golden-0001",
            committed_sequence: 314,
            envelope_bytes: &golden,
        })
        .expect("store opaque event");

    let loaded = storage
        .load_committed_event("tenant-golden", conversation, "evt-golden-0001")
        .expect("load opaque event")
        .expect("event exists");
    assert_eq!(loaded, golden);
}

#[test]
fn committed_sequence_is_lossless_sortable_and_unique_per_conversation() {
    let mut connection = Connection::open_in_memory().expect("open synthetic database");
    let channel = ConversationRef::channel("channel-a").expect("valid channel");
    let dm = ConversationRef::direct_message("dm-a").expect("valid dm");
    let mut storage = Storage::attach(&mut connection).expect("attach migrated storage");

    for (event_id, sequence) in [
        ("evt-max", u64::MAX),
        ("evt-zero", 0),
        ("evt-i64-max", i64::MAX as u64),
    ] {
        storage
            .insert_committed_event(synthetic_event("tenant-a", channel, event_id, sequence))
            .expect("store boundary event");
    }

    assert_eq!(
        storage
            .list_committed_sequences("tenant-a", channel)
            .expect("list ordered sequences"),
        vec![0, i64::MAX as u64, u64::MAX]
    );
    assert_eq!(
        storage.insert_committed_event(synthetic_event(
            "tenant-a",
            channel,
            "evt-duplicate-sequence",
            u64::MAX,
        )),
        Err(StorageError::Conflict)
    );
    storage
        .insert_committed_event(synthetic_event(
            "tenant-a",
            dm,
            "evt-same-sequence-other-scope",
            u64::MAX,
        ))
        .expect("sequence uniqueness is conversation-scoped");
}

#[test]
fn outbox_preserves_bytes_and_scopes_idempotency() {
    let mut connection = Connection::open_in_memory().expect("open synthetic database");
    let channel = ConversationRef::channel("channel-a").expect("valid channel");
    let other_channel = ConversationRef::channel("channel-b").expect("valid channel");
    let pending_bytes = [
        0x0A, 0x05, b'e', b'v', b't', b'-', b'1', 0x82, 0xB5, 0x18, 0x01, 0xA5,
    ];
    let mut storage = Storage::attach(&mut connection).expect("attach migrated storage");

    storage
        .enqueue_outbox(PendingOutboxEvent {
            tenant_id: "tenant-a",
            conversation: channel,
            event_id: "evt-1",
            idempotency_key: "idem-1",
            envelope_bytes: &pending_bytes,
        })
        .expect("enqueue pending event");

    assert_eq!(
        storage
            .load_outbox_event("tenant-a", channel, "evt-1")
            .expect("load pending event"),
        Some(pending_bytes.to_vec())
    );
    assert_eq!(
        storage.enqueue_outbox(PendingOutboxEvent {
            tenant_id: "tenant-a",
            conversation: channel,
            event_id: "evt-1",
            idempotency_key: "idem-2",
            envelope_bytes: &pending_bytes,
        }),
        Err(StorageError::Conflict)
    );
    assert_eq!(
        storage.enqueue_outbox(PendingOutboxEvent {
            tenant_id: "tenant-a",
            conversation: channel,
            event_id: "evt-2",
            idempotency_key: "idem-1",
            envelope_bytes: &pending_bytes,
        }),
        Err(StorageError::Conflict)
    );
    storage
        .enqueue_outbox(PendingOutboxEvent {
            tenant_id: "tenant-a",
            conversation: other_channel,
            event_id: "evt-2",
            idempotency_key: "idem-1",
            envelope_bytes: &pending_bytes,
        })
        .expect("idempotency is conversation-scoped");
    storage
        .enqueue_outbox(PendingOutboxEvent {
            tenant_id: "tenant-b",
            conversation: channel,
            event_id: "evt-2",
            idempotency_key: "idem-1",
            envelope_bytes: &pending_bytes,
        })
        .expect("idempotency is tenant-scoped");
}

#[test]
fn cursor_compare_and_swap_preserves_contiguous_progress_and_bytes() {
    let mut connection = Connection::open_in_memory().expect("open synthetic database");
    let channel = ConversationRef::channel("channel-a").expect("valid channel");
    let mut storage = Storage::attach(&mut connection).expect("attach migrated storage");

    storage
        .record_cursor(CursorUpdate {
            tenant_id: "tenant-a",
            device_id: "device-a",
            conversation: channel,
            expected_previous_sequence: None,
            last_applied_sequence: 0,
            cursor_bytes: &[
                0x0A, 0x09, b'c', b'h', b'a', b'n', b'n', b'e', b'l', b'-', b'a',
            ],
        })
        .expect("record initial cursor");
    storage
        .record_cursor(CursorUpdate {
            tenant_id: "tenant-a",
            device_id: "device-a",
            conversation: channel,
            expected_previous_sequence: Some(0),
            last_applied_sequence: i64::MAX as u64,
            cursor_bytes: &[0x18, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F],
        })
        .expect("advance over a verified contiguous batch");
    let max_cursor = [0x18, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x01];
    storage
        .record_cursor(CursorUpdate {
            tenant_id: "tenant-a",
            device_id: "device-a",
            conversation: channel,
            expected_previous_sequence: Some(i64::MAX as u64),
            last_applied_sequence: u64::MAX,
            cursor_bytes: &max_cursor,
        })
        .expect("record u64 max cursor");

    let stored = storage
        .load_cursor("tenant-a", "device-a", channel)
        .expect("load cursor")
        .expect("cursor exists");
    assert_eq!(stored.last_applied_sequence, u64::MAX);
    assert_eq!(stored.cursor_bytes, max_cursor);
    assert_eq!(
        storage.record_cursor(CursorUpdate {
            tenant_id: "tenant-a",
            device_id: "device-a",
            conversation: channel,
            expected_previous_sequence: Some(u64::MAX),
            last_applied_sequence: u64::MAX,
            cursor_bytes: &[0x18, 0x00],
        }),
        Err(StorageError::Conflict)
    );
    assert_eq!(
        storage.record_cursor(CursorUpdate {
            tenant_id: "tenant-a",
            device_id: "device-a",
            conversation: channel,
            expected_previous_sequence: Some(0),
            last_applied_sequence: 1,
            cursor_bytes: &[0x18, 0x01],
        }),
        Err(StorageError::Conflict)
    );
}

#[test]
fn public_errors_are_stable_and_do_not_echo_sensitive_inputs() {
    let mut connection = Connection::open_in_memory().expect("open synthetic database");
    let conversation = ConversationRef::channel("channel-secret").expect("valid channel");
    let mut storage = Storage::attach(&mut connection).expect("attach migrated storage");

    let error = storage
        .enqueue_outbox(PendingOutboxEvent {
            tenant_id: "tenant-secret",
            conversation,
            event_id: "event-secret",
            idempotency_key: "idempotency-secret",
            envelope_bytes: &[],
        })
        .expect_err("reject empty envelope");

    assert_eq!(error.code(), "storage_invalid_input");
    assert_eq!(error.to_string(), "storage_invalid_input");
}

fn pragma_i64(connection: &Connection, name: &str) -> i64 {
    connection
        .query_row(&format!("PRAGMA {name}"), [], |row| row.get(0))
        .expect("read pragma")
}

fn user_object_count(connection: &Connection) -> i64 {
    connection
        .query_row(
            "SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'",
            [],
            |row| row.get(0),
        )
        .expect("count user objects")
}

fn table_columns(connection: &Connection, table: &str) -> Vec<String> {
    let query = match table {
        "opaque_events" => "PRAGMA table_info(opaque_events)",
        "outbox_events" => "PRAGMA table_info(outbox_events)",
        "sync_cursors" => "PRAGMA table_info(sync_cursors)",
        _ => panic!("unexpected table"),
    };
    let mut statement = connection.prepare(query).expect("inspect table columns");
    statement
        .query_map([], |row| row.get(1))
        .expect("query table columns")
        .collect::<Result<Vec<_>, _>>()
        .expect("read table columns")
}

fn decode_hex(input: &str) -> Vec<u8> {
    let hex = input.trim();
    assert_eq!(hex.len() % 2, 0, "fixture hex has whole bytes");
    (0..hex.len())
        .step_by(2)
        .map(|index| u8::from_str_radix(&hex[index..index + 2], 16).expect("valid fixture hex"))
        .collect()
}

fn synthetic_event<'value>(
    tenant_id: &'value str,
    conversation: ConversationRef<'value>,
    event_id: &'value str,
    committed_sequence: u64,
) -> CommittedEvent<'value> {
    CommittedEvent {
        tenant_id,
        conversation,
        event_id,
        committed_sequence,
        envelope_bytes: &[0x8A, 0x01, 0x02, 0xA5],
    }
}

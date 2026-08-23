use std::{fmt::Write as _, fs, path::PathBuf};

use rusqlite::Connection;
use threadline_client_core::storage::{
    CommittedEvent, ConversationRef, CursorUpdate, DatabaseKey, EncryptedDatabase,
    PendingOutboxEvent, StorageError, StoredCursor,
};
use zeroize::Zeroizing;

const EXPECTED_APPLICATION_ID: i64 = 1_414_285_646;
const EXPECTED_SCHEMA_VERSION: i64 = 1;
const MIGRATION_0001_SHA256: [u8; 32] = [
    0x61, 0x16, 0xaa, 0xca, 0xec, 0x95, 0x84, 0x15, 0xb7, 0x73, 0xbd, 0xd1, 0xe2, 0xf9, 0xe3, 0x4d,
    0x7d, 0x9f, 0x55, 0xf3, 0x0d, 0xef, 0x8a, 0x5b, 0x8d, 0x9a, 0xce, 0x11, 0xbf, 0x63, 0x37, 0x43,
];

#[test]
fn create_and_reopen_preserve_application_version_and_ledger_invariants() {
    let fixture = DatabaseFixture::create();
    let database = fixture.open();
    drop(database);

    let connection = fixture.open_for_fixture_inspection();
    assert_eq!(
        pragma_i64(&connection, "application_id"),
        EXPECTED_APPLICATION_ID
    );
    assert_eq!(
        pragma_i64(&connection, "user_version"),
        EXPECTED_SCHEMA_VERSION
    );
    assert_eq!(
        connection
            .query_row(
                "SELECT version, sha256 FROM threadline_schema_migrations",
                [],
                |row| Ok((row.get::<_, i64>(0)?, row.get::<_, Vec<u8>>(1)?)),
            )
            .expect("read migration ledger"),
        (EXPECTED_SCHEMA_VERSION, MIGRATION_0001_SHA256.to_vec())
    );
    drop(connection);

    fixture.open();
}

#[test]
fn foreign_database_fails_closed_without_rewriting_it() {
    let fixture = DatabaseFixture::create();
    let connection = fixture.open_empty_keyed_fixture();
    connection
        .execute_batch("CREATE TABLE foreign_data (value TEXT NOT NULL);")
        .expect("create foreign encrypted schema");
    drop(connection);
    let before = fixture.database_family_state();

    let error = rejected_open(&fixture);

    assert_eq!(error, StorageError::ForeignDatabase);
    assert_eq!(fixture.database_family_state(), before);
}

#[test]
fn newer_schema_fails_closed_without_changing_version() {
    let fixture = DatabaseFixture::create();
    drop(fixture.open());
    let connection = fixture.open_for_fixture_inspection();
    connection
        .pragma_update(None, "user_version", EXPECTED_SCHEMA_VERSION + 1)
        .expect("inject future schema version");
    drop(connection);
    let before = fixture.database_family_state();

    let error = rejected_open(&fixture);

    assert_eq!(error, StorageError::NewerSchema);
    assert_eq!(fixture.database_family_state(), before);
}

#[test]
fn checksum_drift_fails_closed_without_repairing_the_ledger() {
    let fixture = DatabaseFixture::create();
    drop(fixture.open());
    let drifted = [0xA5_u8; 32];
    let connection = fixture.open_for_fixture_inspection();
    connection
        .execute(
            "UPDATE threadline_schema_migrations SET sha256 = ?1 WHERE version = 1",
            [drifted.as_slice()],
        )
        .expect("inject ledger checksum drift");
    drop(connection);
    let before = fixture.database_family_state();

    let error = rejected_open(&fixture);

    assert_eq!(error, StorageError::MigrationLedgerInvalid);
    assert_eq!(fixture.database_family_state(), before);
}

#[test]
fn live_schema_tampering_fails_closed() {
    let fixture = DatabaseFixture::create();
    drop(fixture.open());
    let connection = fixture.open_for_fixture_inspection();
    connection
        .execute_batch("ALTER TABLE opaque_events ADD COLUMN injected_value TEXT;")
        .expect("inject live schema drift");
    drop(connection);
    let before = fixture.database_family_state();

    let error = rejected_open(&fixture);

    assert_eq!(error, StorageError::MigrationLedgerInvalid);
    assert_eq!(fixture.database_family_state(), before);
}

#[test]
fn normal_record_operations_use_the_owning_encrypted_interface() {
    let fixture = DatabaseFixture::create();
    let channel = ConversationRef::channel("channel-a").expect("valid channel");
    let direct_message = ConversationRef::direct_message("dm-a").expect("valid dm");
    let golden = decode_hex(include_str!(
        "../../../proto/golden/v1/channel-event-envelope.golden.hex"
    ));
    let mut database = fixture.open();

    for (event_id, sequence) in [
        ("evt-max", u64::MAX),
        ("evt-zero", 0),
        ("evt-i64-max", i64::MAX as u64),
    ] {
        database
            .insert_committed_event(CommittedEvent {
                tenant_id: "tenant-a",
                conversation: channel,
                event_id,
                committed_sequence: sequence,
                envelope_bytes: &golden,
            })
            .expect("store opaque event");
    }
    assert_eq!(
        database
            .list_committed_sequences("tenant-a", channel)
            .expect("list ordered sequences"),
        vec![0, i64::MAX as u64, u64::MAX]
    );
    assert_eq!(
        database.insert_committed_event(CommittedEvent {
            tenant_id: "tenant-a",
            conversation: channel,
            event_id: "evt-duplicate-sequence",
            committed_sequence: u64::MAX,
            envelope_bytes: &golden,
        }),
        Err(StorageError::Conflict)
    );
    database
        .insert_committed_event(CommittedEvent {
            tenant_id: "tenant-a",
            conversation: direct_message,
            event_id: "evt-other-scope",
            committed_sequence: u64::MAX,
            envelope_bytes: &golden,
        })
        .expect("scope sequence uniqueness by conversation");
}

#[test]
fn outbox_and_cursor_semantics_survive_correct_key_reopen() {
    let fixture = DatabaseFixture::create();
    let channel = ConversationRef::channel("channel-a").expect("valid channel");
    let pending_bytes = [0x0A, 0x05, b'e', b'v', b't', b'-', b'1', 0x82, 0xB5, 0x18];
    let cursor_bytes = [0x18, 0x01];
    let mut database = fixture.open();
    database
        .enqueue_outbox(PendingOutboxEvent {
            tenant_id: "tenant-a",
            conversation: channel,
            event_id: "evt-1",
            idempotency_key: "idem-1",
            envelope_bytes: &pending_bytes,
        })
        .expect("enqueue pending event");
    assert_eq!(
        database.enqueue_outbox(PendingOutboxEvent {
            tenant_id: "tenant-a",
            conversation: channel,
            event_id: "evt-1",
            idempotency_key: "idem-conflicting-event",
            envelope_bytes: &pending_bytes,
        }),
        Err(StorageError::Conflict)
    );
    assert_eq!(
        database.enqueue_outbox(PendingOutboxEvent {
            tenant_id: "tenant-a",
            conversation: channel,
            event_id: "evt-conflicting-idempotency",
            idempotency_key: "idem-1",
            envelope_bytes: &pending_bytes,
        }),
        Err(StorageError::Conflict)
    );
    database
        .enqueue_outbox(PendingOutboxEvent {
            tenant_id: "tenant-a",
            conversation: ConversationRef::channel("channel-b").expect("valid channel"),
            event_id: "evt-other-channel",
            idempotency_key: "idem-1",
            envelope_bytes: &pending_bytes,
        })
        .expect("scope idempotency by conversation");
    database
        .enqueue_outbox(PendingOutboxEvent {
            tenant_id: "tenant-b",
            conversation: channel,
            event_id: "evt-other-tenant",
            idempotency_key: "idem-1",
            envelope_bytes: &pending_bytes,
        })
        .expect("scope idempotency by tenant");
    database
        .record_cursor(CursorUpdate {
            tenant_id: "tenant-a",
            device_id: "device-a",
            conversation: channel,
            expected_previous_sequence: None,
            last_applied_sequence: 1,
            cursor_bytes: &cursor_bytes,
        })
        .expect("record initial cursor");
    drop(database);

    let reopened = fixture.open();
    assert_eq!(
        reopened
            .load_outbox_event("tenant-a", channel, "evt-1")
            .expect("load pending event"),
        Some(pending_bytes.to_vec())
    );
    assert_eq!(
        reopened
            .load_cursor("tenant-a", "device-a", channel)
            .expect("load cursor"),
        Some(StoredCursor {
            last_applied_sequence: 1,
            cursor_bytes: cursor_bytes.to_vec(),
        })
    );
}

#[test]
fn cursor_compare_and_swap_rejects_stale_progress_and_preserves_u64_boundaries() {
    let fixture = DatabaseFixture::create();
    let channel = ConversationRef::channel("channel-cursor").expect("valid channel");
    let mut database = fixture.open();
    let initial_cursor = [0x18, 0x00];
    let signed_max_cursor = [0x18, 0x7F];
    let unsigned_max_cursor = [0x18, 0xFF, 0x01];

    database
        .record_cursor(CursorUpdate {
            tenant_id: "tenant-a",
            device_id: "device-a",
            conversation: channel,
            expected_previous_sequence: None,
            last_applied_sequence: 0,
            cursor_bytes: &initial_cursor,
        })
        .expect("record initial cursor");
    database
        .record_cursor(CursorUpdate {
            tenant_id: "tenant-a",
            device_id: "device-a",
            conversation: channel,
            expected_previous_sequence: Some(0),
            last_applied_sequence: i64::MAX as u64,
            cursor_bytes: &signed_max_cursor,
        })
        .expect("advance through signed maximum");
    database
        .record_cursor(CursorUpdate {
            tenant_id: "tenant-a",
            device_id: "device-a",
            conversation: channel,
            expected_previous_sequence: Some(i64::MAX as u64),
            last_applied_sequence: u64::MAX,
            cursor_bytes: &unsigned_max_cursor,
        })
        .expect("advance through unsigned maximum");
    database
        .record_cursor(CursorUpdate {
            tenant_id: "tenant-a",
            device_id: "device-a",
            conversation: channel,
            expected_previous_sequence: Some(u64::MAX),
            last_applied_sequence: u64::MAX,
            cursor_bytes: &unsigned_max_cursor,
        })
        .expect("accept exact idempotent cursor replay");

    assert_eq!(
        database
            .load_cursor("tenant-a", "device-a", channel)
            .expect("load maximum cursor"),
        Some(StoredCursor {
            last_applied_sequence: u64::MAX,
            cursor_bytes: unsigned_max_cursor.to_vec(),
        })
    );
    assert_eq!(
        database.record_cursor(CursorUpdate {
            tenant_id: "tenant-a",
            device_id: "device-a",
            conversation: channel,
            expected_previous_sequence: Some(u64::MAX),
            last_applied_sequence: u64::MAX,
            cursor_bytes: &[0x18, 0x01],
        }),
        Err(StorageError::Conflict)
    );
    assert_eq!(
        database.record_cursor(CursorUpdate {
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
fn public_record_errors_are_stable_and_secret_safe() {
    let fixture = DatabaseFixture::create();
    let conversation = ConversationRef::channel("channel-secret").expect("valid channel");
    let mut database = fixture.open();
    let error = database
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

fn rejected_open(fixture: &DatabaseFixture) -> StorageError {
    match EncryptedDatabase::open(
        &fixture.path,
        DatabaseKey::new(fixture.key).expect("accept generated fixed-size key"),
    ) {
        Ok(_) => panic!("invalid fixture unexpectedly opened"),
        Err(error) => error,
    }
}

fn pragma_i64(connection: &Connection, name: &str) -> i64 {
    connection
        .query_row(&format!("PRAGMA {name}"), [], |row| row.get(0))
        .expect("read integer pragma")
}

fn decode_hex(input: &str) -> Vec<u8> {
    let input = input.trim();
    assert!(input.len().is_multiple_of(2), "fixture has whole bytes");
    (0..input.len())
        .step_by(2)
        .map(|index| u8::from_str_radix(&input[index..index + 2], 16).expect("valid fixture hex"))
        .collect()
}

struct DatabaseFixture {
    directory: PathBuf,
    path: PathBuf,
    key: [u8; 32],
}

impl DatabaseFixture {
    fn create() -> Self {
        let mut nonce = [0_u8; 16];
        let mut key = [0_u8; 32];
        getrandom::fill(&mut nonce).expect("obtain runtime directory nonce");
        getrandom::fill(&mut key).expect("obtain runtime database key");
        if key.iter().all(|byte| *byte == 0) {
            key[0] = 1;
        }
        let suffix = nonce
            .iter()
            .map(|byte| format!("{byte:02x}"))
            .collect::<String>();
        let directory = std::env::temp_dir().join(format!("threadline-storage-{suffix}"));
        fs::create_dir(&directory).expect("create isolated storage test directory");
        let path = directory.join("client-core.db");
        Self {
            directory,
            path,
            key,
        }
    }

    fn open(&self) -> EncryptedDatabase {
        EncryptedDatabase::open(
            &self.path,
            DatabaseKey::new(self.key).expect("accept generated fixed-size key"),
        )
        .expect("open encrypted database through production interface")
    }

    fn open_empty_keyed_fixture(&self) -> Connection {
        let connection = Connection::open(&self.path).expect("open encrypted fixture");
        apply_raw_key(&connection, &self.key);
        connection
    }

    fn open_for_fixture_inspection(&self) -> Connection {
        let connection = self.open_empty_keyed_fixture();
        connection
            .query_row("SELECT count(*) FROM sqlite_schema", [], |row| {
                row.get::<_, i64>(0)
            })
            .expect("verify fixture key");
        connection
    }

    fn database_family_state(&self) -> [Option<Vec<u8>>; 3] {
        let suffix_path = |suffix: &str| {
            let mut path = self.path.as_os_str().to_os_string();
            path.push(suffix);
            PathBuf::from(path)
        };
        [self.path.clone(), suffix_path("-wal"), suffix_path("-shm")].map(|path| {
            path.exists()
                .then(|| fs::read(path).expect("read encrypted database-family member"))
        })
    }
}

impl Drop for DatabaseFixture {
    fn drop(&mut self) {
        let _ = fs::remove_dir_all(&self.directory);
    }
}

fn apply_raw_key(connection: &Connection, key: &[u8; 32]) {
    let mut command = Zeroizing::new(String::with_capacity(86));
    command.push_str("PRAGMA key = \"x'");
    for byte in key {
        write!(&mut command, "{byte:02x}").expect("encode fixture key");
    }
    command.push_str("'\";");
    connection
        .execute_batch(&command)
        .expect("apply fixture inspection key");
}

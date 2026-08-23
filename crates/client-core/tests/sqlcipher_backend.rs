use std::{
    ffi::OsString,
    fs,
    path::{Path, PathBuf},
};

use rusqlite::Connection;
use threadline_client_core::storage::{CommittedEvent, ConversationRef, Storage};

const SQLITE_HEADER: &[u8] = b"SQLite format 3\0";

#[test]
fn community_sqlcipher_is_keyed_fail_closed_and_preserves_opaque_bytes() {
    let test_directory = EphemeralDirectory::create();
    let database_path = test_directory.path().join("client-core.db");
    let mut key = [0_u8; 32];
    let mut wrong_key = [0_u8; 32];
    let mut canary = [0_u8; 32];
    getrandom::fill(&mut key).expect("obtain a runtime database key from the OS CSPRNG");
    getrandom::fill(&mut wrong_key).expect("obtain a distinct runtime key from the OS CSPRNG");
    getrandom::fill(&mut canary).expect("obtain a runtime fixture canary from the OS CSPRNG");
    if wrong_key == key {
        wrong_key[0] ^= 1;
    }

    let mut connection =
        Connection::open(&database_path).expect("create SQLCipher evidence database");
    apply_raw_key(&connection, &key).expect("apply runtime database key");
    let cipher_version: String = connection
        .query_row("PRAGMA cipher_version", [], |row| row.get(0))
        .expect("query linked SQLCipher version");
    assert_eq!(cipher_version, "4.14.0 community");
    let journal_mode: String = connection
        .query_row("PRAGMA journal_mode = WAL", [], |row| row.get(0))
        .expect("enable WAL evidence");
    assert_eq!(journal_mode.to_ascii_lowercase(), "wal");

    let conversation = ConversationRef::channel("channel-sqlcipher").expect("valid conversation");
    let golden = decode_hex(include_str!(
        "../../../proto/golden/v1/channel-event-envelope.golden.hex"
    ));
    let mut storage = Storage::attach(&mut connection).expect("attach keyed storage");
    storage
        .insert_committed_event(CommittedEvent {
            tenant_id: "tenant-sqlcipher",
            conversation,
            event_id: "evt-golden",
            committed_sequence: 1,
            envelope_bytes: &golden,
        })
        .expect("store Golden Envelope bytes");
    storage
        .insert_committed_event(CommittedEvent {
            tenant_id: "tenant-sqlcipher",
            conversation,
            event_id: "evt-canary",
            committed_sequence: 2,
            envelope_bytes: &canary,
        })
        .expect("store runtime fixture canary");
    let loaded = storage
        .load_committed_event("tenant-sqlcipher", conversation, "evt-golden")
        .expect("load Golden Envelope bytes")
        .expect("Golden Envelope row exists");
    assert!(loaded == golden, "Golden Envelope bytes changed");

    let schema_before_rejection = schema_state(&connection);
    let family_paths = database_family(&database_path);
    let family_before_rejection = snapshot_database_family(&family_paths);
    for bytes in &family_before_rejection {
        assert_hidden(bytes, SQLITE_HEADER, "SQLite header is visible at rest");
        assert_hidden(bytes, &canary, "fixture canary is visible at rest");
        assert_hidden(bytes, &key, "database key is visible at rest");
    }

    assert_rejected_without_rewrite(
        &database_path,
        &family_paths,
        &wrong_key,
        &family_before_rejection,
    );
    assert_rejected_without_rewrite(&database_path, &family_paths, &[], &family_before_rejection);
    drop(connection);

    let mut reopened = Connection::open(&database_path).expect("reopen evidence database");
    apply_raw_key(&reopened, &key).expect("reapply correct runtime key");
    verify_key(&reopened).expect("accept correct runtime key");
    assert!(
        schema_state(&reopened) == schema_before_rejection,
        "schema version or migration ledger changed after rejected opens"
    );
    let storage = Storage::attach(&mut reopened).expect("reattach keyed storage");
    let reopened_golden = storage
        .load_committed_event("tenant-sqlcipher", conversation, "evt-golden")
        .expect("load Golden Envelope after reopen")
        .expect("Golden Envelope remains present");
    assert!(
        reopened_golden == golden,
        "Golden Envelope bytes changed after keyed reopen"
    );
}

fn apply_raw_key(connection: &Connection, key: &[u8]) -> rusqlite::Result<()> {
    if key.is_empty() {
        return connection.execute_batch("PRAGMA key = '';");
    }

    let mut hexadecimal_key = String::with_capacity(key.len() * 2);
    for byte in key {
        use std::fmt::Write as _;
        write!(&mut hexadecimal_key, "{byte:02x}").expect("write hexadecimal key");
    }
    connection.execute_batch(&format!("PRAGMA key = \"x'{hexadecimal_key}'\";"))
}

fn verify_key(connection: &Connection) -> rusqlite::Result<i64> {
    connection.query_row("SELECT count(*) FROM sqlite_schema", [], |row| row.get(0))
}

fn assert_rejected_without_rewrite(
    database_path: &Path,
    family_paths: &[PathBuf; 3],
    key: &[u8],
    original: &[Vec<u8>; 3],
) {
    let connection = Connection::open(database_path).expect("open database for rejection check");
    let rejected = match apply_raw_key(&connection, key) {
        Ok(()) => verify_key(&connection).is_err(),
        Err(_) => true,
    };
    assert!(rejected, "invalid key unexpectedly opened database");
    drop(connection);

    assert!(
        snapshot_database_family(family_paths) == *original,
        "rejected open rewrote the DB/WAL/SHM family"
    );
}

fn snapshot_database_family(family_paths: &[PathBuf; 3]) -> [Vec<u8>; 3] {
    std::array::from_fn(|index| {
        assert!(
            family_paths[index].exists(),
            "expected DB/WAL/SHM evidence file"
        );
        fs::read(&family_paths[index]).expect("read encrypted evidence file")
    })
}

#[derive(Eq, PartialEq)]
struct SchemaState {
    user_version: i64,
    objects: Vec<(String, String, String, Option<String>)>,
    migration_ledger: Vec<(i64, Vec<u8>)>,
}

fn schema_state(connection: &Connection) -> SchemaState {
    let user_version = connection
        .query_row("PRAGMA user_version", [], |row| row.get(0))
        .expect("read schema version");
    let mut schema_statement = connection
        .prepare(
            "SELECT type, name, tbl_name, sql
             FROM sqlite_schema
             ORDER BY type, name, tbl_name",
        )
        .expect("prepare schema snapshot");
    let objects = schema_statement
        .query_map([], |row| {
            Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?))
        })
        .expect("query schema snapshot")
        .collect::<Result<Vec<_>, _>>()
        .expect("read schema snapshot");
    let mut ledger_statement = connection
        .prepare(
            "SELECT version, sha256
             FROM threadline_schema_migrations
             ORDER BY version",
        )
        .expect("prepare migration-ledger snapshot");
    let migration_ledger = ledger_statement
        .query_map([], |row| Ok((row.get(0)?, row.get(1)?)))
        .expect("query migration-ledger snapshot")
        .collect::<Result<Vec<_>, _>>()
        .expect("read migration-ledger snapshot");

    SchemaState {
        user_version,
        objects,
        migration_ledger,
    }
}

fn assert_hidden(haystack: &[u8], needle: &[u8], message: &str) {
    assert!(
        !haystack
            .windows(needle.len())
            .any(|window| window == needle),
        "{message}"
    );
}

fn database_family(database_path: &Path) -> [PathBuf; 3] {
    let suffix_path = |suffix: &str| {
        let mut path = OsString::from(database_path.as_os_str());
        path.push(suffix);
        PathBuf::from(path)
    };
    [
        database_path.to_path_buf(),
        suffix_path("-wal"),
        suffix_path("-shm"),
    ]
}

fn decode_hex(input: &str) -> Vec<u8> {
    let input = input.trim();
    assert!(
        input.len().is_multiple_of(2),
        "Golden Envelope fixture has odd hex length"
    );
    (0..input.len())
        .step_by(2)
        .map(|index| u8::from_str_radix(&input[index..index + 2], 16).expect("valid fixture hex"))
        .collect()
}

struct EphemeralDirectory(PathBuf);

impl EphemeralDirectory {
    fn create() -> Self {
        let mut nonce = [0_u8; 16];
        getrandom::fill(&mut nonce).expect("obtain a runtime test-directory nonce");
        let suffix = nonce
            .iter()
            .map(|byte| format!("{byte:02x}"))
            .collect::<String>();
        let path = std::env::temp_dir().join(format!("threadline-sqlcipher-{suffix}"));
        fs::create_dir(&path).expect("create isolated SQLCipher test directory");
        Self(path)
    }

    fn path(&self) -> &Path {
        &self.0
    }
}

impl Drop for EphemeralDirectory {
    fn drop(&mut self) {
        let _ = fs::remove_dir_all(&self.0);
    }
}

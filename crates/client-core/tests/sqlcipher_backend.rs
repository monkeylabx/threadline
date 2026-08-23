use std::{
    ffi::OsString,
    fs,
    path::{Path, PathBuf},
};

use threadline_client_core::storage::{
    CommittedEvent, ConversationRef, DatabaseKey, EncryptedDatabase, StorageError,
};

const SQLITE_HEADER: &[u8] = b"SQLite format 3\0";

#[test]
fn production_interface_is_keyed_fail_closed_and_preserves_opaque_bytes() {
    let test_directory = EphemeralDirectory::create();
    let database_path = test_directory.path().join("client-core.db");
    let key = random_distinct_from(&[]);
    let wrong_key = random_distinct_from(&key);
    let canary = random_distinct_from(&key);
    let conversation = ConversationRef::channel("channel-sqlcipher").expect("valid conversation");
    let golden = decode_hex(include_str!(
        "../../../proto/golden/v1/channel-event-envelope.golden.hex"
    ));

    let mut database = EncryptedDatabase::open(
        &database_path,
        DatabaseKey::new(key).expect("accept generated fixed-size key"),
    )
    .expect("create encrypted database through the production interface");
    database
        .insert_committed_event(CommittedEvent {
            tenant_id: "tenant-sqlcipher",
            conversation,
            event_id: "evt-golden",
            committed_sequence: 1,
            envelope_bytes: &golden,
        })
        .expect("store Golden Envelope bytes");
    database
        .insert_committed_event(CommittedEvent {
            tenant_id: "tenant-sqlcipher",
            conversation,
            event_id: "evt-canary",
            committed_sequence: 2,
            envelope_bytes: &canary,
        })
        .expect("store runtime fixture canary");
    assert_eq!(
        database
            .load_committed_event("tenant-sqlcipher", conversation, "evt-golden")
            .expect("load Golden Envelope bytes"),
        Some(golden.clone())
    );

    let family_paths = database_family(&database_path);
    let family_before_rejection = snapshot_database_family(&family_paths);
    for bytes in &family_before_rejection {
        assert_hidden(bytes, SQLITE_HEADER, "SQLite header is visible at rest");
        assert_hidden(bytes, &canary, "fixture canary is visible at rest");
        assert_hidden(bytes, &key, "database key is visible at rest");
        assert_hidden(bytes, &wrong_key, "rejected key is visible at rest");
    }

    let error = match EncryptedDatabase::open(
        &database_path,
        DatabaseKey::new(wrong_key).expect("accept distinct fixed-size key"),
    ) {
        Ok(_) => panic!("wrong key unexpectedly opened database"),
        Err(error) => error,
    };
    assert_eq!(error, StorageError::Database);
    let family_unchanged = database_family_matches(&family_paths, &family_before_rejection);
    drop(database);
    assert!(
        family_unchanged,
        "wrong-key open rewrote the DB/WAL/SHM family"
    );
    let reopened = EncryptedDatabase::open(
        &database_path,
        DatabaseKey::new(key).expect("accept generated fixed-size key"),
    )
    .expect("reopen with the correct key");
    assert_eq!(
        reopened
            .load_committed_event("tenant-sqlcipher", conversation, "evt-golden")
            .expect("load Golden Envelope after reopen"),
        Some(golden)
    );
}

#[test]
fn empty_key_is_unrepresentable_and_does_not_rewrite_database_family() {
    let test_directory = EphemeralDirectory::create();
    let database_path = test_directory.path().join("client-core.db");
    let family_paths = database_family(&database_path);
    for (index, path) in family_paths.iter().enumerate() {
        fs::write(path, vec![index as u8 + 1; 64 + index])
            .expect("create immutable database-family sentinel");
    }
    let family_before_rejection = snapshot_database_family(&family_paths);

    let error = match DatabaseKey::new([0_u8; 32]) {
        Ok(_) => panic!("empty key sentinel unexpectedly accepted"),
        Err(error) => error,
    };

    assert_eq!(error, StorageError::InvalidKey);
    let family_unchanged = database_family_matches(&family_paths, &family_before_rejection);
    assert!(
        family_unchanged,
        "empty-key rejection rewrote the DB/WAL/SHM family"
    );
}

#[test]
fn public_errors_do_not_echo_key_or_path_material() {
    let test_directory = EphemeralDirectory::create();
    let secret_path_fragment = "path-secret-canary";
    let database_path = test_directory.path().join(secret_path_fragment);
    let key = random_distinct_from(&[]);
    let wrong_key = random_distinct_from(&key);
    let database = EncryptedDatabase::open(
        &database_path,
        DatabaseKey::new(key).expect("accept generated fixed-size key"),
    )
    .expect("create encrypted database");

    let error = match EncryptedDatabase::open(
        &database_path,
        DatabaseKey::new(wrong_key).expect("accept distinct fixed-size key"),
    ) {
        Ok(_) => panic!("wrong key unexpectedly opened database"),
        Err(error) => error,
    };
    let display = error.to_string();
    let debug = format!("{error:?}");

    assert_eq!(display, "storage_database_error");
    assert!(!display.contains(secret_path_fragment));
    assert!(!debug.contains(secret_path_fragment));
    assert!(!display.contains(&encode_hex(&wrong_key)));
    assert!(!debug.contains(&encode_hex(&wrong_key)));
    drop(database);
}

#[test]
fn corrupted_ciphertext_is_not_misclassified_as_a_confirmed_key_error() {
    let test_directory = EphemeralDirectory::create();
    let database_path = test_directory.path().join("client-core.db");
    let key = random_distinct_from(&[]);
    let database = EncryptedDatabase::open(
        &database_path,
        DatabaseKey::new(key).expect("accept generated fixed-size key"),
    )
    .expect("create encrypted database");
    drop(database);

    let mut corrupted = fs::read(&database_path).expect("read encrypted fixture");
    assert!(corrupted.len() > 100, "encrypted fixture has a first page");
    corrupted[100] ^= 0x80;
    fs::write(&database_path, &corrupted).expect("inject ciphertext corruption");

    let error = match EncryptedDatabase::open(
        &database_path,
        DatabaseKey::new(key).expect("accept original fixed-size key"),
    ) {
        Ok(_) => panic!("corrupted ciphertext unexpectedly opened"),
        Err(error) => error,
    };

    assert_eq!(error, StorageError::Database);
    assert_eq!(error.code(), "storage_database_error");
    assert!(
        fs::read(&database_path)
            .map(|actual| actual == corrupted)
            .unwrap_or(false),
        "corrupt database rejection rewrote ciphertext"
    );
}

fn snapshot_database_family(family_paths: &[PathBuf; 3]) -> [Vec<u8>; 3] {
    std::array::from_fn(|index| {
        assert!(
            family_paths[index].exists(),
            "expected encrypted database-family member"
        );
        fs::read(&family_paths[index]).expect("read encrypted database-family member")
    })
}

fn database_family_matches(family_paths: &[PathBuf; 3], expected: &[Vec<u8>; 3]) -> bool {
    family_paths
        .iter()
        .zip(expected)
        .all(|(path, expected_bytes)| {
            fs::read(path)
                .map(|actual_bytes| actual_bytes == *expected_bytes)
                .unwrap_or(false)
        })
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
    assert!(input.len().is_multiple_of(2), "fixture has whole bytes");
    (0..input.len())
        .step_by(2)
        .map(|index| u8::from_str_radix(&input[index..index + 2], 16).expect("valid fixture hex"))
        .collect()
}

fn encode_hex(input: &[u8]) -> String {
    input.iter().map(|byte| format!("{byte:02x}")).collect()
}

fn random_distinct_from(other: &[u8]) -> [u8; 32] {
    loop {
        let mut value = [0_u8; 32];
        getrandom::fill(&mut value).expect("obtain runtime test bytes from the OS CSPRNG");
        if value != other {
            return value;
        }
    }
}

struct EphemeralDirectory(PathBuf);

impl EphemeralDirectory {
    fn create() -> Self {
        let nonce = random_distinct_from(&[]);
        let suffix = encode_hex(&nonce[..16]);
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

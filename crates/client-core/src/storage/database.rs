use std::{
    fmt::Write as _,
    path::{Path, PathBuf},
    time::Duration,
};

#[cfg(target_os = "windows")]
use std::sync::{Mutex, MutexGuard, OnceLock};

use rusqlite::{Connection, OpenFlags};
use zeroize::{Zeroize, Zeroizing};

use super::{migrate, StorageError};

const DATABASE_KEY_BYTES: usize = 32;
#[cfg(target_os = "windows")]
const DATABASE_OPEN_STACK_BYTES: usize = 8 * 1024 * 1024;
const EXPECTED_CIPHER_VERSION: &str = "4.14.0 community";

/// A host-supplied SQLCipher key.
///
/// The key is fixed-size, cannot be cloned or formatted, and is zeroized after
/// [`EncryptedDatabase::open`] applies it. Key generation, persistence,
/// rotation, and OS Secure Storage are owned by P05-05.
pub struct DatabaseKey([u8; DATABASE_KEY_BYTES]);

impl DatabaseKey {
    pub fn new(bytes: [u8; DATABASE_KEY_BYTES]) -> Result<Self, StorageError> {
        if bytes.iter().all(|byte| *byte == 0) {
            return Err(StorageError::InvalidKey);
        }
        Ok(Self(bytes))
    }
}

impl Drop for DatabaseKey {
    fn drop(&mut self) {
        self.0.zeroize();
    }
}

/// The owning encrypted database module used by client-core callers.
///
/// A successful open guarantees a Community SQLCipher connection, a validated
/// Threadline schema at the current version, foreign-key enforcement, WAL
/// journal mode, and full synchronous durability. The raw connection and key
/// application sequence are intentionally not exposed.
pub struct EncryptedDatabase {
    pub(super) connection: Connection,
}

impl EncryptedDatabase {
    pub fn open(path: impl AsRef<Path>, key: DatabaseKey) -> Result<Self, StorageError> {
        let path = path.as_ref().to_path_buf();

        #[cfg(target_os = "windows")]
        {
            let _open_guard = windows_database_open_guard();

            // Three hosted Windows runs overflowed the default test-thread
            // stack during this complete secure construction path. P05-02
            // will replace this short-lived worker with the longer-lived
            // StoreActor boundary.
            let result = std::thread::Builder::new()
                .name("threadline-database-open".to_owned())
                .stack_size(DATABASE_OPEN_STACK_BYTES)
                .spawn(move || Self::open_on_current_thread(path, key))
                .map_err(|_| StorageError::Database)?
                .join();

            return result.unwrap_or_else(|payload| std::panic::resume_unwind(payload));
        }

        #[cfg(not(target_os = "windows"))]
        Self::open_on_current_thread(path, key)
    }

    fn open_on_current_thread(path: PathBuf, key: DatabaseKey) -> Result<Self, StorageError> {
        let mut connection = Connection::open_with_flags(
            path,
            OpenFlags::SQLITE_OPEN_READ_WRITE
                | OpenFlags::SQLITE_OPEN_CREATE
                | OpenFlags::SQLITE_OPEN_NO_MUTEX,
        )
        .map_err(|_| StorageError::Database)?;

        apply_key(&connection, &key)?;
        enable_enhanced_memory_security(&connection)?;
        verify_community_cipher(&connection)?;
        verify_key(&connection)?;
        connection
            .pragma_update(None, "foreign_keys", "ON")
            .map_err(|_| StorageError::Database)?;
        if pragma_i64(&connection, "foreign_keys")? != 1 {
            return Err(StorageError::Database);
        }

        // Reject foreign, newer, or tampered schemas before any WAL/durability
        // configuration can change an existing database family.
        migrate(&mut connection)?;
        configure_durability(&connection)?;

        Ok(Self { connection })
    }
}

fn enable_enhanced_memory_security(connection: &Connection) -> Result<(), StorageError> {
    #[cfg(target_os = "windows")]
    {
        // This SQLCipher setting is process-wide and cannot be disabled after
        // it is enabled. Avoid re-running its Windows allocator setup for each
        // connection while retaining the enhanced memory-security policy.
        static MEMORY_SECURITY: OnceLock<Result<(), ()>> = OnceLock::new();
        return MEMORY_SECURITY
            .get_or_init(|| {
                connection
                    .pragma_update(None, "cipher_memory_security", "ON")
                    .map_err(|_| ())
            })
            .map_err(|()| StorageError::EncryptionUnavailable);
    }

    #[cfg(not(target_os = "windows"))]
    connection
        .pragma_update(None, "cipher_memory_security", "ON")
        .map_err(|_| StorageError::EncryptionUnavailable)
}

#[cfg(target_os = "windows")]
fn windows_database_open_guard() -> MutexGuard<'static, ()> {
    // Limit concurrent short-lived workers and their 8 MiB stack reservations.
    // This guard is only a scheduling token, so a poisoned lock is recoverable.
    static DATABASE_OPEN: OnceLock<Mutex<()>> = OnceLock::new();
    DATABASE_OPEN
        .get_or_init(|| Mutex::new(()))
        .lock()
        .unwrap_or_else(|poisoned| poisoned.into_inner())
}

fn apply_key(connection: &Connection, key: &DatabaseKey) -> Result<(), StorageError> {
    let mut command = Zeroizing::new(String::with_capacity(22 + DATABASE_KEY_BYTES * 2));
    command.push_str("PRAGMA key = \"x'");
    for byte in &key.0 {
        write!(&mut command, "{byte:02x}").map_err(|_| StorageError::Database)?;
    }
    command.push_str("'\";");
    connection
        .execute_batch(&command)
        .map_err(|_| StorageError::Database)
}

fn verify_community_cipher(connection: &Connection) -> Result<(), StorageError> {
    let version = connection
        .query_row("PRAGMA cipher_version", [], |row| row.get::<_, String>(0))
        .map_err(|_| StorageError::EncryptionUnavailable)?;
    if version != EXPECTED_CIPHER_VERSION {
        return Err(StorageError::EncryptionUnavailable);
    }
    Ok(())
}

fn verify_key(connection: &Connection) -> Result<(), StorageError> {
    connection
        .query_row("SELECT count(*) FROM sqlite_schema", [], |row| {
            row.get::<_, i64>(0)
        })
        .map(|_| ())
        // SQLCipher deliberately does not provide a reliable distinction
        // between a wrong key, damaged ciphertext, and an underlying read/I/O
        // failure. Do not misclassify those cases as a confirmed key error.
        .map_err(|_| StorageError::Database)
}

fn pragma_i64(connection: &Connection, name: &str) -> Result<i64, StorageError> {
    let query = match name {
        "foreign_keys" => "PRAGMA foreign_keys",
        _ => return Err(StorageError::Database),
    };
    connection
        .query_row(query, [], |row| row.get(0))
        .map_err(|_| StorageError::Database)
}

fn configure_durability(connection: &Connection) -> Result<(), StorageError> {
    let journal_mode = connection
        .query_row("PRAGMA journal_mode = WAL", [], |row| {
            row.get::<_, String>(0)
        })
        .map_err(|_| StorageError::Database)?;
    if !journal_mode.eq_ignore_ascii_case("wal") {
        return Err(StorageError::Database);
    }
    connection
        .pragma_update(None, "synchronous", "FULL")
        .map_err(|_| StorageError::Database)?;
    let synchronous = connection
        .query_row("PRAGMA synchronous", [], |row| row.get::<_, i64>(0))
        .map_err(|_| StorageError::Database)?;
    if synchronous != 2 {
        return Err(StorageError::Database);
    }
    connection
        .busy_timeout(Duration::from_secs(5))
        .map_err(|_| StorageError::Database)
}

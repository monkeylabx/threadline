//! Owning, encrypted local database for opaque client state.
//!
//! Hosts supply a fixed-size [`DatabaseKey`] and a file path. This module owns
//! SQLCipher connection ordering, cipher verification, durability settings,
//! schema validation, migration, and all database access after a successful
//! [`EncryptedDatabase::open`].

use std::fmt;

use rusqlite::{Connection, TransactionBehavior};

mod database;
mod outbox;
mod records;

pub use database::{DatabaseKey, EncryptedDatabase};
pub use outbox::{CommitPending, CommittedEventRecord, PendingOutboxRecord};
pub use records::{
    CommittedEvent, ConversationKind, ConversationRef, CursorUpdate, PendingOutboxEvent,
    StoredCursor,
};

const MIGRATION_0001_SQL: &str = include_str!("migrations/0001.sql");
const MIGRATION_0002_SQL: &str = include_str!("migrations/0002.sql");
/// The complete schema after every migration, compared statement-for-statement
/// with `sqlite_schema` so a tampered or partially migrated database fails closed.
const CURRENT_SCHEMA_SQL: &str = include_str!("schema/current.sql");
// SHA-256 of the exact checked-in `migrations/0001.sql` bytes.
const MIGRATION_0001_SHA256: [u8; 32] = [
    0x61, 0x16, 0xaa, 0xca, 0xec, 0x95, 0x84, 0x15, 0xb7, 0x73, 0xbd, 0xd1, 0xe2, 0xf9, 0xe3, 0x4d,
    0x7d, 0x9f, 0x55, 0xf3, 0x0d, 0xef, 0x8a, 0x5b, 0x8d, 0x9a, 0xce, 0x11, 0xbf, 0x63, 0x37, 0x43,
];
// SHA-256 of the exact checked-in `migrations/0002.sql` bytes.
const MIGRATION_0002_SHA256: [u8; 32] = [
    0x30, 0x54, 0xe3, 0x47, 0x8e, 0x49, 0x64, 0xdf, 0xa6, 0x5a, 0x65, 0xcf, 0x8d, 0x41, 0x1b, 0x96,
    0x89, 0x0c, 0x21, 0xd1, 0x01, 0x17, 0x9b, 0x1d, 0x26, 0xa9, 0xf8, 0xa9, 0xc2, 0x6e, 0x5e, 0x3b,
];

/// Ordered migration ledger: version, SQL, and the digest recorded on apply.
const MIGRATIONS: [(i64, &str, [u8; 32]); 2] = [
    (1, MIGRATION_0001_SQL, MIGRATION_0001_SHA256),
    (2, MIGRATION_0002_SQL, MIGRATION_0002_SHA256),
];

/// ASCII `TLIN`, used to reject unrelated SQLite files.
pub(crate) const APPLICATION_ID: i64 = 1_414_285_646;
pub(crate) const LATEST_SCHEMA_VERSION: i64 = 2;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum StorageError {
    Database,
    Conflict,
    EncryptionUnavailable,
    ForeignDatabase,
    InvalidKey,
    InvalidInput,
    NewerSchema,
    MigrationLedgerInvalid,
}

impl StorageError {
    pub const fn code(self) -> &'static str {
        match self {
            Self::Database => "storage_database_error",
            Self::Conflict => "storage_conflict",
            Self::EncryptionUnavailable => "storage_encryption_unavailable",
            Self::ForeignDatabase => "storage_foreign_database",
            Self::InvalidKey => "storage_invalid_key",
            Self::InvalidInput => "storage_invalid_input",
            Self::NewerSchema => "storage_newer_schema",
            Self::MigrationLedgerInvalid => "storage_migration_ledger_invalid",
        }
    }
}

impl fmt::Display for StorageError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(self.code())
    }
}

impl std::error::Error for StorageError {}

fn migrate(connection: &mut Connection) -> Result<(), StorageError> {
    let application_id = application_id(connection)?;
    let user_version = user_version(connection)?;

    if application_id != 0 && application_id != APPLICATION_ID {
        return Err(StorageError::ForeignDatabase);
    }
    if user_version > LATEST_SCHEMA_VERSION {
        return Err(StorageError::NewerSchema);
    }
    if user_version == 0 && has_user_objects(connection)? {
        return Err(StorageError::ForeignDatabase);
    }
    if user_version > 0 {
        if application_id != APPLICATION_ID {
            return Err(StorageError::ForeignDatabase);
        }
        verify_ledger(connection, user_version)?;
    }
    if user_version < LATEST_SCHEMA_VERSION {
        apply_pending_migrations(connection, user_version)?;
    }
    verify_current_schema(connection)
}

/// apply_pending_migrations applies every migration above `from_version` in
/// one transaction, so a failure anywhere leaves the version, ledger and schema
/// exactly as they were.
fn apply_pending_migrations(
    connection: &mut Connection,
    from_version: i64,
) -> Result<(), StorageError> {
    let transaction = connection
        .transaction_with_behavior(TransactionBehavior::Immediate)
        .map_err(|_| StorageError::Database)?;
    for (version, sql, digest) in MIGRATIONS
        .iter()
        .filter(|(version, _, _)| *version > from_version)
    {
        transaction
            .execute_batch(sql)
            .map_err(|_| StorageError::Database)?;
        transaction
            .execute(
                "INSERT INTO threadline_schema_migrations (version, sha256) VALUES (?1, ?2)",
                (version, digest.as_slice()),
            )
            .map_err(|_| StorageError::Database)?;
    }
    transaction
        .pragma_update(None, "application_id", APPLICATION_ID)
        .map_err(|_| StorageError::Database)?;
    transaction
        .pragma_update(None, "user_version", LATEST_SCHEMA_VERSION)
        .map_err(|_| StorageError::Database)?;
    transaction.commit().map_err(|_| StorageError::Database)
}

fn application_id(connection: &Connection) -> Result<i64, StorageError> {
    connection
        .query_row("PRAGMA application_id", [], |row| row.get(0))
        .map_err(|_| StorageError::Database)
}

fn user_version(connection: &Connection) -> Result<i64, StorageError> {
    connection
        .query_row("PRAGMA user_version", [], |row| row.get(0))
        .map_err(|_| StorageError::Database)
}

fn has_user_objects(connection: &Connection) -> Result<bool, StorageError> {
    connection
        .query_row(
            "SELECT EXISTS(SELECT 1 FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%')",
            [],
            |row| row.get(0),
        )
        .map_err(|_| StorageError::Database)
}

/// verify_ledger requires exactly one ledger row per applied version with the
/// checked-in digest, so a drifted or partially recorded history fails closed
/// before any further migration runs.
fn verify_ledger(connection: &Connection, through_version: i64) -> Result<(), StorageError> {
    let mut statement = connection
        .prepare("SELECT version, sha256 FROM threadline_schema_migrations ORDER BY version")
        .map_err(|_| StorageError::MigrationLedgerInvalid)?;
    let rows = statement
        .query_map([], |row| {
            Ok((row.get::<_, i64>(0)?, row.get::<_, Vec<u8>>(1)?))
        })
        .map_err(|_| StorageError::MigrationLedgerInvalid)?
        .collect::<Result<Vec<_>, _>>()
        .map_err(|_| StorageError::MigrationLedgerInvalid)?;
    let expected = MIGRATIONS
        .iter()
        .filter(|(version, _, _)| *version <= through_version)
        .map(|(version, _, digest)| (*version, digest.to_vec()))
        .collect::<Vec<_>>();
    if rows != expected {
        return Err(StorageError::MigrationLedgerInvalid);
    }
    Ok(())
}

fn verify_current_schema(connection: &Connection) -> Result<(), StorageError> {
    if application_id(connection)? != APPLICATION_ID
        || user_version(connection)? != LATEST_SCHEMA_VERSION
    {
        return Err(StorageError::ForeignDatabase);
    }
    verify_ledger(connection, LATEST_SCHEMA_VERSION)?;

    let mut expected_schema = CURRENT_SCHEMA_SQL
        .split(';')
        .map(str::trim)
        .filter(|statement| !statement.is_empty())
        .map(str::to_owned)
        .collect::<Vec<_>>();
    expected_schema.sort();

    let mut statement = connection
        .prepare(
            "SELECT sql FROM sqlite_schema
             WHERE name NOT LIKE 'sqlite_%' AND sql IS NOT NULL
             ORDER BY sql",
        )
        .map_err(|_| StorageError::MigrationLedgerInvalid)?;
    let actual_schema = statement
        .query_map([], |row| row.get::<_, String>(0))
        .map_err(|_| StorageError::MigrationLedgerInvalid)?
        .collect::<Result<Vec<_>, _>>()
        .map_err(|_| StorageError::MigrationLedgerInvalid)?;
    if actual_schema != expected_schema {
        return Err(StorageError::MigrationLedgerInvalid);
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use std::sync::{
        atomic::{AtomicBool, Ordering},
        Arc,
    };

    use rusqlite::{
        hooks::{AuthAction, AuthContext, Authorization},
        Connection,
    };

    use super::{has_user_objects, migrate, user_version, StorageError};

    #[test]
    fn migration_failure_rolls_back_every_schema_change() {
        let mut connection = Connection::open_in_memory().expect("open failure fixture");
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
            .expect("install deterministic migration failure");

        let error = migrate(&mut connection).expect_err("migration must fail closed");
        connection
            .authorizer(None::<fn(AuthContext<'_>) -> Authorization>)
            .expect("remove migration failure injection");

        assert_eq!(error, StorageError::Database);
        assert!(first_table_was_created.load(Ordering::SeqCst));
        assert_eq!(user_version(&connection), Ok(0));
        assert_eq!(has_user_objects(&connection), Ok(false));
    }
}

//! Owning, encrypted local database for opaque client state.
//!
//! Hosts supply a fixed-size [`DatabaseKey`] and a file path. This module owns
//! SQLCipher connection ordering, cipher verification, durability settings,
//! schema validation, migration, and all database access after a successful
//! [`EncryptedDatabase::open`].

use std::fmt;

use rusqlite::{Connection, TransactionBehavior};

mod database;
mod records;

pub use database::{DatabaseKey, EncryptedDatabase};
pub use records::{
    CommittedEvent, ConversationKind, ConversationRef, CursorUpdate, PendingOutboxEvent,
    StoredCursor,
};

const MIGRATION_0001_SQL: &str = include_str!("migrations/0001.sql");
// SHA-256 of the exact checked-in `migrations/0001.sql` bytes.
const MIGRATION_0001_SHA256: [u8; 32] = [
    0x61, 0x16, 0xaa, 0xca, 0xec, 0x95, 0x84, 0x15, 0xb7, 0x73, 0xbd, 0xd1, 0xe2, 0xf9, 0xe3, 0x4d,
    0x7d, 0x9f, 0x55, 0xf3, 0x0d, 0xef, 0x8a, 0x5b, 0x8d, 0x9a, 0xce, 0x11, 0xbf, 0x63, 0x37, 0x43,
];

/// ASCII `TLIN`, used to reject unrelated SQLite files.
pub(crate) const APPLICATION_ID: i64 = 1_414_285_646;
pub(crate) const LATEST_SCHEMA_VERSION: i64 = 1;

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
    if user_version == LATEST_SCHEMA_VERSION {
        verify_current_schema(connection)?;
        return Ok(());
    }
    if user_version != 0 || has_user_objects(connection)? {
        return Err(StorageError::ForeignDatabase);
    }

    let transaction = connection
        .transaction_with_behavior(TransactionBehavior::Immediate)
        .map_err(|_| StorageError::Database)?;
    transaction
        .execute_batch(MIGRATION_0001_SQL)
        .map_err(|_| StorageError::Database)?;
    transaction
        .execute(
            "INSERT INTO threadline_schema_migrations (version, sha256) VALUES (?1, ?2)",
            (LATEST_SCHEMA_VERSION, MIGRATION_0001_SHA256.as_slice()),
        )
        .map_err(|_| StorageError::Database)?;
    transaction
        .pragma_update(None, "application_id", APPLICATION_ID)
        .map_err(|_| StorageError::Database)?;
    transaction
        .pragma_update(None, "user_version", LATEST_SCHEMA_VERSION)
        .map_err(|_| StorageError::Database)?;
    transaction.commit().map_err(|_| StorageError::Database)?;
    verify_current_schema(connection)
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

fn verify_current_schema(connection: &Connection) -> Result<(), StorageError> {
    if application_id(connection)? != APPLICATION_ID {
        return Err(StorageError::ForeignDatabase);
    }

    let ledger = connection
        .query_row(
            "SELECT
                count(*),
                max(CASE WHEN version = ?1 THEN sha256 END)
             FROM threadline_schema_migrations",
            [LATEST_SCHEMA_VERSION],
            |row| Ok((row.get::<_, i64>(0)?, row.get::<_, Option<Vec<u8>>>(1)?)),
        )
        .map_err(|_| StorageError::MigrationLedgerInvalid)?;

    if ledger.0 != 1 || ledger.1.as_deref() != Some(MIGRATION_0001_SHA256.as_slice()) {
        return Err(StorageError::MigrationLedgerInvalid);
    }

    let mut expected_schema = MIGRATION_0001_SQL
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

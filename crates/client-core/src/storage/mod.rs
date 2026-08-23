//! Encrypted-SQLite-ready schema and storage primitives.
//!
//! This module never opens a database. The host supplies an already-opened,
//! correctly keyed connection from the later security adapter.

use std::fmt;

use rusqlite::{Connection, TransactionBehavior};

mod records;

pub use records::{
    CommittedEvent, ConversationKind, ConversationRef, CursorUpdate, PendingOutboxEvent, Storage,
    StoredCursor,
};

const MIGRATION_0001_SQL: &str = include_str!("migrations/0001.sql");
// SHA-256 of the exact checked-in `migrations/0001.sql` bytes.
const MIGRATION_0001_SHA256: [u8; 32] = [
    0x61, 0x16, 0xaa, 0xca, 0xec, 0x95, 0x84, 0x15, 0xb7, 0x73, 0xbd, 0xd1, 0xe2, 0xf9, 0xe3, 0x4d,
    0x7d, 0x9f, 0x55, 0xf3, 0x0d, 0xef, 0x8a, 0x5b, 0x8d, 0x9a, 0xce, 0x11, 0xbf, 0x63, 0x37, 0x43,
];

/// ASCII `TLIN`, used to reject unrelated SQLite files.
pub const APPLICATION_ID: i64 = 1_414_285_646;
pub const LATEST_SCHEMA_VERSION: i64 = 1;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct MigrationOutcome {
    pub from_version: i64,
    pub to_version: i64,
    pub applied_migrations: usize,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum StorageError {
    Database,
    Conflict,
    ForeignDatabase,
    InvalidInput,
    NewerSchema,
    MigrationLedgerInvalid,
}

impl StorageError {
    pub const fn code(self) -> &'static str {
        match self {
            Self::Database => "storage_database_error",
            Self::Conflict => "storage_conflict",
            Self::ForeignDatabase => "storage_foreign_database",
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

pub fn migrate(connection: &mut Connection) -> Result<MigrationOutcome, StorageError> {
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
        return Ok(MigrationOutcome {
            from_version: user_version,
            to_version: user_version,
            applied_migrations: 0,
        });
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

    Ok(MigrationOutcome {
        from_version: user_version,
        to_version: LATEST_SCHEMA_VERSION,
        applied_migrations: 1,
    })
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

    Ok(())
}

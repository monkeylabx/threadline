use rusqlite::{ffi::ErrorCode, Connection, OptionalExtension};

use super::{migrate, StorageError};

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
#[repr(i64)]
pub enum ConversationKind {
    Channel = 1,
    DirectMessage = 2,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ConversationRef<'value> {
    kind: ConversationKind,
    id: &'value str,
}

impl<'value> ConversationRef<'value> {
    pub fn channel(id: &'value str) -> Result<Self, StorageError> {
        Self::new(ConversationKind::Channel, id)
    }

    pub fn direct_message(id: &'value str) -> Result<Self, StorageError> {
        Self::new(ConversationKind::DirectMessage, id)
    }

    fn new(kind: ConversationKind, id: &'value str) -> Result<Self, StorageError> {
        validate_text(id)?;
        Ok(Self { kind, id })
    }

    fn kind_code(self) -> i64 {
        self.kind as i64
    }
}

#[derive(Clone, Copy, Debug)]
pub struct CommittedEvent<'value> {
    pub tenant_id: &'value str,
    pub conversation: ConversationRef<'value>,
    pub event_id: &'value str,
    pub committed_sequence: u64,
    pub envelope_bytes: &'value [u8],
}

#[derive(Clone, Copy, Debug)]
pub struct PendingOutboxEvent<'value> {
    pub tenant_id: &'value str,
    pub conversation: ConversationRef<'value>,
    pub event_id: &'value str,
    pub idempotency_key: &'value str,
    pub envelope_bytes: &'value [u8],
}

#[derive(Clone, Copy, Debug)]
pub struct CursorUpdate<'value> {
    pub tenant_id: &'value str,
    pub device_id: &'value str,
    pub conversation: ConversationRef<'value>,
    pub expected_previous_sequence: Option<u64>,
    pub last_applied_sequence: u64,
    pub cursor_bytes: &'value [u8],
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StoredCursor {
    pub last_applied_sequence: u64,
    pub cursor_bytes: Vec<u8>,
}

pub struct Storage<'connection> {
    connection: &'connection mut Connection,
}

impl<'connection> Storage<'connection> {
    pub fn attach(connection: &'connection mut Connection) -> Result<Self, StorageError> {
        migrate(connection)?;
        Ok(Self { connection })
    }

    pub fn insert_committed_event(
        &mut self,
        event: CommittedEvent<'_>,
    ) -> Result<(), StorageError> {
        validate_text(event.tenant_id)?;
        validate_text(event.event_id)?;
        validate_bytes(event.envelope_bytes)?;
        let sequence = event.committed_sequence.to_be_bytes();

        self.connection
            .execute(
                "INSERT INTO opaque_events (
                    tenant_id, conversation_kind, conversation_id, event_id,
                    committed_sequence, envelope_bytes
                 ) VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
                (
                    event.tenant_id,
                    event.conversation.kind_code(),
                    event.conversation.id,
                    event.event_id,
                    sequence.as_slice(),
                    event.envelope_bytes,
                ),
            )
            .map_err(map_write_error)?;
        Ok(())
    }

    pub fn load_committed_event(
        &self,
        tenant_id: &str,
        conversation: ConversationRef<'_>,
        event_id: &str,
    ) -> Result<Option<Vec<u8>>, StorageError> {
        validate_text(tenant_id)?;
        validate_text(event_id)?;
        self.connection
            .query_row(
                "SELECT envelope_bytes FROM opaque_events
                 WHERE tenant_id = ?1 AND conversation_kind = ?2
                   AND conversation_id = ?3 AND event_id = ?4",
                (
                    tenant_id,
                    conversation.kind_code(),
                    conversation.id,
                    event_id,
                ),
                |row| row.get(0),
            )
            .optional()
            .map_err(|_| StorageError::Database)
    }

    pub fn list_committed_sequences(
        &self,
        tenant_id: &str,
        conversation: ConversationRef<'_>,
    ) -> Result<Vec<u64>, StorageError> {
        validate_text(tenant_id)?;
        let mut statement = self
            .connection
            .prepare(
                "SELECT committed_sequence FROM opaque_events
                 WHERE tenant_id = ?1 AND conversation_kind = ?2
                   AND conversation_id = ?3
                 ORDER BY committed_sequence ASC",
            )
            .map_err(|_| StorageError::Database)?;
        let rows = statement
            .query_map(
                (tenant_id, conversation.kind_code(), conversation.id),
                |row| row.get::<_, Vec<u8>>(0),
            )
            .map_err(|_| StorageError::Database)?;

        rows.map(|row| {
            let bytes = row.map_err(|_| StorageError::Database)?;
            decode_sequence(&bytes)
        })
        .collect()
    }

    pub fn enqueue_outbox(&mut self, event: PendingOutboxEvent<'_>) -> Result<(), StorageError> {
        validate_text(event.tenant_id)?;
        validate_text(event.event_id)?;
        validate_text(event.idempotency_key)?;
        validate_bytes(event.envelope_bytes)?;

        self.connection
            .execute(
                "INSERT INTO outbox_events (
                    tenant_id, conversation_kind, conversation_id, event_id,
                    idempotency_key, envelope_bytes
                 ) VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
                (
                    event.tenant_id,
                    event.conversation.kind_code(),
                    event.conversation.id,
                    event.event_id,
                    event.idempotency_key,
                    event.envelope_bytes,
                ),
            )
            .map_err(map_write_error)?;
        Ok(())
    }

    pub fn load_outbox_event(
        &self,
        tenant_id: &str,
        conversation: ConversationRef<'_>,
        event_id: &str,
    ) -> Result<Option<Vec<u8>>, StorageError> {
        validate_text(tenant_id)?;
        validate_text(event_id)?;
        self.connection
            .query_row(
                "SELECT envelope_bytes FROM outbox_events
                 WHERE tenant_id = ?1 AND conversation_kind = ?2
                   AND conversation_id = ?3 AND event_id = ?4",
                (
                    tenant_id,
                    conversation.kind_code(),
                    conversation.id,
                    event_id,
                ),
                |row| row.get(0),
            )
            .optional()
            .map_err(|_| StorageError::Database)
    }

    pub fn record_cursor(&mut self, update: CursorUpdate<'_>) -> Result<(), StorageError> {
        validate_text(update.tenant_id)?;
        validate_text(update.device_id)?;
        validate_bytes(update.cursor_bytes)?;
        let new_sequence = update.last_applied_sequence.to_be_bytes();
        let transaction = self
            .connection
            .transaction_with_behavior(rusqlite::TransactionBehavior::Immediate)
            .map_err(|_| StorageError::Database)?;
        let existing = transaction
            .query_row(
                "SELECT last_applied_sequence, cursor_bytes FROM sync_cursors
                 WHERE tenant_id = ?1 AND device_id = ?2
                   AND conversation_kind = ?3 AND conversation_id = ?4",
                (
                    update.tenant_id,
                    update.device_id,
                    update.conversation.kind_code(),
                    update.conversation.id,
                ),
                |row| Ok((row.get::<_, Vec<u8>>(0)?, row.get::<_, Vec<u8>>(1)?)),
            )
            .optional()
            .map_err(|_| StorageError::Database)?;

        match existing {
            None if update.expected_previous_sequence.is_none() => {
                transaction
                    .execute(
                        "INSERT INTO sync_cursors (
                            tenant_id, device_id, conversation_kind, conversation_id,
                            last_applied_sequence, cursor_bytes
                         ) VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
                        (
                            update.tenant_id,
                            update.device_id,
                            update.conversation.kind_code(),
                            update.conversation.id,
                            new_sequence.as_slice(),
                            update.cursor_bytes,
                        ),
                    )
                    .map_err(map_write_error)?;
            }
            Some((current_bytes, current_cursor)) => {
                let current = decode_sequence(&current_bytes)?;
                if current == update.last_applied_sequence && current_cursor == update.cursor_bytes
                {
                    return Ok(());
                }
                if update.expected_previous_sequence != Some(current)
                    || update.last_applied_sequence <= current
                {
                    return Err(StorageError::Conflict);
                }
                transaction
                    .execute(
                        "UPDATE sync_cursors
                         SET last_applied_sequence = ?5, cursor_bytes = ?6
                         WHERE tenant_id = ?1 AND device_id = ?2
                           AND conversation_kind = ?3 AND conversation_id = ?4",
                        (
                            update.tenant_id,
                            update.device_id,
                            update.conversation.kind_code(),
                            update.conversation.id,
                            new_sequence.as_slice(),
                            update.cursor_bytes,
                        ),
                    )
                    .map_err(map_write_error)?;
            }
            None => return Err(StorageError::Conflict),
        }

        transaction.commit().map_err(|_| StorageError::Database)
    }

    pub fn load_cursor(
        &self,
        tenant_id: &str,
        device_id: &str,
        conversation: ConversationRef<'_>,
    ) -> Result<Option<StoredCursor>, StorageError> {
        validate_text(tenant_id)?;
        validate_text(device_id)?;
        self.connection
            .query_row(
                "SELECT last_applied_sequence, cursor_bytes FROM sync_cursors
                 WHERE tenant_id = ?1 AND device_id = ?2
                   AND conversation_kind = ?3 AND conversation_id = ?4",
                (
                    tenant_id,
                    device_id,
                    conversation.kind_code(),
                    conversation.id,
                ),
                |row| Ok((row.get::<_, Vec<u8>>(0)?, row.get::<_, Vec<u8>>(1)?)),
            )
            .optional()
            .map_err(|_| StorageError::Database)?
            .map(|(sequence, cursor_bytes)| {
                Ok(StoredCursor {
                    last_applied_sequence: decode_sequence(&sequence)?,
                    cursor_bytes,
                })
            })
            .transpose()
    }
}

fn validate_text(value: &str) -> Result<(), StorageError> {
    if value.is_empty() {
        Err(StorageError::InvalidInput)
    } else {
        Ok(())
    }
}

fn validate_bytes(value: &[u8]) -> Result<(), StorageError> {
    if value.is_empty() {
        Err(StorageError::InvalidInput)
    } else {
        Ok(())
    }
}

fn decode_sequence(value: &[u8]) -> Result<u64, StorageError> {
    let bytes: [u8; 8] = value.try_into().map_err(|_| StorageError::Database)?;
    Ok(u64::from_be_bytes(bytes))
}

fn map_write_error(error: rusqlite::Error) -> StorageError {
    if error.sqlite_error_code() == Some(ErrorCode::ConstraintViolation) {
        StorageError::Conflict
    } else {
        StorageError::Database
    }
}

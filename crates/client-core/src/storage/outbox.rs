//! Outbox replay, pending-to-committed merge and committed-event paging: the
//! operations `locald` needs to send in enqueue order, fold a Durable ACK into
//! exactly one committed logical item, and page history for a conversation.

use rusqlite::{OptionalExtension, TransactionBehavior};

use super::{
    records::{decode_sequence, map_write_error, validate_bytes, validate_text},
    ConversationRef, EncryptedDatabase, StorageError,
};

/// One pending outbox row in enqueue order.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct PendingOutboxRecord {
    pub event_id: String,
    pub idempotency_key: String,
    pub envelope_bytes: Vec<u8>,
    pub enqueue_sequence: u64,
}

/// One committed event with its server sequence.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct CommittedEventRecord {
    pub event_id: String,
    pub committed_sequence: u64,
    pub envelope_bytes: Vec<u8>,
}

/// The Durable ACK facts that turn one pending row into one committed event.
#[derive(Clone, Copy, Debug)]
pub struct CommitPending<'value> {
    pub tenant_id: &'value str,
    pub conversation: ConversationRef<'value>,
    pub event_id: &'value str,
    pub committed_sequence: u64,
    pub envelope_bytes: &'value [u8],
}

impl EncryptedDatabase {
    /// Lists the conversation's pending events in enqueue order so replay
    /// preserves the order the user sent them.
    pub fn list_pending_outbox(
        &self,
        tenant_id: &str,
        conversation: ConversationRef<'_>,
    ) -> Result<Vec<PendingOutboxRecord>, StorageError> {
        validate_text(tenant_id)?;
        let mut statement = self
            .connection
            .prepare(
                "SELECT event_id, idempotency_key, envelope_bytes, enqueue_sequence
                 FROM outbox_events
                 WHERE tenant_id = ?1 AND conversation_kind = ?2 AND conversation_id = ?3
                 ORDER BY enqueue_sequence ASC",
            )
            .map_err(|_| StorageError::Database)?;
        let rows = statement
            .query_map(
                (tenant_id, conversation.kind_code(), conversation.id()),
                |row| {
                    Ok(PendingOutboxRecord {
                        event_id: row.get(0)?,
                        idempotency_key: row.get(1)?,
                        envelope_bytes: row.get(2)?,
                        enqueue_sequence: row.get::<_, i64>(3)?.unsigned_abs(),
                    })
                },
            )
            .map_err(|_| StorageError::Database)?;
        rows.collect::<Result<Vec<_>, _>>()
            .map_err(|_| StorageError::Database)
    }

    /// Folds a Durable ACK into local state in one transaction: the pending row
    /// disappears and the committed event appears, or nothing changes. A repeat
    /// with the same sequence is idempotent; a different sequence for the same
    /// event is a conflict; an ACK for an event never seen locally is refused.
    pub fn commit_pending(&mut self, commit: CommitPending<'_>) -> Result<(), StorageError> {
        validate_text(commit.tenant_id)?;
        validate_text(commit.event_id)?;
        validate_bytes(commit.envelope_bytes)?;
        let transaction = self
            .connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(|_| StorageError::Database)?;
        let key = (
            commit.tenant_id,
            commit.conversation.kind_code(),
            commit.conversation.id(),
            commit.event_id,
        );
        let existing = transaction
            .query_row(
                "SELECT committed_sequence FROM opaque_events
                 WHERE tenant_id = ?1 AND conversation_kind = ?2 AND conversation_id = ?3 AND event_id = ?4",
                key,
                |row| row.get::<_, Vec<u8>>(0),
            )
            .optional()
            .map_err(|_| StorageError::Database)?;
        let removed = transaction
            .execute(
                "DELETE FROM outbox_events
                 WHERE tenant_id = ?1 AND conversation_kind = ?2 AND conversation_id = ?3 AND event_id = ?4",
                key,
            )
            .map_err(|_| StorageError::Database)?;
        match existing {
            Some(stored) if decode_sequence(&stored)? == commit.committed_sequence => {}
            Some(_) => return Err(StorageError::Conflict),
            None if removed == 0 => return Err(StorageError::Conflict),
            None => {
                let sequence = commit.committed_sequence.to_be_bytes();
                transaction
                    .execute(
                        "INSERT INTO opaque_events (
                            tenant_id, conversation_kind, conversation_id, event_id,
                            committed_sequence, envelope_bytes
                         ) VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
                        (
                            key.0,
                            key.1,
                            key.2,
                            key.3,
                            sequence.as_slice(),
                            commit.envelope_bytes,
                        ),
                    )
                    .map_err(map_write_error)?;
            }
        }
        transaction.commit().map_err(|_| StorageError::Database)
    }

    /// Pages committed events strictly after `after_sequence` in ascending
    /// order. The sequence is stored big-endian, so byte order is numeric order.
    pub fn list_committed_events(
        &self,
        tenant_id: &str,
        conversation: ConversationRef<'_>,
        after_sequence: u64,
        limit: u32,
    ) -> Result<Vec<CommittedEventRecord>, StorageError> {
        validate_text(tenant_id)?;
        if limit == 0 {
            return Err(StorageError::InvalidInput);
        }
        let after = after_sequence.to_be_bytes();
        let mut statement = self
            .connection
            .prepare(
                "SELECT event_id, committed_sequence, envelope_bytes FROM opaque_events
                 WHERE tenant_id = ?1 AND conversation_kind = ?2 AND conversation_id = ?3
                   AND committed_sequence > ?4
                 ORDER BY committed_sequence ASC
                 LIMIT ?5",
            )
            .map_err(|_| StorageError::Database)?;
        let rows = statement
            .query_map(
                (
                    tenant_id,
                    conversation.kind_code(),
                    conversation.id(),
                    after.as_slice(),
                    i64::from(limit),
                ),
                |row| {
                    Ok((
                        row.get::<_, String>(0)?,
                        row.get::<_, Vec<u8>>(1)?,
                        row.get::<_, Vec<u8>>(2)?,
                    ))
                },
            )
            .map_err(|_| StorageError::Database)?;
        rows.map(|row| {
            let (event_id, sequence, envelope_bytes) = row.map_err(|_| StorageError::Database)?;
            Ok(CommittedEventRecord {
                event_id,
                committed_sequence: decode_sequence(&sequence)?,
                envelope_bytes,
            })
        })
        .collect()
    }
}

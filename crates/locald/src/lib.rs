//! Local message service for one Device: the single writer of the encrypted
//! SQLite store that queues outgoing events durably before any network send,
//! replays them in order, folds Durable ACKs into exactly one committed item,
//! and pulls committed history by cursor without ever skipping a hole.
//!
//! The server is a trait so the whole state machine is testable without a
//! network; the Tauri host (A8) supplies the Connect-backed implementation.
//! Payload bytes are opaque; nothing here claims end-to-end encryption.

mod replay;
mod server;
mod sync;

use std::path::Path;

pub use replay::ReplayReport;
pub use server::{SendAck, SendFailure, SendRequest, Server, SyncBatch, SyncFailure, SyncedEvent};
pub use sync::PullReport;
use threadline_client_core::storage::{
    CommittedEventRecord, ConversationRef, DatabaseKey, EncryptedDatabase, PendingOutboxEvent,
    PendingOutboxRecord, StorageError,
};

/// Stable, secret-safe failure categories of the local service.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum LocaldError {
    InvalidInput,
    Storage(StorageError),
    /// The server refused the request for this Device; the pending item stays
    /// queued and is reported, never dropped silently.
    Denied,
    /// The server was unreachable or did not durably commit; retry later with
    /// the same pending identity.
    Retryable,
    /// The server's history disagrees with local state (wrong sequence for a
    /// known event, or a batch that does not start at cursor+1).
    HistoryMismatch,
}

impl From<StorageError> for LocaldError {
    fn from(error: StorageError) -> Self {
        Self::Storage(error)
    }
}

/// The authenticated Device this store belongs to. Tenant and device never
/// come from a server response.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct DeviceIdentity {
    pub tenant_id: String,
    pub device_id: String,
}

/// Owning handle on one Device's local store.
pub struct Locald {
    database: EncryptedDatabase,
    identity: DeviceIdentity,
}

impl Locald {
    /// Opens (or creates) the encrypted store for one Device.
    pub fn open(
        path: impl AsRef<Path>,
        key: DatabaseKey,
        identity: DeviceIdentity,
    ) -> Result<Self, LocaldError> {
        if identity.tenant_id.is_empty() || identity.device_id.is_empty() {
            return Err(LocaldError::InvalidInput);
        }
        let database = EncryptedDatabase::open(path, key)?;
        Ok(Self { database, identity })
    }

    pub fn identity(&self) -> &DeviceIdentity {
        &self.identity
    }

    /// Durably queues one outgoing event before any network activity. The
    /// caller supplies the client-generated event id and idempotency key that
    /// stay fixed across every retry.
    pub fn enqueue(
        &mut self,
        channel_id: &str,
        event_id: &str,
        idempotency_key: &str,
        envelope_bytes: &[u8],
    ) -> Result<(), LocaldError> {
        let conversation = ConversationRef::channel(channel_id)?;
        self.database.enqueue_outbox(PendingOutboxEvent {
            tenant_id: &self.identity.tenant_id,
            conversation,
            event_id,
            idempotency_key,
            envelope_bytes,
        })?;
        Ok(())
    }

    /// Pending events for a Channel in enqueue order, for the UI's optimistic
    /// rendering and for replay.
    pub fn pending(&self, channel_id: &str) -> Result<Vec<PendingOutboxRecord>, LocaldError> {
        let conversation = ConversationRef::channel(channel_id)?;
        Ok(self
            .database
            .list_pending_outbox(&self.identity.tenant_id, conversation)?)
    }

    /// Committed events strictly after `after_sequence`, ascending.
    pub fn committed(
        &self,
        channel_id: &str,
        after_sequence: u64,
        limit: u32,
    ) -> Result<Vec<CommittedEventRecord>, LocaldError> {
        let conversation = ConversationRef::channel(channel_id)?;
        Ok(self.database.list_committed_events(
            &self.identity.tenant_id,
            conversation,
            after_sequence,
            limit,
        )?)
    }

    /// The Device's applied cursor for a Channel; 0 before anything applied.
    pub fn cursor(&self, channel_id: &str) -> Result<u64, LocaldError> {
        let conversation = ConversationRef::channel(channel_id)?;
        Ok(self
            .database
            .load_cursor(
                &self.identity.tenant_id,
                &self.identity.device_id,
                conversation,
            )?
            .map(|cursor| cursor.last_applied_sequence)
            .unwrap_or(0))
    }
}

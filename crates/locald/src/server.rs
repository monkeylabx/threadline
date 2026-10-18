//! The network seam. The Tauri host implements it over Connect; tests use an
//! in-memory fake. Every method is a single bounded request with no retry.

/// One send command: the pending envelope bytes exactly as stored, with the
/// identifiers that stay fixed across retries.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct SendRequest {
    pub tenant_id: String,
    pub channel_id: String,
    pub event_id: String,
    pub idempotency_key: String,
    pub envelope_bytes: Vec<u8>,
}

/// A Durable ACK: the server committed (or had already committed) this event
/// at `channel_seq` and returns the envelope bytes it stored.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct SendAck {
    pub event_id: String,
    pub idempotency_key: String,
    pub channel_seq: u64,
    pub envelope_bytes: Vec<u8>,
    pub deduplicated: bool,
}

/// Why a send produced no Durable ACK.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum SendFailure {
    /// Transport or commit outcome unknown: keep the pending item and retry
    /// with the same identity.
    Retryable,
    /// The server refused this Device or Channel: keep the pending item and
    /// surface it; never drop or rewrite it.
    Denied,
    /// The key or event id is bound to different bytes on the server.
    Conflict,
}

/// One synced event from the server's committed history.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct SyncedEvent {
    pub event_id: String,
    pub channel_seq: u64,
    pub envelope_bytes: Vec<u8>,
}

/// A contiguous batch starting right after the requested cursor.
#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct SyncBatch {
    pub events: Vec<SyncedEvent>,
    pub has_more: bool,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum SyncFailure {
    Retryable,
    Denied,
}

/// Server is the only network dependency of `locald`.
pub trait Server {
    fn send(&mut self, request: SendRequest) -> Result<SendAck, SendFailure>;
    fn sync(
        &mut self,
        tenant_id: &str,
        channel_id: &str,
        after_sequence: u64,
        max_events: u32,
    ) -> Result<SyncBatch, SyncFailure>;
}

//! Serializable results of the host commands. Only display facts cross the
//! IPC boundary; credentials, keys and raw envelope bytes stay in the host.

use prost_types::Timestamp;
use serde::Serialize;
use threadline_core_client::envelope::{DecodedMessage, EnvelopeError};

#[derive(Clone, Debug, Serialize, PartialEq, Eq)]
pub struct SessionInfo {
    pub core_url: String,
    pub realtime_enabled: bool,
    pub tenant_id: String,
    pub actor_id: String,
    pub device_id: String,
}

#[derive(Clone, Debug, Serialize, PartialEq, Eq)]
pub struct ChannelDto {
    pub channel_id: String,
    pub name: String,
}

#[derive(Clone, Copy, Debug, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum MessageState {
    Pending,
    Committed,
    /// Stored bytes that this build cannot decode (foreign profile or
    /// corruption). Shown as a placeholder rather than dropped.
    Unreadable,
}

#[derive(Clone, Debug, Serialize, PartialEq, Eq)]
pub struct MessageDto {
    pub event_id: String,
    pub channel_id: String,
    pub author_actor_id: String,
    pub author_device_id: String,
    pub text: String,
    pub state: MessageState,
    pub channel_seq: Option<u64>,
    pub sent_at_unix_ms: Option<i64>,
    pub committed_at_unix_ms: Option<i64>,
}

impl MessageDto {
    pub fn from_decoded(
        channel_id: &str,
        decoded: Result<DecodedMessage, EnvelopeError>,
        event_id: &str,
        channel_seq: Option<u64>,
    ) -> Self {
        let state = if channel_seq.is_some() {
            MessageState::Committed
        } else {
            MessageState::Pending
        };
        match decoded {
            Ok(message) => Self {
                event_id: message.event_id,
                channel_id: channel_id.to_owned(),
                author_actor_id: message.sender_actor_id,
                author_device_id: message.sender_device_id,
                text: message.text,
                state,
                channel_seq,
                sent_at_unix_ms: message.client_sent_at.as_ref().map(unix_millis),
                committed_at_unix_ms: message.committed_at.as_ref().map(unix_millis),
            },
            Err(_) => Self {
                event_id: event_id.to_owned(),
                channel_id: channel_id.to_owned(),
                author_actor_id: String::new(),
                author_device_id: String::new(),
                text: String::new(),
                state: MessageState::Unreadable,
                channel_seq,
                sent_at_unix_ms: None,
                committed_at_unix_ms: None,
            },
        }
    }
}

/// What one `sync_channel` did. `online` is false only when the server was
/// unreachable or did not durably answer; a denial is reported through
/// `retained` and keeps `online` true so the banner reflects connectivity.
#[derive(Clone, Copy, Debug, Default, Serialize, PartialEq, Eq)]
pub struct SyncReportDto {
    pub committed: u32,
    pub deduplicated: u32,
    pub retained: u32,
    pub applied: u32,
    pub has_more: bool,
    pub online: bool,
    pub realtime_woke: bool,
}

/// Stable, secret-free failure codes for the renderer.
#[derive(Clone, Copy, Debug, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum HostError {
    NoSession,
    InvalidInput,
    Storage,
    Denied,
    Unavailable,
    HistoryMismatch,
}

fn unix_millis(timestamp: &Timestamp) -> i64 {
    timestamp.seconds * 1000 + i64::from(timestamp.nanos / 1_000_000)
}

use crate::acp::TransportError;
use std::fmt;

/// Engine observations only, never business Run state or cleanup proof.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum StopReason {
    EndTurn,
    MaxTokens,
    MaxTurnRequests,
    Refusal,
    Cancelled,
}

pub struct TurnResult {
    pub(super) text: String,
    pub(super) stop_reason: StopReason,
}

impl TurnResult {
    pub fn text(&self) -> &str {
        &self.text
    }
    pub fn stop_reason(&self) -> StopReason {
        self.stop_reason
    }
}

impl fmt::Debug for TurnResult {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("TurnResult")
            .field("stop_reason", &self.stop_reason)
            .finish_non_exhaustive()
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum TurnError {
    Unavailable,
    InvalidInput,
    InvalidDeadline,
    Timeout,
    Transport(TransportError),
    UnexpectedMessage,
    RemoteError,
    MalformedPayload,
    LimitExceeded,
}

impl fmt::Display for TurnError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "ACP text turn {:?}", self)
    }
}

impl std::error::Error for TurnError {}

use super::{AcpReader, AcpWriter, RpcMessage, TransportError};
use serde_json::{json, Value};
use std::{fmt, time::Duration};
use tokio::io::{AsyncRead, AsyncWrite};

const INITIALIZE_ID: i64 = 1;

/// Protocol agreement only: not authentication, Run authority or admission.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct InitializedV1(());

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum InitializeError {
    Unavailable,
    InvalidDeadline,
    Timeout,
    Transport(TransportError),
    UnexpectedMessage,
    RemoteError,
    MalformedResult,
    UnsupportedVersion,
}

impl fmt::Display for InitializeError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "ACP initialization {:?}", self)
    }
}

impl std::error::Error for InitializeError {}

/// Owns one initialization attempt. No raw transport or worker metadata escapes.
/// Dropping this owner is not process termination or cleanup evidence.
pub struct Initialization<R, W> {
    reader: AcpReader<R>,
    writer: AcpWriter<W>,
    deadline: Duration,
    attempted: bool,
}

impl<R: AsyncRead + Unpin, W: AsyncWrite + Unpin> Initialization<R, W> {
    pub fn new(
        reader: AcpReader<R>,
        writer: AcpWriter<W>,
        deadline: Duration,
    ) -> Result<Self, InitializeError> {
        if deadline.is_zero() {
            return Err(InitializeError::InvalidDeadline);
        }
        Ok(Self {
            reader,
            writer,
            deadline,
            attempted: false,
        })
    }

    /// A polled attempt consumes the connection even if its future is cancelled.
    /// This restricted profile rejects every pre-initialization callback/notice.
    pub async fn initialize(&mut self) -> Result<InitializedV1, InitializeError> {
        if self.attempted {
            return Err(InitializeError::Unavailable);
        }
        self.attempted = true;
        tokio::time::timeout(self.deadline, self.exchange())
            .await
            .unwrap_or(Err(InitializeError::Timeout))
    }

    async fn exchange(&mut self) -> Result<InitializedV1, InitializeError> {
        let request = RpcMessage::new(json!({
            "jsonrpc": "2.0", "id": INITIALIZE_ID, "method": "initialize",
            "params": {"protocolVersion": 1, "clientCapabilities": {}}
        }))
        .map_err(InitializeError::Transport)?;
        self.writer
            .send(&request)
            .await
            .map_err(InitializeError::Transport)?;
        let response = self
            .reader
            .receive()
            .await
            .map_err(InitializeError::Transport)?;
        validate_response(response.value())
    }
}

fn validate_response(response: &Value) -> Result<InitializedV1, InitializeError> {
    if response.get("method").is_some() || response.get("id") != Some(&json!(INITIALIZE_ID)) {
        return Err(InitializeError::UnexpectedMessage);
    }
    if response.get("error").is_some() {
        return Err(InitializeError::RemoteError);
    }
    let result = response
        .get("result")
        .and_then(Value::as_object)
        .ok_or(InitializeError::MalformedResult)?;
    let version = result
        .get("protocolVersion")
        .filter(|value| value.is_i64() || value.is_u64())
        .ok_or(InitializeError::MalformedResult)?;
    if version != &json!(1) {
        return Err(InitializeError::UnsupportedVersion);
    }
    // No capability flags or authentication entries are consumed or granted.
    // Omission uses ACP defaults; unknown extensions remain inert.
    if result
        .get("agentCapabilities")
        .is_some_and(|value| !value.is_object())
        || result
            .get("authMethods")
            .is_some_and(|value| !value.is_array())
    {
        return Err(InitializeError::MalformedResult);
    }
    Ok(InitializedV1(()))
}

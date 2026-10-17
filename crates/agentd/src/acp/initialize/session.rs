use super::{Initialization, Phase, RpcMessage, TransportError};
use serde_json::{json, Value};
use std::{fmt, path::Path, time::Duration};
use tokio::io::{AsyncRead, AsyncWrite};

const SESSION_REQUEST_ID: i64 = 2;

/// An internal engine reference, never Run identity, permission or admission.
#[derive(Clone, Eq, PartialEq)]
pub struct CreatedSession(String);

impl fmt::Debug for CreatedSession {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("CreatedSession").finish_non_exhaustive()
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum SessionError {
    Unavailable,
    InvalidWorkspace,
    InvalidDeadline,
    Timeout,
    Transport(TransportError),
    UnexpectedMessage,
    RemoteError,
    MalformedResult,
}

impl fmt::Display for SessionError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "ACP Session creation {:?}", self)
    }
}

impl std::error::Error for SessionError {}

impl<R: AsyncRead + Unpin, W: AsyncWrite + Unpin> Initialization<R, W> {
    /// The trusted owner supplies protocol data; this validates no file consent.
    /// A polled exchange cannot be retried, even after cancellation or success.
    pub async fn create_session(
        &mut self,
        workspace: &str,
        deadline: Duration,
    ) -> Result<CreatedSession, SessionError> {
        if self.phase != Phase::Initialized {
            return Err(SessionError::Unavailable);
        }
        let request = session_request(workspace, deadline)?;
        self.phase = Phase::Unavailable;
        let response = tokio::time::timeout(deadline, self.exchange_rpc(&request))
            .await
            .map_err(|_| SessionError::Timeout)?
            .map_err(SessionError::Transport)?;
        let session = validate_response(response.value())?;
        self.phase = Phase::Session(session.clone());
        Ok(session)
    }
}

fn session_request(workspace: &str, deadline: Duration) -> Result<RpcMessage, SessionError> {
    if deadline.is_zero() {
        return Err(SessionError::InvalidDeadline);
    }
    if workspace.len() > 4096 || workspace.contains('\0') || !Path::new(workspace).is_absolute() {
        return Err(SessionError::InvalidWorkspace);
    }
    RpcMessage::new(
        json!({"jsonrpc":"2.0","id":SESSION_REQUEST_ID,"method":"session/new",
        "params":{"cwd":workspace,"mcpServers":[]}}),
    )
    .map_err(SessionError::Transport)
}

fn validate_response(response: &Value) -> Result<CreatedSession, SessionError> {
    if response.get("method").is_some() || response.get("id") != Some(&json!(SESSION_REQUEST_ID)) {
        return Err(SessionError::UnexpectedMessage);
    }
    if response.get("error").is_some() {
        return Err(SessionError::RemoteError);
    }
    let id = response
        .get("result")
        .and_then(Value::as_object)
        .and_then(|result| result.get("sessionId"))
        .and_then(Value::as_str)
        .ok_or(SessionError::MalformedResult)?;
    if id.is_empty() || id.len() > 256 || id.chars().any(char::is_control) {
        return Err(SessionError::MalformedResult);
    }
    Ok(CreatedSession(id.to_owned()))
}

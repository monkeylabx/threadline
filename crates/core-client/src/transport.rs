//! Connect unary over HTTP/1.1: one POST per call, protobuf bodies, and the
//! Connect JSON error envelope on failure.

use std::time::Duration;

use prost::Message;
use reqwest::blocking::Client;
use reqwest::StatusCode;

use crate::Endpoint;

const REQUEST_TIMEOUT: Duration = Duration::from_secs(15);
const MAX_RESPONSE_BYTES: usize = 8 * 1024 * 1024;

/// Connect error codes the client distinguishes. Anything else is `Unknown`.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum ConnectCode {
    Canceled,
    Unknown,
    InvalidArgument,
    DeadlineExceeded,
    NotFound,
    AlreadyExists,
    PermissionDenied,
    ResourceExhausted,
    FailedPrecondition,
    Unauthenticated,
    Unavailable,
    Internal,
}

impl ConnectCode {
    fn parse(value: &str) -> Self {
        match value {
            "canceled" => Self::Canceled,
            "invalid_argument" => Self::InvalidArgument,
            "deadline_exceeded" => Self::DeadlineExceeded,
            "not_found" => Self::NotFound,
            "already_exists" => Self::AlreadyExists,
            "permission_denied" => Self::PermissionDenied,
            "resource_exhausted" => Self::ResourceExhausted,
            "failed_precondition" => Self::FailedPrecondition,
            "unauthenticated" => Self::Unauthenticated,
            "unavailable" => Self::Unavailable,
            "internal" => Self::Internal,
            _ => Self::Unknown,
        }
    }
}

/// Why a call produced no decoded response. Variants carry no request data
/// and no credential material.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum ClientError {
    /// `Endpoint` was not usable (empty token, malformed base URL).
    InvalidEndpoint,
    /// The request never completed: connect, timeout or read failure.
    Transport,
    /// The server answered with a Connect error.
    Status(ConnectCode),
    /// A 200 whose body did not decode as the expected message.
    Malformed,
}

pub(crate) struct Transport {
    http: Client,
    base_url: String,
    bearer_token: String,
}

impl Transport {
    pub(crate) fn new(endpoint: Endpoint) -> Result<Self, ClientError> {
        let base_url = endpoint.base_url.trim_end_matches('/').to_owned();
        if !(base_url.starts_with("http://") || base_url.starts_with("https://"))
            || !valid_token68(&endpoint.bearer_token)
        {
            return Err(ClientError::InvalidEndpoint);
        }
        let http = Client::builder()
            .timeout(REQUEST_TIMEOUT)
            .build()
            .map_err(|_| ClientError::InvalidEndpoint)?;
        Ok(Self {
            http,
            base_url,
            bearer_token: endpoint.bearer_token,
        })
    }

    pub(crate) fn base_url(&self) -> &str {
        &self.base_url
    }

    /// One Connect unary call. `procedure` is `/package.Service/Method`.
    pub(crate) fn unary<Request: Message, Response: Message + Default>(
        &self,
        procedure: &str,
        request: &Request,
    ) -> Result<Response, ClientError> {
        let response = self
            .http
            .post(format!("{}{procedure}", self.base_url))
            .header("content-type", "application/proto")
            .header("connect-protocol-version", "1")
            .bearer_auth(&self.bearer_token)
            .body(request.encode_to_vec())
            .send()
            .map_err(|_| ClientError::Transport)?;
        let status = response.status();
        let body = read_bounded(response)?;
        if status != StatusCode::OK {
            return Err(ClientError::Status(parse_error_code(&body)));
        }
        Response::decode(body.as_slice()).map_err(|_| ClientError::Malformed)
    }
}

fn read_bounded(response: reqwest::blocking::Response) -> Result<Vec<u8>, ClientError> {
    if response
        .content_length()
        .is_some_and(|len| len > MAX_RESPONSE_BYTES as u64)
    {
        return Err(ClientError::Malformed);
    }
    let bytes = response.bytes().map_err(|_| ClientError::Transport)?;
    if bytes.len() > MAX_RESPONSE_BYTES {
        return Err(ClientError::Malformed);
    }
    Ok(bytes.to_vec())
}

/// Connect error bodies are `{"code": "...", "message": "..."}`; an unreadable
/// body is `Unknown` so the caller still retries conservatively.
fn parse_error_code(body: &[u8]) -> ConnectCode {
    serde_json::from_slice::<serde_json::Value>(body)
        .ok()
        .and_then(|value| value.get("code")?.as_str().map(ConnectCode::parse))
        .unwrap_or(ConnectCode::Unknown)
}

/// RFC 7235 token68: the same grammar Core's bearer reader enforces.
fn valid_token68(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 4096
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || b"-._~+/=".contains(&byte))
}

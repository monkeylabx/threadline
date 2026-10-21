//! One bounded notification read; durable application remains locald cursor sync.
use std::{fmt, time::Duration};

use prost::Message;
use reqwest::{blocking::Client, redirect::Policy, Url};
use threadline_client_proto::threadline::sync::v1 as syncv1;

use crate::{transport::valid_token68, ClientError, Endpoint};

mod frame;
#[cfg(test)]
mod frame_tests;

/// Caller-owned, already-applied position. A notification never updates it.
#[derive(Clone, Debug)]
pub struct ChannelPosition {
    pub tenant_id: String,
    pub channel_id: String,
    pub after_sequence: u64,
}

/// Authenticated Realtime endpoint. No retry or persistence is hidden here.
#[derive(Clone)]
pub struct RealtimeClient {
    http: Client,
    endpoint: Endpoint,
}

impl fmt::Debug for RealtimeClient {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("RealtimeClient").finish_non_exhaustive()
    }
}

impl RealtimeClient {
    pub fn new(mut endpoint: Endpoint) -> Result<Self, ClientError> {
        let url = Url::parse(&endpoint.base_url).map_err(|_| ClientError::InvalidEndpoint)?;
        if !matches!(url.scheme(), "http" | "https")
            || url.host_str().is_none()
            || !url.username().is_empty()
            || url.password().is_some()
            || url.query().is_some()
            || url.fragment().is_some()
            || url.path() != "/"
            || !valid_token68(&endpoint.bearer_token)
        {
            return Err(ClientError::InvalidEndpoint);
        }
        endpoint.base_url = endpoint.base_url.trim_end_matches('/').to_owned();
        let http = Client::builder()
            .timeout(Duration::from_secs(15))
            .redirect(Policy::none())
            .build()
            .map_err(|_| ClientError::InvalidEndpoint)?;
        Ok(Self { http, endpoint })
    }

    /// True wakes a fresh Core sync from local storage; false is a clean end.
    /// Drops the response after one frame. Callers bound concurrency/backoff.
    pub fn wait(&self, position: &ChannelPosition) -> Result<bool, ClientError> {
        if position.tenant_id.is_empty()
            || position.channel_id.is_empty()
            || position.tenant_id.len() > 512
            || position.channel_id.len() > 512
        {
            return Err(ClientError::InvalidEndpoint);
        }
        let body = request_body(position);
        let mut response = self
            .http
            .post(format!(
                "{}/threadline.sync.v1.SyncService/StreamEvents",
                self.endpoint.base_url
            ))
            .header("content-type", "application/connect+proto")
            .header("connect-protocol-version", "1")
            .header("connect-timeout-ms", "14000")
            .bearer_auth(&self.endpoint.bearer_token)
            .body(body)
            .send()
            .map_err(|_| ClientError::Transport)?;
        if response.status() != 200
            || response
                .headers()
                .get("content-type")
                .and_then(|value| value.to_str().ok())
                != Some("application/connect+proto")
        {
            return Err(ClientError::Malformed);
        }
        frame::read(&mut response, position)
    }
}

fn request_body(position: &ChannelPosition) -> Vec<u8> {
    let request = syncv1::StreamEventsRequest {
        cursors: vec![syncv1::SyncCursor {
            last_applied_seq: position.after_sequence,
            conversation: Some(syncv1::sync_cursor::Conversation::ChannelId(
                position.channel_id.clone(),
            )),
            ..Default::default()
        }],
    };
    let bytes = request.encode_to_vec();
    let mut body = vec![0];
    body.extend_from_slice(&(bytes.len() as u32).to_be_bytes());
    body.extend(bytes);
    body
}

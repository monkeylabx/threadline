//! Strict, allocation-bounded Connect frame decoder for a single wake-up.
use std::io::Read;

use prost::Message;
use threadline_client_proto::threadline::{message::v1 as messagev1, sync::v1 as syncv1};

use super::ChannelPosition;
use crate::{transport::ConnectCode, ClientError};

const MAX_FRAME: usize = 1_048_576;

pub(super) fn read(
    reader: &mut impl Read,
    position: &ChannelPosition,
) -> Result<bool, ClientError> {
    let mut header = [0; 5];
    reader
        .read_exact(&mut header)
        .map_err(|_| ClientError::Transport)?;
    let length =
        u32::from_be_bytes(header[1..].try_into().map_err(|_| ClientError::Malformed)?) as usize;
    if length > MAX_FRAME || !matches!(header[0], 0 | 2) {
        return Err(ClientError::Malformed);
    }
    let mut body = vec![0; length];
    reader
        .read_exact(&mut body)
        .map_err(|_| ClientError::Transport)?;
    if header[0] == 2 {
        return terminal(&body);
    }
    let response = syncv1::StreamEventsResponse::decode(body.as_slice())
        .map_err(|_| ClientError::Malformed)?;
    if !response.cursor_rejections.is_empty() {
        return Err(ClientError::Status(ConnectCode::PermissionDenied));
    }
    let batch = response.batch.ok_or(ClientError::Malformed)?;
    let mut next = position.after_sequence;
    if batch.events.is_empty() {
        return Err(ClientError::Malformed);
    }
    for event in batch.events {
        next = next.checked_add(1).ok_or(ClientError::Malformed)?;
        if !valid_event(&event, position, next) {
            return Err(ClientError::Malformed);
        }
    }
    if batch.next_seq != next {
        return Err(ClientError::Malformed);
    }
    Ok(true)
}

fn terminal(body: &[u8]) -> Result<bool, ClientError> {
    let value: serde_json::Value =
        serde_json::from_slice(body).map_err(|_| ClientError::Malformed)?;
    if !value.is_object() {
        return Err(ClientError::Malformed);
    }
    match value.get("error") {
        None => Ok(false),
        Some(error) => {
            let code = error
                .get("code")
                .and_then(|v| v.as_str())
                .ok_or(ClientError::Malformed)?;
            Err(ClientError::Status(ConnectCode::parse(code)))
        }
    }
}

fn valid_event(
    event: &messagev1::ChannelEventEnvelope,
    position: &ChannelPosition,
    sequence: u64,
) -> bool {
    event.tenant_id == position.tenant_id
        && matches!(&event.conversation, Some(messagev1::channel_event_envelope::Conversation::ChannelId(id)) if id == &position.channel_id)
        && !event.event_id.is_empty()
        && event.server_commit.as_ref().map(|c| c.channel_seq) == Some(sequence)
}

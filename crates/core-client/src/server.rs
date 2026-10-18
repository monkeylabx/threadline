//! `locald::Server` over the Connect transport: maps Connect outcomes onto the
//! local state machine's retry categories so a pending item is only ever
//! committed by a Durable ACK, retained on doubt, and never dropped.

use prost::Message;
use threadline_client_proto::threadline::message::v1 as messagev1;
use threadline_client_proto::threadline::sync::v1 as syncv1;
use threadline_locald::{
    SendAck, SendFailure, SendRequest, Server, SyncBatch, SyncFailure, SyncedEvent,
};

use crate::transport::{ClientError, ConnectCode};
use crate::CoreClient;

const SEND_EVENT: &str = "/threadline.message.v1.MessageService/SendEvent";
const SYNC_EVENTS: &str = "/threadline.sync.v1.SyncService/SyncEvents";

impl Server for CoreClient {
    /// A Durable ACK is the stored envelope re-encoded with the server commit
    /// attached, so committed rows decode uniformly whether they arrived by
    /// ACK or by sync.
    fn send(&mut self, request: SendRequest) -> Result<SendAck, SendFailure> {
        let mut envelope =
            messagev1::ChannelEventEnvelope::decode(request.envelope_bytes.as_slice())
                .map_err(|_| SendFailure::Conflict)?;
        let response: messagev1::SendEventResponse = self
            .transport
            .unary(
                SEND_EVENT,
                &messagev1::SendEventRequest {
                    envelope: Some(envelope.clone()),
                },
            )
            .map_err(send_failure)?;
        let commit = response.server_commit.ok_or(SendFailure::Retryable)?;
        if response.event_id != request.event_id {
            return Err(SendFailure::Conflict);
        }
        envelope.server_commit = Some(commit.clone());
        Ok(SendAck {
            event_id: response.event_id,
            // An N-1 server may omit the echoed key; the request-local key is
            // then authoritative because the event id already matched.
            idempotency_key: if response.idempotency_key.is_empty() {
                request.idempotency_key
            } else {
                response.idempotency_key
            },
            channel_seq: commit.channel_seq,
            envelope_bytes: envelope.encode_to_vec(),
            deduplicated: response.deduplicated,
        })
    }

    fn sync(
        &mut self,
        _tenant_id: &str,
        channel_id: &str,
        after_sequence: u64,
        max_events: u32,
    ) -> Result<SyncBatch, SyncFailure> {
        let request = syncv1::SyncEventsRequest {
            cursors: vec![syncv1::SyncCursor {
                last_applied_seq: after_sequence,
                conversation: Some(syncv1::sync_cursor::Conversation::ChannelId(
                    channel_id.to_owned(),
                )),
                ..Default::default()
            }],
            max_events,
        };
        let response: syncv1::SyncEventsResponse = self
            .transport
            .unary(SYNC_EVENTS, &request)
            .map_err(sync_failure)?;
        if !response.cursor_rejections.is_empty() {
            return Err(SyncFailure::Denied);
        }
        let Some(batch) = response.batches.into_iter().next() else {
            return Ok(SyncBatch::default());
        };
        let events = batch
            .events
            .into_iter()
            .map(synced_event)
            .collect::<Result<Vec<_>, _>>()?;
        Ok(SyncBatch {
            events,
            has_more: batch.has_more,
        })
    }
}

fn synced_event(envelope: messagev1::ChannelEventEnvelope) -> Result<SyncedEvent, SyncFailure> {
    let channel_seq = envelope
        .server_commit
        .as_ref()
        .map(|commit| commit.channel_seq)
        .filter(|seq| *seq > 0)
        .ok_or(SyncFailure::Retryable)?;
    Ok(SyncedEvent {
        event_id: envelope.event_id.clone(),
        channel_seq,
        envelope_bytes: envelope.encode_to_vec(),
    })
}

fn send_failure(error: ClientError) -> SendFailure {
    match error {
        ClientError::Status(ConnectCode::AlreadyExists) => SendFailure::Conflict,
        ClientError::Status(code) if is_denial(code) => SendFailure::Denied,
        _ => SendFailure::Retryable,
    }
}

fn sync_failure(error: ClientError) -> SyncFailure {
    match error {
        ClientError::Status(code) if is_denial(code) => SyncFailure::Denied,
        _ => SyncFailure::Retryable,
    }
}

/// Codes that will not change by retrying the same bytes with the same
/// credential. Everything else keeps the pending item and retries later.
fn is_denial(code: ConnectCode) -> bool {
    matches!(
        code,
        ConnectCode::Unauthenticated
            | ConnectCode::PermissionDenied
            | ConnectCode::FailedPrecondition
            | ConnectCode::InvalidArgument
            | ConnectCode::NotFound
    )
}

//! Slice A plaintext envelope codec (ADR-0008). The server treats the body as
//! opaque bytes; this module is the only place that knows the body is a
//! deterministic `MessagePayload{message_created}` and names its profile so
//! the E2EE profile can replace it without a wire break.

use prost::Message;
use prost_types::Timestamp;
use sha2::{Digest, Sha256};
use threadline_client_proto::threadline::message::v1 as messagev1;

pub const CRYPTO_PROFILE: &str = "tl-plaintext-slice-a";
pub const ENVELOPE_VERSION: u32 = 1;
const MAX_IDENTIFIER_BYTES: usize = 128;
const MAX_TEXT_BYTES: usize = 64 * 1024;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum EnvelopeError {
    /// An identifier is empty, too long, padded or contains control characters,
    /// or the text is empty or too long.
    InvalidInput,
    /// Bytes did not decode as an envelope.
    Malformed,
    /// The envelope is not a Slice A plaintext application event.
    UnsupportedProfile,
}

/// Everything the sender knows before Core assigns a sequence.
#[derive(Clone, Copy, Debug)]
pub struct PlaintextMessage<'value> {
    pub tenant_id: &'value str,
    pub channel_id: &'value str,
    pub event_id: &'value str,
    pub idempotency_key: &'value str,
    pub sender_actor_id: &'value str,
    pub sender_device_id: &'value str,
    pub text: &'value str,
    pub client_sent_at: Option<Timestamp>,
}

/// A displayable event. `channel_seq` and `committed_at` are present only on
/// envelopes that carry a server commit.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct DecodedMessage {
    pub event_id: String,
    pub idempotency_key: String,
    pub sender_actor_id: String,
    pub sender_device_id: String,
    pub text: String,
    pub client_sent_at: Option<Timestamp>,
    pub channel_seq: Option<u64>,
    pub committed_at: Option<Timestamp>,
}

/// Encodes one message as the bytes `locald` queues and Core stores.
pub fn encode(message: PlaintextMessage<'_>) -> Result<Vec<u8>, EnvelopeError> {
    for identifier in [
        message.tenant_id,
        message.channel_id,
        message.event_id,
        message.idempotency_key,
        message.sender_actor_id,
        message.sender_device_id,
    ] {
        if !valid_identifier(identifier) {
            return Err(EnvelopeError::InvalidInput);
        }
    }
    if message.text.is_empty() || message.text.len() > MAX_TEXT_BYTES {
        return Err(EnvelopeError::InvalidInput);
    }
    let body = messagev1::MessagePayload {
        authored_at: message.client_sent_at,
        kind: Some(messagev1::message_payload::Kind::MessageCreated(
            messagev1::MessageCreated {
                body: Some(messagev1::MessageBody {
                    text: message.text.to_owned(),
                    ..Default::default()
                }),
                ..Default::default()
            },
        )),
    }
    .encode_to_vec();
    let envelope = messagev1::ChannelEventEnvelope {
        event_id: message.event_id.to_owned(),
        tenant_id: message.tenant_id.to_owned(),
        conversation: Some(messagev1::channel_event_envelope::Conversation::ChannelId(
            message.channel_id.to_owned(),
        )),
        category: messagev1::EventCategory::Application as i32,
        sender_device_id: message.sender_device_id.to_owned(),
        sender_actor_id: message.sender_actor_id.to_owned(),
        idempotency_key: message.idempotency_key.to_owned(),
        client_sent_at: message.client_sent_at,
        crypto_profile: CRYPTO_PROFILE.to_owned(),
        envelope_version: ENVELOPE_VERSION,
        content_hash: Sha256::digest(&body).to_vec().into(),
        body: Some(messagev1::channel_event_envelope::Body::ApplicationCiphertext(body.into())),
        ..Default::default()
    };
    Ok(envelope.encode_to_vec())
}

/// Decodes stored or synced envelope bytes for display. Unknown fields are
/// ignored; a different profile or category is reported, not guessed at.
pub fn decode(bytes: &[u8]) -> Result<DecodedMessage, EnvelopeError> {
    let envelope =
        messagev1::ChannelEventEnvelope::decode(bytes).map_err(|_| EnvelopeError::Malformed)?;
    if envelope.crypto_profile != CRYPTO_PROFILE
        || envelope.category != messagev1::EventCategory::Application as i32
    {
        return Err(EnvelopeError::UnsupportedProfile);
    }
    let Some(messagev1::channel_event_envelope::Body::ApplicationCiphertext(body)) = envelope.body
    else {
        return Err(EnvelopeError::UnsupportedProfile);
    };
    let payload =
        messagev1::MessagePayload::decode(body.as_ref()).map_err(|_| EnvelopeError::Malformed)?;
    let Some(messagev1::message_payload::Kind::MessageCreated(created)) = payload.kind else {
        return Err(EnvelopeError::UnsupportedProfile);
    };
    Ok(DecodedMessage {
        event_id: envelope.event_id,
        idempotency_key: envelope.idempotency_key,
        sender_actor_id: envelope.sender_actor_id,
        sender_device_id: envelope.sender_device_id,
        text: created.body.map(|body| body.text).unwrap_or_default(),
        client_sent_at: envelope.client_sent_at,
        channel_seq: envelope
            .server_commit
            .as_ref()
            .map(|commit| commit.channel_seq),
        committed_at: envelope
            .server_commit
            .and_then(|commit| commit.committed_at),
    })
}

/// Mirrors Core's identifier rule so a rejected send is caught before it is
/// queued durably.
fn valid_identifier(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= MAX_IDENTIFIER_BYTES
        && value == value.trim()
        && !value.chars().any(char::is_control)
}

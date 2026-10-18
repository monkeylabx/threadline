use prost::Message;
use prost_types::Timestamp;
use threadline_client_proto::threadline::message::v1 as messagev1;
use threadline_core_client::envelope::{
    decode, encode, EnvelopeError, PlaintextMessage, CRYPTO_PROFILE, ENVELOPE_VERSION,
};

fn sample<'value>(text: &'value str) -> PlaintextMessage<'value> {
    PlaintextMessage {
        tenant_id: "tenant-synthetic",
        channel_id: "channel-synthetic",
        event_id: "evt-synthetic",
        idempotency_key: "key-synthetic",
        sender_actor_id: "actor-synthetic",
        sender_device_id: "device-synthetic",
        text,
        client_sent_at: Some(Timestamp {
            seconds: 1_700_000_000,
            nanos: 0,
        }),
    }
}

#[test]
fn encoded_envelope_satisfies_core_send_validation_shape() {
    let bytes = encode(sample("你好，Threadline")).expect("encode");
    let envelope = messagev1::ChannelEventEnvelope::decode(bytes.as_slice()).expect("decode");
    assert!(envelope.server_commit.is_none());
    assert_eq!(
        envelope.category,
        messagev1::EventCategory::Application as i32
    );
    assert_eq!(envelope.crypto_profile, CRYPTO_PROFILE);
    assert_eq!(envelope.envelope_version, ENVELOPE_VERSION);
    assert_eq!(envelope.content_hash.len(), 32);
    assert!(envelope.agent_attribution.is_none());
    assert!(envelope.attachment_blob_ids.is_empty());
    assert!(envelope.redaction_target_event_id.is_empty());
    let Some(messagev1::channel_event_envelope::Body::ApplicationCiphertext(body)) = envelope.body
    else {
        panic!("application body expected");
    };
    assert!(!body.is_empty());
    assert_eq!(
        envelope.conversation,
        Some(messagev1::channel_event_envelope::Conversation::ChannelId(
            "channel-synthetic".to_owned()
        ))
    );
    assert_eq!(envelope.sender_actor_id, "actor-synthetic");
    assert_eq!(envelope.sender_device_id, "device-synthetic");
}

#[test]
fn encoding_is_deterministic_and_hash_binds_the_body() {
    let first = encode(sample("same")).expect("encode");
    let second = encode(sample("same")).expect("encode");
    assert_eq!(first, second);
    let hash = |bytes: &[u8]| {
        messagev1::ChannelEventEnvelope::decode(bytes)
            .expect("decode")
            .content_hash
    };
    assert_ne!(
        hash(&first),
        hash(&encode(sample("other")).expect("encode"))
    );
}

#[test]
fn decode_round_trips_and_reads_server_commit_when_present() {
    let bytes = encode(sample("round trip")).expect("encode");
    let pending = decode(&bytes).expect("decode pending");
    assert_eq!(pending.text, "round trip");
    assert_eq!(pending.event_id, "evt-synthetic");
    assert_eq!(pending.idempotency_key, "key-synthetic");
    assert_eq!(pending.sender_actor_id, "actor-synthetic");
    assert_eq!(
        pending.client_sent_at.map(|at| at.seconds),
        Some(1_700_000_000)
    );
    assert_eq!((pending.channel_seq, pending.committed_at), (None, None));

    let mut envelope = messagev1::ChannelEventEnvelope::decode(bytes.as_slice()).expect("decode");
    envelope.server_commit = Some(messagev1::ServerCommit {
        channel_seq: 9,
        committed_at: Some(Timestamp {
            seconds: 1_700_000_100,
            nanos: 0,
        }),
        ..Default::default()
    });
    let committed = decode(&envelope.encode_to_vec()).expect("decode committed");
    assert_eq!(committed.channel_seq, Some(9));
    assert_eq!(
        committed.committed_at.map(|at| at.seconds),
        Some(1_700_000_100)
    );
}

#[test]
fn encode_rejects_invalid_identifiers_and_empty_text() {
    assert_eq!(encode(sample("")), Err(EnvelopeError::InvalidInput));
    let long = "x".repeat(64 * 1024 + 1);
    assert_eq!(encode(sample(&long)), Err(EnvelopeError::InvalidInput));
    for bad in ["", " padded", "ctl\u{7}", &"i".repeat(129)] {
        let mut message = sample("ok");
        message.event_id = bad;
        assert_eq!(encode(message), Err(EnvelopeError::InvalidInput), "{bad:?}");
    }
}

#[test]
fn decode_reports_malformed_bytes_and_foreign_profiles() {
    assert_eq!(decode(&[0xff, 0xff]), Err(EnvelopeError::Malformed));
    let mut foreign =
        messagev1::ChannelEventEnvelope::decode(encode(sample("ok")).expect("encode").as_slice())
            .expect("decode");
    foreign.crypto_profile = "tl-mls-1".to_owned();
    assert_eq!(
        decode(&foreign.encode_to_vec()),
        Err(EnvelopeError::UnsupportedProfile)
    );
    let mut redaction =
        messagev1::ChannelEventEnvelope::decode(encode(sample("ok")).expect("encode").as_slice())
            .expect("decode");
    redaction.category = messagev1::EventCategory::Redaction as i32;
    assert_eq!(
        decode(&redaction.encode_to_vec()),
        Err(EnvelopeError::UnsupportedProfile)
    );
    let mut garbage_body =
        messagev1::ChannelEventEnvelope::decode(encode(sample("ok")).expect("encode").as_slice())
            .expect("decode");
    garbage_body.body = Some(
        messagev1::channel_event_envelope::Body::ApplicationCiphertext(vec![0xff, 0xff].into()),
    );
    assert_eq!(
        decode(&garbage_body.encode_to_vec()),
        Err(EnvelopeError::Malformed)
    );
}

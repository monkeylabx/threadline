mod support;

use prost::Message;
use support::{FakeCore, Scripted};
use threadline_client_proto::threadline::message::v1 as messagev1;
use threadline_client_proto::threadline::r#type::v1 as typev1;
use threadline_client_proto::threadline::sync::v1 as syncv1;
use threadline_core_client::{envelope, CoreClient, Endpoint};
use threadline_locald::{SendFailure, SendRequest, Server, SyncFailure};

const TOKEN: &str = "dev-token-synthetic-0123456789";
const TENANT: &str = "tenant-synthetic";
const CHANNEL: &str = "channel-synthetic";

fn client(base_url: &str) -> CoreClient {
    CoreClient::new(Endpoint {
        base_url: base_url.to_owned(),
        bearer_token: TOKEN.to_owned(),
    })
    .expect("client")
}

fn pending_request(event_id: &str) -> SendRequest {
    let envelope_bytes = envelope::encode(envelope::PlaintextMessage {
        tenant_id: TENANT,
        channel_id: CHANNEL,
        event_id,
        idempotency_key: &format!("key-{event_id}"),
        sender_actor_id: "actor-synthetic",
        sender_device_id: "device-synthetic",
        text: "hello",
        client_sent_at: None,
    })
    .expect("encode");
    SendRequest {
        tenant_id: TENANT.to_owned(),
        channel_id: CHANNEL.to_owned(),
        event_id: event_id.to_owned(),
        idempotency_key: format!("key-{event_id}"),
        envelope_bytes,
    }
}

fn committed(event_id: &str, channel_seq: u64) -> messagev1::ChannelEventEnvelope {
    let mut envelope = messagev1::ChannelEventEnvelope::decode(
        pending_request(event_id).envelope_bytes.as_slice(),
    )
    .expect("decode");
    envelope.server_commit = Some(messagev1::ServerCommit {
        channel_seq,
        ..Default::default()
    });
    envelope
}

#[test]
fn send_posts_connect_unary_and_returns_a_durable_ack() {
    let response = messagev1::SendEventResponse {
        server_commit: Some(messagev1::ServerCommit {
            channel_seq: 7,
            ..Default::default()
        }),
        event_id: "evt-1".to_owned(),
        deduplicated: false,
        idempotency_key: "key-evt-1".to_owned(),
    };
    let fake = FakeCore::start(vec![Scripted::proto(response.encode_to_vec())]);
    let mut client = client(&fake.base_url);

    let ack = client.send(pending_request("evt-1")).expect("ack");

    let recorded = fake.next_request();
    assert_eq!(
        recorded.path,
        "/threadline.message.v1.MessageService/SendEvent"
    );
    assert_eq!(recorded.header("content-type"), Some("application/proto"));
    assert_eq!(recorded.header("connect-protocol-version"), Some("1"));
    assert_eq!(
        recorded.header("authorization"),
        Some(&*format!("Bearer {TOKEN}"))
    );
    let sent = messagev1::SendEventRequest::decode(recorded.body.as_slice()).expect("request");
    assert_eq!(sent.envelope.expect("envelope").event_id, "evt-1");
    assert_eq!(
        (ack.event_id.as_str(), ack.channel_seq, ack.deduplicated),
        ("evt-1", 7, false)
    );
    assert_eq!(ack.idempotency_key, "key-evt-1");
    let decoded = envelope::decode(&ack.envelope_bytes).expect("decode ack");
    assert_eq!(decoded.channel_seq, Some(7));
}

#[test]
fn send_without_echoed_key_uses_the_request_key_after_matching_the_event_id() {
    let response = messagev1::SendEventResponse {
        server_commit: Some(messagev1::ServerCommit {
            channel_seq: 1,
            ..Default::default()
        }),
        event_id: "evt-1".to_owned(),
        deduplicated: true,
        idempotency_key: String::new(),
    };
    let fake = FakeCore::start(vec![Scripted::proto(response.encode_to_vec())]);
    let ack = client(&fake.base_url)
        .send(pending_request("evt-1"))
        .expect("ack");
    assert_eq!(ack.idempotency_key, "key-evt-1");
    assert!(ack.deduplicated);
}

#[test]
fn send_maps_connect_codes_onto_locald_retry_categories() {
    let cases = [
        (401, "unauthenticated", SendFailure::Denied),
        (403, "permission_denied", SendFailure::Denied),
        (412, "failed_precondition", SendFailure::Denied),
        (400, "invalid_argument", SendFailure::Denied),
        (409, "already_exists", SendFailure::Conflict),
        (503, "unavailable", SendFailure::Retryable),
        (504, "deadline_exceeded", SendFailure::Retryable),
        (500, "not-a-connect-code", SendFailure::Retryable),
    ];
    let fake = FakeCore::start(
        cases
            .iter()
            .map(|(status, code, _)| Scripted::error(*status, code))
            .collect(),
    );
    let mut client = client(&fake.base_url);
    for (_, code, expected) in cases {
        assert_eq!(
            client.send(pending_request("evt-1")),
            Err(expected),
            "{code}"
        );
    }
}

#[test]
fn send_without_server_commit_or_with_foreign_event_id_never_commits() {
    let no_commit = messagev1::SendEventResponse {
        event_id: "evt-1".to_owned(),
        ..Default::default()
    };
    let other_event = messagev1::SendEventResponse {
        server_commit: Some(messagev1::ServerCommit::default()),
        event_id: "evt-other".to_owned(),
        ..Default::default()
    };
    let fake = FakeCore::start(vec![
        Scripted::proto(no_commit.encode_to_vec()),
        Scripted::proto(other_event.encode_to_vec()),
    ]);
    let mut client = client(&fake.base_url);
    assert_eq!(
        client.send(pending_request("evt-1")),
        Err(SendFailure::Retryable)
    );
    assert_eq!(
        client.send(pending_request("evt-1")),
        Err(SendFailure::Conflict)
    );
}

#[test]
fn unreachable_server_is_retryable() {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("bind");
    let base_url = format!("http://{}", listener.local_addr().expect("addr"));
    drop(listener);
    let mut client = client(&base_url);
    assert_eq!(
        client.send(pending_request("evt-1")),
        Err(SendFailure::Retryable)
    );
    assert_eq!(
        client.sync(TENANT, CHANNEL, 0, 10),
        Err(SyncFailure::Retryable)
    );
}

#[test]
fn sync_sends_one_channel_cursor_and_returns_the_batch_with_commits_attached() {
    let response = syncv1::SyncEventsResponse {
        batches: vec![syncv1::EventBatch {
            events: vec![committed("evt-1", 3), committed("evt-2", 4)],
            next_seq: 4,
            has_more: true,
            ..Default::default()
        }],
        ..Default::default()
    };
    let fake = FakeCore::start(vec![Scripted::proto(response.encode_to_vec())]);
    let batch = client(&fake.base_url)
        .sync(TENANT, CHANNEL, 2, 50)
        .expect("batch");

    let recorded = fake.next_request();
    assert_eq!(recorded.path, "/threadline.sync.v1.SyncService/SyncEvents");
    let request = syncv1::SyncEventsRequest::decode(recorded.body.as_slice()).expect("request");
    assert_eq!(request.max_events, 50);
    assert_eq!(request.cursors.len(), 1);
    assert_eq!(request.cursors[0].last_applied_seq, 2);
    assert_eq!(
        request.cursors[0].conversation,
        Some(syncv1::sync_cursor::Conversation::ChannelId(
            CHANNEL.to_owned()
        ))
    );
    assert!(batch.has_more);
    let sequences: Vec<(&str, u64)> = batch
        .events
        .iter()
        .map(|event| (event.event_id.as_str(), event.channel_seq))
        .collect();
    assert_eq!(sequences, [("evt-1", 3), ("evt-2", 4)]);
    assert_eq!(
        envelope::decode(&batch.events[1].envelope_bytes)
            .expect("decode")
            .channel_seq,
        Some(4)
    );
}

#[test]
fn sync_treats_empty_response_as_caught_up_and_rejection_as_denied() {
    let empty = syncv1::SyncEventsResponse::default();
    let rejected = syncv1::SyncEventsResponse {
        cursor_rejections: vec![syncv1::CursorRejection {
            error: Some(typev1::ErrorDetail {
                reason: "read_unavailable".to_owned(),
                ..Default::default()
            }),
            ..Default::default()
        }],
        ..Default::default()
    };
    let missing_commit = syncv1::SyncEventsResponse {
        batches: vec![syncv1::EventBatch {
            events: vec![messagev1::ChannelEventEnvelope::default()],
            ..Default::default()
        }],
        ..Default::default()
    };
    let fake = FakeCore::start(vec![
        Scripted::proto(empty.encode_to_vec()),
        Scripted::proto(rejected.encode_to_vec()),
        Scripted::proto(missing_commit.encode_to_vec()),
        Scripted::error(401, "unauthenticated"),
        Scripted::error(503, "unavailable"),
    ]);
    let mut client = client(&fake.base_url);
    let batch = client.sync(TENANT, CHANNEL, 0, 10).expect("empty");
    assert!(batch.events.is_empty() && !batch.has_more);
    assert_eq!(
        client.sync(TENANT, CHANNEL, 0, 10),
        Err(SyncFailure::Denied)
    );
    assert_eq!(
        client.sync(TENANT, CHANNEL, 0, 10),
        Err(SyncFailure::Retryable)
    );
    assert_eq!(
        client.sync(TENANT, CHANNEL, 0, 10),
        Err(SyncFailure::Denied)
    );
    assert_eq!(
        client.sync(TENANT, CHANNEL, 0, 10),
        Err(SyncFailure::Retryable)
    );
}

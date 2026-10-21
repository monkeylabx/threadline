use super::{frame, ChannelPosition};
use crate::{ClientError, ConnectCode};
use prost::Message;
use threadline_client_proto::threadline::{message::v1 as messagev1, sync::v1 as syncv1};

fn position() -> ChannelPosition {
    ChannelPosition {
        tenant_id: "tenant".into(),
        channel_id: "channel".into(),
        after_sequence: 4,
    }
}

fn response() -> syncv1::StreamEventsResponse {
    syncv1::StreamEventsResponse {
        batch: Some(syncv1::EventBatch {
            events: vec![messagev1::ChannelEventEnvelope {
                tenant_id: "tenant".into(),
                conversation: Some(messagev1::channel_event_envelope::Conversation::ChannelId(
                    "channel".into(),
                )),
                event_id: "event".into(),
                server_commit: Some(messagev1::ServerCommit {
                    channel_seq: 5,
                    ..Default::default()
                }),
                ..Default::default()
            }],
            next_seq: 5,
            ..Default::default()
        }),
        ..Default::default()
    }
}

fn framed(flags: u8, body: &[u8]) -> Vec<u8> {
    let mut bytes = vec![flags];
    bytes.extend_from_slice(&(body.len() as u32).to_be_bytes());
    bytes.extend_from_slice(body);
    bytes
}

fn decode(value: &syncv1::StreamEventsResponse) -> Result<bool, ClientError> {
    frame::read(
        &mut framed(0, &value.encode_to_vec()).as_slice(),
        &position(),
    )
}

#[test]
fn wakes_only_for_valid_contiguous_scoped_batch_without_advancing_input() {
    let cursor = position();
    assert_eq!(decode(&response()), Ok(true));
    assert_eq!(cursor.after_sequence, 4);
    let mut value = response();
    value.batch.as_mut().unwrap().events[0].body = Some(
        messagev1::channel_event_envelope::Body::ApplicationCiphertext(vec![0xff; 262_144].into()),
    );
    assert_eq!(decode(&value), Ok(true));
}

#[test]
fn rejects_foreign_identity_missing_commit_empty_id_gap_and_wrong_next() {
    for case in 0..7 {
        let mut value = response();
        let batch = value.batch.as_mut().unwrap();
        let event = &mut batch.events[0];
        match case {
            0 => event.tenant_id = "foreign".into(),
            1 => {
                event.conversation = Some(
                    messagev1::channel_event_envelope::Conversation::ChannelId("foreign".into()),
                )
            }
            2 => event.event_id.clear(),
            3 => event.server_commit = None,
            4 => event.server_commit.as_mut().unwrap().channel_seq = 6,
            5 => batch.next_seq = 6,
            _ => batch.events.clear(),
        }
        assert_eq!(decode(&value), Err(ClientError::Malformed));
    }
    assert_eq!(
        decode(&syncv1::StreamEventsResponse::default()),
        Err(ClientError::Malformed)
    );
}

#[test]
fn rejects_rejected_cursor_and_sequence_overflow() {
    let mut value = response();
    value
        .cursor_rejections
        .push(syncv1::CursorRejection::default());
    assert_eq!(
        decode(&value),
        Err(ClientError::Status(ConnectCode::PermissionDenied))
    );
    let mut cursor = position();
    cursor.after_sequence = u64::MAX;
    assert_eq!(
        frame::read(
            &mut framed(0, &response().encode_to_vec()).as_slice(),
            &cursor
        ),
        Err(ClientError::Malformed)
    );
}

#[test]
fn bounds_allocations_and_rejects_compression_reserved_flags_and_truncation() {
    for bytes in [vec![0, 0], vec![0, 0, 0, 0, 3, 1]] {
        assert_eq!(
            frame::read(&mut bytes.as_slice(), &position()),
            Err(ClientError::Transport)
        );
    }
    for flags in [1, 3, 4, 128] {
        assert_eq!(
            frame::read(&mut framed(flags, b"").as_slice(), &position()),
            Err(ClientError::Malformed)
        );
    }
    assert_eq!(
        frame::read(&mut [0, 0, 16, 0, 1].as_slice(), &position()),
        Err(ClientError::Malformed)
    );
    assert_eq!(
        frame::read(&mut framed(0, &[0xff]).as_slice(), &position()),
        Err(ClientError::Malformed)
    );
}

#[test]
fn terminal_success_denial_unknown_and_malformed_are_secret_safe() {
    for (body, expected) in [
        ("{}", Ok(false)),
        (
            r#"{"error":{"code":"permission_denied","message":"private"}}"#,
            Err(ClientError::Status(ConnectCode::PermissionDenied)),
        ),
        (
            r#"{"error":{"code":"unavailable"}}"#,
            Err(ClientError::Status(ConnectCode::Unavailable)),
        ),
        (
            r#"{"error":{"code":"future"}}"#,
            Err(ClientError::Status(ConnectCode::Unknown)),
        ),
        ("[]", Err(ClientError::Malformed)),
        ("invalid", Err(ClientError::Malformed)),
        (r#"{"error":{}}"#, Err(ClientError::Malformed)),
    ] {
        let result = frame::read(&mut framed(2, body.as_bytes()).as_slice(), &position());
        assert_eq!(result, expected);
        assert!(!format!("{result:?}").contains("private"));
    }
}

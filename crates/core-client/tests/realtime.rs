mod support;
use prost::Message;
use support::{FakeCore, Scripted};
use threadline_client_proto::threadline::sync::v1 as syncv1;
use threadline_core_client::{
    realtime::{ChannelPosition, RealtimeClient},
    ClientError, Endpoint,
};

#[test]
fn request_uses_applied_cursor_and_host_credential_and_a_bounded_stream_frame() {
    let fake = FakeCore::start(vec![Scripted {
        status: 200,
        content_type: "application/connect+proto",
        body: vec![2, 0, 0, 0, 2, b'{', b'}'],
    }]);
    let client = RealtimeClient::new(Endpoint {
        base_url: fake.base_url.clone(),
        bearer_token: "synthetic-token".into(),
    })
    .unwrap();
    assert!(!format!("{client:?}").contains("synthetic-token"));
    let cursor = ChannelPosition {
        tenant_id: "tenant".into(),
        channel_id: "channel".into(),
        after_sequence: 17,
    };
    assert_eq!(client.wait(&cursor), Ok(false));
    let received = fake.next_request();
    assert_eq!(
        received.path,
        "/threadline.sync.v1.SyncService/StreamEvents"
    );
    assert_eq!(
        received.header("authorization"),
        Some("Bearer synthetic-token")
    );
    assert_eq!(
        received.header("content-type"),
        Some("application/connect+proto")
    );
    assert_eq!(received.header("connect-timeout-ms"), Some("14000"));
    let request = syncv1::StreamEventsRequest::decode(&received.body[5..]).unwrap();
    assert_eq!(received.body[0], 0);
    assert_eq!(request.cursors[0].last_applied_seq, 17);
    assert_eq!(
        request.cursors[0].conversation,
        Some(syncv1::sync_cursor::Conversation::ChannelId(
            "channel".into()
        ))
    );
    assert_eq!(cursor.after_sequence, 17);
}

#[test]
fn rejects_unsafe_endpoint_or_credential_and_non_connect_response() {
    for base in [
        "ftp://host",
        "http://user:secret@host",
        "http://host/path",
        "http://host/?q=secret",
        "http://host/#fragment",
        "broken",
    ] {
        assert!(matches!(
            RealtimeClient::new(Endpoint {
                base_url: base.into(),
                bearer_token: "token".into()
            }),
            Err(ClientError::InvalidEndpoint)
        ));
    }
    assert!(matches!(
        RealtimeClient::new(Endpoint {
            base_url: "http://127.0.0.1:1".into(),
            bearer_token: "bad token".into()
        }),
        Err(ClientError::InvalidEndpoint)
    ));
    let fake = FakeCore::start(vec![
        Scripted::proto(vec![]),
        Scripted::error(401, "unauthenticated"),
    ]);
    let client = RealtimeClient::new(Endpoint {
        base_url: fake.base_url.clone(),
        bearer_token: "token".into(),
    })
    .unwrap();
    let cursor = ChannelPosition {
        tenant_id: "tenant".into(),
        channel_id: "channel".into(),
        after_sequence: 0,
    };
    assert_eq!(client.wait(&cursor), Err(ClientError::Malformed));
    fake.next_request();
    assert_eq!(client.wait(&cursor), Err(ClientError::Malformed));
    fake.next_request();
}

#[test]
fn redirect_is_rejected_without_following_the_location() {
    use std::{
        io::{Read, Write},
        net::TcpListener,
        thread,
    };
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let base_url = format!("http://{}", listener.local_addr().unwrap());
    let server = thread::spawn(move || {
        let (mut socket, _) = listener.accept().unwrap();
        socket
            .set_read_timeout(Some(std::time::Duration::from_secs(2)))
            .unwrap();
        let mut buffer = [0; 4096];
        let received = socket.read(&mut buffer).unwrap();
        assert!(received > 0);
        socket.write_all(b"HTTP/1.1 302 Found\r\nLocation: http://127.0.0.1:1/credential-trap\r\nContent-Length: 0\r\n\r\n").unwrap();
    });
    let client = RealtimeClient::new(Endpoint {
        base_url,
        bearer_token: "synthetic-token".into(),
    })
    .unwrap();
    let mut cursor = ChannelPosition {
        tenant_id: "tenant".into(),
        channel_id: "channel".into(),
        after_sequence: 0,
    };
    assert_eq!(client.wait(&cursor), Err(ClientError::Malformed));
    server.join().unwrap();
    cursor.channel_id.clear();
    assert_eq!(client.wait(&cursor), Err(ClientError::InvalidEndpoint));
}

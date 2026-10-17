mod acp_support;

use acp_support::{run, Broken};
use serde_json::{json, Value};
use std::{
    future::{poll_fn, Future},
    task::Poll,
    time::Duration,
};
use threadline_agentd::acp::{stdio, Initialization, InitializeError as Error, TransportError};
use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt, BufReader};

const IO_DEADLINE: Duration = Duration::from_secs(60);
const HANDSHAKE_DEADLINE: Duration = Duration::from_secs(10);

fn assert_wire(bytes: &[u8]) {
    assert_eq!(bytes.iter().filter(|byte| **byte == b'\n').count(), 1);
    assert_eq!(
        serde_json::from_slice::<Value>(bytes).expect("one request"),
        json!({"jsonrpc":"2.0","id":1,"method":"initialize",
            "params":{"protocolVersion":1,"clientCapabilities":{}}})
    );
}

async fn assert_reply(reply: Value, expected: Option<Error>) {
    let bytes = format!("{reply}\n");
    let (writer, mut peer) = tokio::io::duplex(1024);
    let (reader, writer) = stdio(bytes.as_bytes(), writer, IO_DEADLINE).expect("pipes");
    let mut init = Initialization::new(reader, writer, HANDSHAKE_DEADLINE).expect("owner");
    let result = init.initialize().await;
    assert_eq!(result.as_ref().err().copied(), expected, "{reply}");
    if let Ok(receipt) = result {
        assert_eq!(format!("{receipt:?}"), "InitializedV1(())");
    }
    assert_eq!(init.initialize().await, Err(Error::Unavailable));
    drop(init);
    let mut output = Vec::new();
    peer.read_to_end(&mut output).await.expect("outbound EOF");
    assert_wire(&output);
}

#[test]
fn accepts_v1_defaults_without_exposing_or_granting_worker_metadata() {
    run(async {
        for result in [
            json!({"protocolVersion":1}),
            json!({"protocolVersion":1,"agentCapabilities":{},"authMethods":[]}),
            json!({"protocolVersion":1,"agentCapabilities":{"loadSession":true,
                "futureCapability":{"secret":"worker-only"}},
                "authMethods":[{"id":"private-auth","name":"worker-only"}],
                "agentInfo":{"name":"worker-only","version":"secret"},
                "_meta":{"secret":"worker-only"},"futureExtension":true}),
        ] {
            assert_reply(json!({"jsonrpc":"2.0","id":1,"result":result}), None).await;
        }
    });
}

#[test]
fn rejects_malformed_results_and_unsupported_versions() {
    run(async {
        let malformed = [
            Value::Null,
            json!([]),
            json!(1),
            json!({}),
            json!({"protocolVersion":null}),
            json!({"protocolVersion":"1"}),
            json!({"protocolVersion":true}),
            json!({"protocolVersion":1.0}),
            json!({"protocolVersion":1,"agentCapabilities":null}),
            json!({"protocolVersion":1,"agentCapabilities":[]}),
            json!({"protocolVersion":1,"authMethods":null}),
            json!({"protocolVersion":1,"authMethods":{}}),
        ];
        for result in malformed {
            assert_reply(
                json!({"jsonrpc":"2.0","id":1,"result":result}),
                Some(Error::MalformedResult),
            )
            .await;
        }
        for version in [json!(0), json!(2), json!(-1), json!(u64::MAX)] {
            assert_reply(
                json!({"jsonrpc":"2.0","id":1,"result":{"protocolVersion":version}}),
                Some(Error::UnsupportedVersion),
            )
            .await;
        }
    });
}

#[test]
fn rejects_wrong_ids_errors_and_unsolicited_messages_without_callbacks() {
    run(async {
        for id in [json!(2), json!("1"), json!(-1)] {
            assert_reply(
                json!({"jsonrpc":"2.0","id":id,"result":{"protocolVersion":1}}),
                Some(Error::UnexpectedMessage),
            )
            .await;
        }
        for reply in [
            json!({"jsonrpc":"2.0","id":null,"error":{"code":-1,"message":"worker-only"}}),
            json!({"jsonrpc":"2.0","id":1,"method":"fs/read_text_file","params":{"path":"secret"}}),
            json!({"jsonrpc":"2.0","method":"session/update","params":{}}),
        ] {
            assert_reply(reply, Some(Error::UnexpectedMessage)).await;
        }
        assert_reply(
            json!({"jsonrpc":"2.0","id":1,"error":{"code":-1,
            "message":"worker-only","data":{"token":"secret"}}}),
            Some(Error::RemoteError),
        )
        .await;
        assert_reply(
            json!({"jsonrpc":"2.0","id":null,"result":{"protocolVersion":1}}),
            Some(Error::Transport(TransportError::Malformed)),
        )
        .await;
        assert_eq!(
            format!("{}", Error::RemoteError),
            "ACP initialization RemoteError"
        );
        assert_eq!(format!("{:?}", Error::RemoteError), "RemoteError");
    });
}

#[test]
fn refuses_zero_deadline_and_transport_failures_without_retry() {
    run(async {
        let (reader, writer) = stdio(Broken, Broken, IO_DEADLINE).expect("pipes");
        assert!(matches!(
            Initialization::new(reader, writer, Duration::ZERO),
            Err(Error::InvalidDeadline)
        ));
        let (reader, writer) = stdio(Broken, tokio::io::sink(), IO_DEADLINE).expect("pipes");
        let mut init = Initialization::new(reader, writer, HANDSHAKE_DEADLINE).expect("owner");
        assert_eq!(
            init.initialize().await,
            Err(Error::Transport(TransportError::Io))
        );
        assert_eq!(init.initialize().await, Err(Error::Unavailable));
        let (reader, writer) = stdio(tokio::io::empty(), Broken, IO_DEADLINE).expect("pipes");
        let mut init = Initialization::new(reader, writer, HANDSHAKE_DEADLINE).expect("owner");
        assert_eq!(
            init.initialize().await,
            Err(Error::Transport(TransportError::Io))
        );
        assert_eq!(init.initialize().await, Err(Error::Unavailable));
        let (reader, writer) =
            stdio(tokio::io::empty(), tokio::io::sink(), IO_DEADLINE).expect("pipes");
        let mut init = Initialization::new(reader, writer, HANDSHAKE_DEADLINE).expect("owner");
        assert_eq!(
            init.initialize().await,
            Err(Error::Transport(TransportError::Closed))
        );
        assert_eq!(init.initialize().await, Err(Error::Unavailable));
    });
}

#[test]
fn cancelled_partial_send_or_pending_receive_cannot_reuse_io() {
    run(async {
        for capacity in [1, 1024] {
            let (reader, _input_peer) = tokio::io::duplex(1024);
            let (writer, mut output_peer) = tokio::io::duplex(capacity);
            let (reader, writer) = stdio(reader, writer, IO_DEADLINE).expect("pipes");
            let mut init = Initialization::new(reader, writer, HANDSHAKE_DEADLINE).expect("owner");
            let mut attempt = Box::pin(init.initialize());
            assert!(poll_fn(|cx| Poll::Ready(attempt.as_mut().poll(cx).is_pending())).await);
            drop(attempt);
            assert_eq!(init.initialize().await, Err(Error::Unavailable));
            drop(init);
            let mut output = Vec::new();
            output_peer
                .read_to_end(&mut output)
                .await
                .expect("outbound EOF");
            if capacity == 1 {
                assert_eq!(output, b"{");
            } else {
                assert_wire(&output);
            }
        }
    });
}

#[test]
fn one_total_budget_includes_delayed_send_and_receive() {
    run(async {
        let (stream, peer) = tokio::io::duplex(1);
        let (reader, writer) = tokio::io::split(stream);
        let (reader, writer) = stdio(reader, writer, IO_DEADLINE).expect("pipes");
        let mut init = Initialization::new(reader, writer, HANDSHAKE_DEADLINE).expect("owner");
        let server = tokio::spawn(async move {
            tokio::time::sleep(Duration::from_secs(6)).await;
            let mut peer = BufReader::new(peer);
            let mut request = String::new();
            peer.read_line(&mut request).await.expect("request");
            assert_wire(request.as_bytes());
            tokio::time::sleep(Duration::from_secs(6)).await;
            peer.get_mut()
                .write_all(b"{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":1}}\n")
                .await
                .expect("late response");
        });
        let start = tokio::time::Instant::now();
        assert_eq!(init.initialize().await, Err(Error::Timeout));
        assert_eq!(start.elapsed(), HANDSHAKE_DEADLINE);
        assert_eq!(init.initialize().await, Err(Error::Unavailable));
        server.abort();
        assert!(server.await.expect_err("cancel fixture").is_cancelled());
    });
}

#[test]
fn a_stalled_send_exhausts_the_total_budget() {
    run(async {
        let (writer, _peer) = tokio::io::duplex(1);
        let (reader, writer) = stdio(tokio::io::empty(), writer, IO_DEADLINE).expect("pipes");
        let mut init = Initialization::new(reader, writer, HANDSHAKE_DEADLINE).expect("owner");
        let start = tokio::time::Instant::now();
        assert_eq!(init.initialize().await, Err(Error::Timeout));
        assert_eq!(start.elapsed(), HANDSHAKE_DEADLINE);
        assert_eq!(init.initialize().await, Err(Error::Unavailable));
    });
}

mod acp_support;
#[path = "acp_session/fixtures.rs"]
mod fixtures;

use acp_support::{run, Broken};
use fixtures::{assert_wire, live, owner, reply, workspace, DEADLINE, INIT_REPLY, IO_DEADLINE};
use serde_json::{json, Value};
use std::{
    future::{poll_fn, Future},
    task::Poll,
    time::Duration,
};
use threadline_agentd::acp::{stdio, Initialization, SessionError as Error, TransportError};
use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt};

#[test]
fn preserves_bounded_opaque_ids_and_discards_optional_metadata() {
    run(async {
        for id in [
            "session".to_owned(),
            " sess-é ".to_owned(),
            "x".repeat(256),
            "é".repeat(128),
        ] {
            let response = json!({"jsonrpc":"2.0","id":2,"result":{"sessionId":id,
                "configOptions":[{"secret":"worker-only"}],"_meta":{"token":"secret"}}});
            let receipt = reply(response, None).await.expect("created");
            assert_eq!(format!("{receipt:?}"), "CreatedSession { .. }");
            let same = reply(
                json!({"jsonrpc":"2.0","id":2,"result":{"sessionId":id}}),
                None,
            )
            .await
            .expect("same opaque ID");
            assert_eq!(receipt, same);
        }
        let spaced = reply(
            json!({"jsonrpc":"2.0","id":2,"result":{"sessionId":" id "}}),
            None,
        )
        .await
        .expect("spaced ID");
        let trimmed = reply(
            json!({"jsonrpc":"2.0","id":2,"result":{"sessionId":"id"}}),
            None,
        )
        .await
        .expect("different ID");
        assert_ne!(spaced, trimmed);
    });
}

#[test]
fn rejects_malformed_and_out_of_bounds_session_ids() {
    run(async {
        for result in [
            Value::Null,
            json!([]),
            json!("wrong"),
            json!({}),
            json!({"sessionId":null}),
            json!({"sessionId":2}),
            json!({"sessionId":true}),
            json!({"sessionId":{}}),
            json!({"sessionId":""}),
            json!({"sessionId":"x".repeat(257)}),
            json!({"sessionId":"é".repeat(129)}),
            json!({"sessionId":"line\nfeed"}),
            json!({"sessionId":"nul\0"}),
            json!({"sessionId":"\u{0085}"}),
        ] {
            reply(
                json!({"jsonrpc":"2.0","id":2,"result":result}),
                Some(Error::MalformedResult),
            )
            .await
            .expect_err("invalid ID/result");
        }
    });
}

#[test]
fn correlates_session_id_and_refuses_errors_and_callbacks() {
    run(async {
        for id in [json!(1), json!(3), json!("2"), json!(-1)] {
            reply(
                json!({"jsonrpc":"2.0","id":id,"result":{"sessionId":"private-id"}}),
                Some(Error::UnexpectedMessage),
            )
            .await
            .expect_err("wrong ID");
        }
        for response in [
            json!({"jsonrpc":"2.0","id":null,"error":{"code":-1,"message":"secret"}}),
            json!({"jsonrpc":"2.0","id":2,"method":"terminal/create","params":{"cwd":"secret"}}),
            json!({"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"secret"}}),
        ] {
            reply(response, Some(Error::UnexpectedMessage))
                .await
                .expect_err("unsolicited");
        }
        reply(
            json!({"jsonrpc":"2.0","id":2,"error":{"code":-1,"message":"private-id",
            "data":{"path":"secret"}}}),
            Some(Error::RemoteError),
        )
        .await
        .expect_err("remote error");
        reply(
            json!({"jsonrpc":"2.0","id":null,"result":{"sessionId":"private-id"}}),
            Some(Error::Transport(TransportError::Malformed)),
        )
        .await
        .expect_err("null ID");
        assert_eq!(
            format!("{}", Error::RemoteError),
            "ACP Session creation RemoteError"
        );
        assert_eq!(format!("{:?}", Error::RemoteError), "RemoteError");
    });
}

#[test]
fn refuses_invalid_local_arguments_without_consuming_the_initialized_owner() {
    run(async {
        let input = format!(
            "{INIT_REPLY}{{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{{\"sessionId\":\"ok\"}}}}\n"
        );
        let (mut client, mut peer) = owner(input.as_bytes());
        assert_eq!(
            client.create_session(workspace(), DEADLINE).await,
            Err(Error::Unavailable)
        );
        client.initialize().await.expect("initialized");
        for invalid in [
            "".to_owned(),
            ".".to_owned(),
            "relative".to_owned(),
            format!("{}\0", workspace()),
            format!("{}{}", workspace(), "x".repeat(4096)),
            format!("{}{}", workspace(), "é".repeat(2048)),
        ] {
            assert_eq!(
                client.create_session(&invalid, DEADLINE).await,
                Err(Error::InvalidWorkspace)
            );
        }
        assert_eq!(
            client.create_session(workspace(), Duration::ZERO).await,
            Err(Error::InvalidDeadline)
        );
        let mut boundary = workspace().to_owned();
        let remaining = 4096 - boundary.len();
        boundary.push_str(&"é".repeat(remaining / 2));
        boundary.push_str(&"x".repeat(remaining % 2));
        assert_eq!(boundary.len(), 4096);
        assert_eq!(
            client
                .create_session(&format!("{boundary}x"), DEADLINE)
                .await,
            Err(Error::InvalidWorkspace)
        );
        client
            .create_session(&boundary, DEADLINE)
            .await
            .expect("exact byte boundary");
        drop(client);
        let mut wire = Vec::new();
        peer.read_to_end(&mut wire).await.expect("outbound EOF");
        assert_wire(&wire, &boundary);
    });
}

#[test]
fn failed_initialization_never_sends_a_session_request() {
    run(async {
        let (mut client, mut peer) =
            owner(b"{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":2}}\n");
        client.initialize().await.expect_err("unsupported version");
        assert_eq!(
            client.create_session(workspace(), DEADLINE).await,
            Err(Error::Unavailable)
        );
        drop(client);
        let mut wire = Vec::new();
        peer.read_to_end(&mut wire).await.expect("outbound EOF");
        let request: Value = serde_json::from_slice(&wire).expect("only initialize");
        assert_eq!(request["method"], "initialize");
        assert_eq!(wire.iter().filter(|b| **b == b'\n').count(), 1);
    });
}

#[test]
fn transport_failures_are_terminal_during_session_creation() {
    run(async {
        for (suffix, expected) in [
            ("", TransportError::Closed),
            ("{", TransportError::Truncated),
            ("not-json\n", TransportError::Malformed),
        ] {
            let input = format!("{INIT_REPLY}{suffix}");
            let (mut client, mut peer) = owner(input.as_bytes());
            client.initialize().await.expect("initialized");
            assert_eq!(
                client.create_session(workspace(), DEADLINE).await,
                Err(Error::Transport(expected))
            );
            assert_eq!(
                client.create_session(workspace(), DEADLINE).await,
                Err(Error::Unavailable)
            );
            drop(client);
            let mut wire = Vec::new();
            peer.read_to_end(&mut wire).await.expect("outbound EOF");
            assert_wire(&wire, workspace());
        }
        let reader = INIT_REPLY.as_bytes().chain(Broken);
        let (reader, writer) = stdio(reader, tokio::io::sink(), IO_DEADLINE).expect("pipes");
        let mut client = Initialization::new(reader, writer, DEADLINE).expect("owner");
        client.initialize().await.expect("initialized");
        assert_eq!(
            client.create_session(workspace(), DEADLINE).await,
            Err(Error::Transport(TransportError::Io))
        );
        assert_eq!(
            client.create_session(workspace(), DEADLINE).await,
            Err(Error::Unavailable)
        );
        let (mut client, peer) = owner(INIT_REPLY.as_bytes());
        client.initialize().await.expect("initialized");
        drop(peer);
        assert_eq!(
            client.create_session(workspace(), DEADLINE).await,
            Err(Error::Transport(TransportError::Io))
        );
        assert_eq!(
            client.create_session(workspace(), DEADLINE).await,
            Err(Error::Unavailable)
        );
    });
}

#[test]
fn cancelled_partial_send_and_pending_receive_never_restore_initialization() {
    run(async {
        for capacity in [1, 1024] {
            let (mut client, mut peer) = live(capacity).await;
            let prefix = peer.wire.len();
            let mut attempt = Box::pin(client.create_session(workspace(), DEADLINE));
            assert!(poll_fn(|cx| Poll::Ready(attempt.as_mut().poll(cx).is_pending())).await);
            drop(attempt);
            assert_eq!(
                client.create_session(workspace(), DEADLINE).await,
                Err(Error::Unavailable)
            );
            assert!(client.initialize().await.is_err());
            drop(client);
            peer.output
                .read_to_end(&mut peer.wire)
                .await
                .expect("outbound EOF");
            if capacity == 1 {
                assert_eq!(&peer.wire[prefix..], b"{");
            } else {
                assert_wire(&peer.wire, workspace());
            }
        }
    });
}

#[test]
fn session_deadline_includes_delayed_send_and_receive() {
    run(async {
        let (mut client, mut peer) = live(1).await;
        let server = tokio::spawn(async move {
            tokio::time::sleep(Duration::from_secs(6)).await;
            peer.output
                .read_until(b'\n', &mut peer.wire)
                .await
                .expect("Session request");
            assert_wire(&peer.wire, workspace());
            tokio::time::sleep(Duration::from_secs(6)).await;
            peer.input
                .write_all(b"{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"sessionId\":\"late\"}}\n")
                .await
                .expect("late response");
        });
        let start = tokio::time::Instant::now();
        assert_eq!(
            client.create_session(workspace(), DEADLINE).await,
            Err(Error::Timeout)
        );
        assert_eq!(start.elapsed(), DEADLINE);
        assert_eq!(
            client.create_session(workspace(), DEADLINE).await,
            Err(Error::Unavailable)
        );
        server.abort();
        assert!(server.await.expect_err("cancel fixture").is_cancelled());
    });
}

#[test]
fn stalled_session_send_is_bounded_without_retry() {
    run(async {
        let (mut client, _peer) = live(1).await;
        let start = tokio::time::Instant::now();
        assert_eq!(
            client.create_session(workspace(), DEADLINE).await,
            Err(Error::Timeout)
        );
        assert_eq!(start.elapsed(), DEADLINE);
        assert_eq!(
            client.create_session(workspace(), DEADLINE).await,
            Err(Error::Unavailable)
        );
    });
}

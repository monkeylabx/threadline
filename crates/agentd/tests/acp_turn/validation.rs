use super::{
    acp_support::{run, Broken},
    fixtures::*,
};
use serde_json::{json, Value};
use std::time::Duration;
use threadline_agentd::acp::{stdio, Initialization, TransportError, TurnError as Error};
use tokio::io::AsyncReadExt;

#[test]
fn local_refusals_preserve_ready_session_without_sending_a_prompt() {
    run(async {
        let input = format!("{READY}{}", stream(&[terminal("end_turn")]));
        let (mut client, mut peer) = owner(input.as_bytes());
        assert_error(client.prompt("a", DEADLINE).await, Error::Unavailable);
        client.initialize().await.expect("initialized");
        assert_error(client.prompt("a", DEADLINE).await, Error::Unavailable);
        client
            .create_session(workspace(), DEADLINE)
            .await
            .expect("Session");
        for text in [String::new(), "x".repeat(65537), "é".repeat(32769)] {
            assert_error(client.prompt(&text, DEADLINE).await, Error::InvalidInput);
        }
        assert_error(
            client.prompt("a", Duration::ZERO).await,
            Error::InvalidDeadline,
        );
        let text = "é".repeat(32768);
        client.prompt(&text, DEADLINE).await.expect("UTF8 boundary");
        drop(client);
        let mut wire = Vec::new();
        peer.read_to_end(&mut wire).await.expect("outbound EOF");
        assert_wire(&wire, &text);
    });
}

#[test]
fn rejects_foreign_ids_errors_and_bad_terminal_results() {
    run(async {
        for id in [json!(1), json!(2), json!(4), json!("3")] {
            let mut end = terminal("end_turn");
            end["id"] = id;
            rejects(&[end], Error::UnexpectedMessage).await;
        }
        for result in [
            Value::Null,
            json!([]),
            json!("wrong"),
            json!({}),
            json!({"stopReason":null}),
            json!({"stopReason":3}),
            json!({"stopReason":"future"}),
        ] {
            rejects(
                &[json!({"jsonrpc":"2.0","id":3,"result":result})],
                Error::MalformedPayload,
            )
            .await;
        }
        for (id, expected) in [
            (json!(3), Error::RemoteError),
            (Value::Null, Error::UnexpectedMessage),
        ] {
            rejects(
                &[json!({"jsonrpc":"2.0","id":id,"error":{"code":-1,
                "message":"private-text","data":{"secret":"private"}}})],
                expected,
            )
            .await;
        }
        rejects(
            &[json!({"jsonrpc":"2.0","id":null,"result":{}})],
            Error::Transport(TransportError::Malformed),
        )
        .await;
    });
}

#[test]
fn rejects_callbacks_unsupported_updates_and_malformed_content() {
    run(async {
        let mutations = [
            (
                "/params/sessionId",
                json!("sess-é"),
                Error::UnexpectedMessage,
            ),
            ("/params/sessionId", Value::Null, Error::UnexpectedMessage),
            ("/params/update", Value::Null, Error::MalformedPayload),
            ("/params/update/content", json!([]), Error::MalformedPayload),
            (
                "/params/update/content/type",
                json!("image"),
                Error::UnexpectedMessage,
            ),
            (
                "/params/update/content/text",
                json!(3),
                Error::MalformedPayload,
            ),
            ("/params", json!([]), Error::MalformedPayload),
            ("/method", json!("future/notice"), Error::UnexpectedMessage),
        ];
        for (pointer, value, error) in mutations {
            let mut update = chunk("private-text");
            *update.pointer_mut(pointer).expect("field") = value;
            rejects(&[chunk("partial"), update], error).await;
        }
    });
}

#[test]
fn rejects_unsupported_updates_and_callbacks() {
    run(async {
        for kind in [
            "agent_thought_chunk",
            "tool_call",
            "tool_call_update",
            "plan",
            "notice",
            "compaction_update",
            "current_mode_update",
            "user_message_chunk",
            "future",
        ] {
            let mut update = chunk("private-text");
            update["params"]["update"]["sessionUpdate"] = json!(kind);
            rejects(&[update], Error::UnexpectedMessage).await;
        }
        for method in [
            "session/request_permission",
            "fs/read_text_file",
            "terminal/create",
            "session/update",
        ] {
            rejects(
                &[json!({"jsonrpc":"2.0","id":3,"method":method,"params":{"secret":"private"}})],
                Error::UnexpectedMessage,
            )
            .await;
        }
    });
}

#[test]
fn transport_failures_discard_partial_output_and_forbid_retry() {
    run(async {
        for (tail, error) in [
            ("", TransportError::Closed),
            ("{", TransportError::Truncated),
            ("not-json\n", TransportError::Malformed),
        ] {
            let tail = format!("{}{tail}", stream(&[chunk("partial-private")]));
            reply(&tail, "a", Some(Error::Transport(error)))
                .await
                .expect_err("transport");
        }
        let reader = READY.as_bytes().chain(Broken);
        let (reader, writer) = stdio(reader, tokio::io::sink(), IO_DEADLINE).expect("pipes");
        let mut client = Initialization::new(reader, writer, DEADLINE).expect("owner");
        client.initialize().await.expect("init");
        client
            .create_session(workspace(), DEADLINE)
            .await
            .expect("Session");
        assert_error(
            client.prompt("a", DEADLINE).await,
            Error::Transport(TransportError::Io),
        );
        assert_error(client.prompt("a", DEADLINE).await, Error::Unavailable);
        let (mut client, peer) = owner(READY.as_bytes());
        client.initialize().await.expect("init");
        client
            .create_session(workspace(), DEADLINE)
            .await
            .expect("Session");
        drop(peer);
        assert_error(
            client.prompt("a", DEADLINE).await,
            Error::Transport(TransportError::Io),
        );
        assert_error(client.prompt("a", DEADLINE).await, Error::Unavailable);
    });
}

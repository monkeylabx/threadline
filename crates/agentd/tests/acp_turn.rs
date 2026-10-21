mod acp_support;
#[path = "acp_turn/fixtures.rs"]
mod fixtures;
#[path = "acp_turn/lifecycle.rs"]
mod lifecycle;
#[path = "acp_turn/passive.rs"]
mod passive;
#[path = "acp_turn/passive_shapes.rs"]
mod passive_shapes;
#[path = "acp_turn/validation.rs"]
mod validation;

use acp_support::run;
use fixtures::{chunk, rejects, reply, stream, terminal};
use threadline_agentd::acp::{StopReason, TurnError as Error};

#[test]
fn aggregates_text_and_returns_only_typed_stop_reasons() {
    run(async {
        for (wire, typed) in [
            ("end_turn", StopReason::EndTurn),
            ("max_tokens", StopReason::MaxTokens),
            ("max_turn_requests", StopReason::MaxTurnRequests),
            ("refusal", StopReason::Refusal),
            ("cancelled", StopReason::Cancelled),
        ] {
            let mut end = terminal(wire);
            end["result"]["_meta"] = serde_json::json!({"secret":"worker-only"});
            let result = reply(
                &stream(&[chunk("private "), chunk("é\ntext"), end]),
                "input",
                None,
            )
            .await
            .expect("turn");
            assert_eq!(result.text(), "private é\ntext");
            assert_eq!(result.stop_reason(), typed);
            assert!(!format!("{result:?}").contains("private"));
            assert!(!format!("{result:?}").contains("worker-only"));
        }
        let result = reply(&stream(&[terminal("end_turn")]), "a", None)
            .await
            .expect("no updates");
        assert_eq!(result.text(), "");
        assert_eq!(
            format!("{}", Error::RemoteError),
            "ACP text turn RemoteError"
        );
        assert_eq!(format!("{:?}", Error::RemoteError), "RemoteError");
    });
}

#[test]
fn enforces_chunk_aggregate_update_and_utf8_byte_limits() {
    run(async {
        let piece = "é".repeat(4096);
        let mut messages = vec![chunk(&piece); 8];
        messages.push(terminal("end_turn"));
        let result = reply(&stream(&messages), &"x".repeat(65536), None)
            .await
            .expect("byte bounds");
        assert_eq!(result.text(), piece.repeat(8));
        let mut messages = vec![chunk(""); 64];
        messages.push(terminal("end_turn"));
        reply(&stream(&messages), "a", None)
            .await
            .expect("64 empty updates");
        for messages in [
            vec![chunk(&"x".repeat(8193))],
            vec![chunk(&"é".repeat(4097))],
            vec![chunk(""); 65],
            [vec![chunk(&"x".repeat(8192)); 8], vec![chunk("a")]].concat(),
            [vec![chunk(&"é".repeat(4096)); 8], vec![chunk("é")]].concat(),
        ] {
            rejects(&messages, Error::LimitExceeded).await;
        }
    });
}

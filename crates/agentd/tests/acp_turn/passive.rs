use super::{acp_support::run, fixtures::*};
use serde_json::{json, Value};
use threadline_agentd::acp::{StopReason, TransportError, TurnError as Error};

pub fn notice(update: Value) -> Value {
    json!({"jsonrpc":"2.0","method":"session/update",
        "params":{"sessionId":SESSION,"update":update}})
}

pub fn usage() -> Value {
    notice(json!({"sessionUpdate":"usage_update","used":0,"size":128000}))
}

pub fn info(id: Value) -> Value {
    notice(json!({"sessionUpdate":"session_info_update","_meta":{"goose":{"activeRunId":id}}}))
}

pub fn commands() -> Value {
    notice(
        json!({"sessionUpdate":"available_commands_update","availableCommands":[{
        "name":"shell","description":"worker claims authority",
        "input":{"hint":"ignored", "_meta":{}},"_meta":{"commandType":"Builtin"}}]}),
    )
}

#[test]
fn pinned_setup_and_lifecycle_interleave_without_exposing_passive_data() {
    run(async {
        let mut cost = usage();
        cost["params"]["update"]["cost"] = json!({"amount":0.25,"currency":"USD","_meta":{}});
        cost["params"]["update"]["_meta"] = json!({"provider":"ignored"});
        let result = reply(
            &stream(&[
                info(json!("run-untrusted")),
                usage(),
                commands(),
                chunk("synthetic "),
                cost,
                chunk("probe complete"),
                info(Value::Null),
                usage(),
                terminal("end_turn"),
            ]),
            "a",
            None,
        )
        .await
        .expect("pinned turn");
        assert_eq!(result.text(), "synthetic probe complete");
        assert_eq!(result.stop_reason(), StopReason::EndTurn);
        assert!(!format!("{result:?}").contains("untrusted"));
        reply(&stream(&[commands(), terminal("end_turn")]), "a", None)
            .await
            .expect("passive-only turn");
    });
}

#[test]
fn every_passive_shape_rejects_foreign_sessions_callbacks_and_methods() {
    run(async {
        for original in [usage(), commands(), info(Value::Null)] {
            for (key, value) in [
                ("session", json!("foreign")),
                ("session", Value::Null),
                ("id", json!(3)),
                ("method", json!("session/request_permission")),
                ("method", json!("fs/read_text_file")),
                ("method", json!("terminal/create")),
            ] {
                let mut frame = original.clone();
                if key == "session" {
                    frame["params"]["sessionId"] = value;
                } else {
                    frame[key] = value;
                }
                rejects(&[chunk("partial"), frame], Error::UnexpectedMessage).await;
            }
        }
    });
}

fn padded(frame: &Value, bytes: usize) -> String {
    let frame = format!("{frame}\n");
    assert!(frame.len() <= bytes);
    format!("{}{frame}", " ".repeat(bytes - frame.len()))
}

#[test]
fn passive_frame_and_raw_aggregate_budgets_include_whitespace_and_terminal() {
    run(async {
        let end = stream(&[terminal("end_turn")]);
        let max = padded(&usage(), 16_384);
        reply(&format!("{max}{end}"), "a", None)
            .await
            .expect("frame boundary");
        reply(&padded(&usage(), 16_385), "a", Some(Error::LimitExceeded))
            .await
            .expect_err("passive frame over limit");
        let prefix = max.repeat(7);
        let tail = padded(&usage(), 16_384 - end.len());
        reply(&format!("{prefix}{tail}{end}"), "a", None)
            .await
            .expect("aggregate boundary");
        reply(
            &format!("{prefix} {tail}{end}"),
            "a",
            Some(Error::LimitExceeded),
        )
        .await
        .expect_err("terminal crosses aggregate boundary");
        reply(&max.repeat(9), "a", Some(Error::LimitExceeded))
            .await
            .expect_err("notification crosses aggregate boundary");
        reply(
            &padded(&usage(), 262_145),
            "a",
            Some(Error::Transport(TransportError::TooLarge)),
        )
        .await
        .expect_err("transport frame limit remains");
    });
}

#[test]
fn passive_and_text_updates_share_one_count_budget() {
    run(async {
        let mut messages = vec![usage(); 63];
        messages.extend([chunk("bounded"), terminal("end_turn")]);
        assert_eq!(
            reply(&stream(&messages), "a", None)
                .await
                .expect("64")
                .text(),
            "bounded"
        );
        rejects(&vec![usage(); 65], Error::LimitExceeded).await;
        messages.insert(0, commands());
        rejects(&messages, Error::LimitExceeded).await;
    });
}

use super::{acp_support::run, fixtures::*, passive::*};
use serde_json::{json, Value};
use threadline_agentd::acp::TurnError as Error;

#[test]
fn malformed_passive_shapes_fail_closed() {
    run(async {
        for update in invalid_shapes() {
            rejects(&[chunk("partial"), notice(update)], Error::MalformedPayload).await;
        }
        for kind in ["plan", "tool_call", "config_option_update", "future"] {
            rejects(
                &[notice(json!({"sessionUpdate":kind}))],
                Error::UnexpectedMessage,
            )
            .await;
        }
    });
}

// Declarative malformed schema fixtures; no business logic.
fn invalid_shapes() -> Vec<Value> {
    vec![
        json!({"sessionUpdate":"usage_update"}),
        json!({"sessionUpdate":"usage_update","used":-1,"size":1}),
        json!({"sessionUpdate":"usage_update","used":0,"size":1.5}),
        json!({"sessionUpdate":"usage_update","used":"0","size":1}),
        json!({"sessionUpdate":"usage_update","used":0,"size":1,"extra":true}),
        json!({"sessionUpdate":"usage_update","used":0,"size":1,"_meta":null}),
        json!({"sessionUpdate":"usage_update","used":0,"size":1,"cost":null}),
        json!({"sessionUpdate":"usage_update","used":0,"size":1,"cost":{"amount":-1,"currency":"USD"}}),
        json!({"sessionUpdate":"usage_update","used":0,"size":1,"cost":{"amount":"0","currency":"USD"}}),
        json!({"sessionUpdate":"usage_update","used":0,"size":1,"cost":{"amount":0,"currency":""}}),
        json!({"sessionUpdate":"usage_update","used":0,"size":1,"cost":{"amount":0,"currency":"x".repeat(17)}}),
        json!({"sessionUpdate":"usage_update","used":0,"size":1,"cost":{"amount":0,"currency":"USD","extra":0}}),
        json!({"sessionUpdate":"available_commands_update"}),
        json!({"sessionUpdate":"available_commands_update","availableCommands":{}}),
        json!({"sessionUpdate":"available_commands_update","availableCommands":[null]}),
        json!({"sessionUpdate":"available_commands_update","availableCommands":[{}]}),
        json!({"sessionUpdate":"available_commands_update","availableCommands":[{"name":"","description":""}]}),
        json!({"sessionUpdate":"available_commands_update","availableCommands":[{"name":"x".repeat(257),"description":""}]}),
        json!({"sessionUpdate":"available_commands_update","availableCommands":[{"name":"a","description":"x".repeat(4097)}]}),
        json!({"sessionUpdate":"available_commands_update","availableCommands":[{"name":"a","description":null}]}),
        json!({"sessionUpdate":"available_commands_update","availableCommands":[{"name":"a","description":"","input":null}]}),
        json!({"sessionUpdate":"available_commands_update","availableCommands":[{"name":"a","description":"","input":{}}]}),
        json!({"sessionUpdate":"available_commands_update","availableCommands":[{"name":"a","description":"","input":{"hint":"x".repeat(1025)}}]}),
        json!({"sessionUpdate":"available_commands_update","availableCommands":[{"name":"a","description":"","extra":0}]}),
        json!({"sessionUpdate":"available_commands_update","availableCommands":vec![json!({"name":"a","description":""});33]}),
        json!({"sessionUpdate":"session_info_update"}),
        json!({"sessionUpdate":"session_info_update","_meta":{}}),
        json!({"sessionUpdate":"session_info_update","_meta":{"goose":null}}),
        json!({"sessionUpdate":"session_info_update","_meta":{"goose":{}}}),
        json!({"sessionUpdate":"session_info_update","_meta":{"goose":{"activeRunId":0}}}),
        json!({"sessionUpdate":"session_info_update","_meta":{"goose":{"activeRunId":""}}}),
        json!({"sessionUpdate":"session_info_update","_meta":{"goose":{"activeRunId":"x".repeat(257)}}}),
        json!({"sessionUpdate":"session_info_update","_meta":{"goose":{"activeRunId":"\n"}}}),
        json!({"sessionUpdate":"session_info_update","_meta":{"goose":{"activeRunId":null,"queuedSteer":{}}}}),
        json!({"sessionUpdate":"session_info_update","_meta":{"goose":{"activeRunId":null},"extra":{}}}),
        json!({"sessionUpdate":"session_info_update","title":"unsupported","_meta":{"goose":{"activeRunId":null}}}),
    ]
}

#[test]
fn accepts_schema_boundaries_without_using_worker_metadata() {
    run(async {
        for update in [
            json!({"sessionUpdate":"usage_update","used":u64::MAX,"size":0,"cost":{"amount":0,"currency":"x".repeat(16)}}),
            json!({"sessionUpdate":"available_commands_update","availableCommands":[]}),
            json!({"sessionUpdate":"available_commands_update","availableCommands":vec![json!({"name":"a","description":""});32],"_meta":{}}),
            json!({"sessionUpdate":"available_commands_update","availableCommands":[{"name":"é".repeat(128),"description":"x".repeat(4096),"input":{"hint":"x".repeat(1024)}}]}),
            json!({"sessionUpdate":"session_info_update","_meta":{"goose":{"activeRunId":"x".repeat(256)}}}),
        ] {
            let result = reply(
                &stream(&[notice(update), chunk("text"), terminal("end_turn")]),
                "a",
                None,
            )
            .await
            .expect("boundary");
            assert_eq!(result.text(), "text");
        }
    });
}

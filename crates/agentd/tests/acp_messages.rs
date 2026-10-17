use serde_json::json;
use threadline_agentd::acp::{RpcMessage, TransportError};

#[test]
fn accepts_calls_notifications_and_exclusive_result_or_error_responses() {
    for value in [
        json!({"jsonrpc":"2.0","method":"initialize","id":1,"params":{}}),
        json!({"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"s"}}),
        json!({"jsonrpc":"2.0","method":"_extension","id":"x","params":[]}),
        json!({"jsonrpc":"2.0","id":1,"result":null,"_meta":{}}),
        json!({"jsonrpc":"2.0","id":"x","error":{"code":-32601,"message":"denied","data":{}}}),
        json!({"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"parse error"}}),
    ] {
        assert_eq!(
            RpcMessage::new(value.clone())
                .expect("valid envelope")
                .value(),
            &value
        );
    }
}

#[test]
fn rejects_malformed_envelopes_and_reserved_field_combinations() {
    for value in [
        json!(null),
        json!([]),
        json!("log"),
        json!({"id":1,"result":{}}),
        json!({"jsonrpc":"1.0","id":1,"result":{}}),
        json!({"jsonrpc":"2.0","method":1}),
        json!({"jsonrpc":"2.0","method":"m","result":{}}),
        json!({"jsonrpc":"2.0","method":"m","error":{}}),
        json!({"jsonrpc":"2.0","method":"m","id":null}),
        json!({"jsonrpc":"2.0","method":"m","id":1.5}),
        json!({"jsonrpc":"2.0","method":"m","params":null}),
        json!({"jsonrpc":"2.0","method":"m","params":1}),
        json!({"jsonrpc":"2.0","result":{}}),
        json!({"jsonrpc":"2.0","id":true,"result":{}}),
        json!({"jsonrpc":"2.0","id":null,"result":{}}),
        json!({"jsonrpc":"2.0","id":1}),
        json!({"jsonrpc":"2.0","id":1,"params":{},"result":{}}),
        json!({"jsonrpc":"2.0","id":1,"result":{},"error":{"code":0,"message":"m"}}),
        json!({"jsonrpc":"2.0","id":1,"error":null}),
        json!({"jsonrpc":"2.0","id":1,"error":{"code":1.5,"message":"m"}}),
        json!({"jsonrpc":"2.0","id":1,"error":{"code":1}}),
        json!({"jsonrpc":"2.0","id":1,"error":{"code":1,"message":7}}),
    ] {
        assert_eq!(
            RpcMessage::new(value).expect_err("invalid envelope"),
            TransportError::Malformed
        );
    }
}

#[test]
fn debug_and_errors_do_not_disclose_payload_content() {
    let frame =
        RpcMessage::new(json!({"jsonrpc":"2.0","id":1,"result":"private-secret"})).expect("frame");
    assert_eq!(format!("{frame:?}"), "RpcMessage { .. }");
    assert_eq!(
        TransportError::Malformed.to_string(),
        "ACP transport Malformed"
    );
}

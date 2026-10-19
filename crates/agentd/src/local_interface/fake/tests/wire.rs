use serde_json::{json, Value};

pub fn assert_turn(bytes: &[u8], text: &str) {
    let frames: Vec<Value> = bytes
        .split(|byte| *byte == b'\n')
        .filter(|line| !line.is_empty())
        .map(|line| serde_json::from_slice(line).expect("JSON request"))
        .collect();
    let workspace = if cfg!(windows) {
        "C:\\synthetic"
    } else {
        "/synthetic"
    };
    assert_eq!(
        frames,
        vec![
            json!({"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientCapabilities":{}}}),
            json!({"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":workspace,"mcpServers":[]}}),
            json!({"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"engine-private","prompt":[{"type":"text","text":text}]}}),
        ]
    );
    assert_eq!(bytes.iter().filter(|byte| **byte == b'\n').count(), 3);
}

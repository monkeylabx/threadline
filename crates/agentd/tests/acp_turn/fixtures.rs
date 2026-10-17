use serde_json::{json, Value};
use std::time::Duration;
use threadline_agentd::acp::{stdio, Initialization, TurnError as Error, TurnResult};
use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt, BufReader, DuplexStream};

pub const DEADLINE: Duration = Duration::from_secs(10);
pub const IO_DEADLINE: Duration = Duration::from_secs(60);
pub const SESSION: &str = " sess-é ";
pub const READY: &str = "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":1}}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"sessionId\":\" sess-é \"}}\n";

pub fn workspace() -> &'static str {
    if cfg!(windows) {
        "C:\\synthetic"
    } else {
        "/synthetic"
    }
}
pub fn chunk(text: &str) -> Value {
    json!({"jsonrpc":"2.0","method":"session/update",
    "params":{"sessionId":SESSION,"update":{"sessionUpdate":"agent_message_chunk",
    "messageId":"ignored","content":{"type":"text","text":text},"_meta":{"private":"ignored"}}}})
}
pub fn terminal(reason: &str) -> Value {
    json!({"jsonrpc":"2.0","id":3,"result":{"stopReason":reason}})
}
pub fn stream(messages: &[Value]) -> String {
    messages.iter().map(|m| format!("{m}\n")).collect()
}

pub fn owner(input: &[u8]) -> (Initialization<&[u8], DuplexStream>, DuplexStream) {
    let (writer, peer) = tokio::io::duplex(1024 * 1024);
    let (reader, writer) = stdio(input, writer, IO_DEADLINE).expect("pipes");
    (
        Initialization::new(reader, writer, DEADLINE).expect("owner"),
        peer,
    )
}

pub fn assert_wire(bytes: &[u8], text: &str) {
    let frames: Vec<Value> = bytes
        .split(|b| *b == b'\n')
        .filter(|b| !b.is_empty())
        .map(|b| serde_json::from_slice(b).expect("JSON frame"))
        .collect();
    assert_eq!(
        frames,
        vec![
            json!({"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientCapabilities":{}}}),
            json!({"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":workspace(),"mcpServers":[]}}),
            json!({"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":SESSION,"prompt":[{"type":"text","text":text}]}}),
        ]
    );
    assert_eq!(bytes.iter().filter(|b| **b == b'\n').count(), 3);
}

pub async fn reply(tail: &str, text: &str, expected: Option<Error>) -> Result<TurnResult, Error> {
    let input = format!("{READY}{tail}");
    let (mut client, mut peer) = owner(input.as_bytes());
    client.initialize().await.expect("initialized");
    client
        .create_session(workspace(), DEADLINE)
        .await
        .expect("Session");
    let result = client.prompt(text, DEADLINE).await;
    assert_eq!(result.as_ref().err().copied(), expected);
    assert_error(client.prompt("retry", DEADLINE).await, Error::Unavailable);
    assert!(client.initialize().await.is_err());
    assert!(client.create_session(workspace(), DEADLINE).await.is_err());
    drop(client);
    let mut output = Vec::new();
    peer.read_to_end(&mut output).await.expect("outbound EOF");
    assert_wire(&output, text);
    result
}

pub struct Peer {
    pub input: DuplexStream,
    pub output: BufReader<DuplexStream>,
    pub wire: Vec<u8>,
}

pub async fn live(capacity: usize) -> (Initialization<DuplexStream, DuplexStream>, Peer) {
    let (reader, mut input) = tokio::io::duplex(4096);
    let (writer, output) = tokio::io::duplex(capacity);
    let (reader, writer) = stdio(reader, writer, IO_DEADLINE).expect("pipes");
    let mut client = Initialization::new(reader, writer, DEADLINE).expect("owner");
    input
        .write_all(READY.as_bytes())
        .await
        .expect("ready replies");
    let capture = tokio::spawn(async move {
        let mut output = BufReader::new(output);
        let mut wire = Vec::new();
        for _ in 0..2 {
            output
                .read_until(b'\n', &mut wire)
                .await
                .expect("setup request");
        }
        (output, wire)
    });
    client.initialize().await.expect("initialized");
    client
        .create_session(workspace(), DEADLINE)
        .await
        .expect("Session");
    let (output, wire) = capture.await.expect("capture");
    (
        client,
        Peer {
            input,
            output,
            wire,
        },
    )
}

pub fn assert_error(result: Result<TurnResult, Error>, expected: Error) {
    assert_eq!(result.expect_err("refused turn"), expected);
}

pub async fn rejects(messages: &[Value], error: Error) {
    reply(&stream(messages), "a", Some(error))
        .await
        .expect_err("rejected exchange");
}

use serde_json::{json, Value};
use std::time::Duration;
use threadline_agentd::acp::{stdio, CreatedSession, Initialization, SessionError as Error};
use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt, BufReader, DuplexStream};

pub const DEADLINE: Duration = Duration::from_secs(10);
pub const IO_DEADLINE: Duration = Duration::from_secs(60);
pub const INIT_REPLY: &str = "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":1}}\n";

pub fn workspace() -> &'static str {
    if cfg!(windows) {
        "C:\\threadline-fixture"
    } else {
        "/threadline-fixture"
    }
}

pub fn owner(input: &[u8]) -> (Initialization<&[u8], DuplexStream>, DuplexStream) {
    let (writer, peer) = tokio::io::duplex(8192);
    let (reader, writer) = stdio(input, writer, IO_DEADLINE).expect("pipes");
    (
        Initialization::new(reader, writer, DEADLINE).expect("owner"),
        peer,
    )
}

pub fn assert_wire(bytes: &[u8], workspace: &str) {
    let frames: Vec<Value> = bytes
        .split(|b| *b == b'\n')
        .filter(|b| !b.is_empty())
        .map(|b| serde_json::from_slice(b).expect("JSON frame"))
        .collect();
    assert_eq!(
        frames,
        vec![
            json!({"jsonrpc":"2.0","id":1,"method":"initialize",
            "params":{"protocolVersion":1,"clientCapabilities":{}}}),
            json!({"jsonrpc":"2.0","id":2,"method":"session/new",
            "params":{"cwd":workspace,"mcpServers":[]}}),
        ]
    );
    assert_eq!(bytes.iter().filter(|b| **b == b'\n').count(), 2);
}

pub async fn reply(response: Value, expected: Option<Error>) -> Result<CreatedSession, Error> {
    let input = format!("{INIT_REPLY}{response}\n");
    let (mut client, mut peer) = owner(input.as_bytes());
    client.initialize().await.expect("initialized");
    let result = client.create_session(workspace(), DEADLINE).await;
    assert_eq!(result.as_ref().err().copied(), expected);
    assert_eq!(
        client.create_session(workspace(), DEADLINE).await,
        Err(Error::Unavailable)
    );
    assert!(client.initialize().await.is_err());
    drop(client);
    let mut wire = Vec::new();
    peer.read_to_end(&mut wire).await.expect("outbound EOF");
    assert_wire(&wire, workspace());
    result
}

pub struct Peer {
    pub input: DuplexStream,
    pub output: BufReader<DuplexStream>,
    pub wire: Vec<u8>,
}

pub async fn live(capacity: usize) -> (Initialization<DuplexStream, DuplexStream>, Peer) {
    let (reader, mut input) = tokio::io::duplex(1024);
    let (writer, output) = tokio::io::duplex(capacity);
    let (reader, writer) = stdio(reader, writer, IO_DEADLINE).expect("pipes");
    let mut client = Initialization::new(reader, writer, DEADLINE).expect("owner");
    input
        .write_all(INIT_REPLY.as_bytes())
        .await
        .expect("init response");
    let capture = tokio::spawn(async move {
        let mut output = BufReader::new(output);
        let mut wire = Vec::new();
        output
            .read_until(b'\n', &mut wire)
            .await
            .expect("initialize request");
        (output, wire)
    });
    client.initialize().await.expect("initialized");
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

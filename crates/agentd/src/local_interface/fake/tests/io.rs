use super::super::FakeRun;
use crate::{
    acp::{stdio, Initialization},
    local_interface::{tests::authority::Source, InputError as Error},
};
use std::time::Duration;
use tokio::io::{AsyncRead, AsyncReadExt, DuplexStream};

pub const INIT: &str = "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":1}}\n";
pub const READY: &str = concat!(
    "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":1}}\n",
    "{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"sessionId\":\"engine-private\"}}\n"
);
pub const CHUNK: &str = "{\"jsonrpc\":\"2.0\",\"method\":\"session/update\",\"params\":{\"sessionId\":\"engine-private\",\"update\":{\"sessionUpdate\":\"agent_message_chunk\",\"content\":{\"type\":\"text\",\"text\":\"safe response\"},\"_meta\":{\"reasoning\":\"PRIVATE-WORKER-CONTENT\"}}}}\n";
pub const END: &str = "{\"jsonrpc\":\"2.0\",\"id\":3,\"result\":{\"stopReason\":\"end_turn\"}}\n";

pub fn pipes<R: AsyncRead + Unpin>(
    reader: R,
    writer: DuplexStream,
) -> Initialization<R, DuplexStream> {
    let (reader, writer) = stdio(reader, writer, Duration::from_secs(60)).expect("fixed pipes");
    Initialization::new(reader, writer, Duration::from_secs(10)).expect("fixed deadline")
}

pub fn unavailable() -> Result<Initialization<&'static [u8], DuplexStream>, Error> {
    Err(Error::Unavailable)
}

pub async fn drive_reply(
    owner: &mut FakeRun,
    source: &Source,
    reply: &str,
) -> (Result<(), Error>, Vec<u8>) {
    let (writer, mut capture) = tokio::io::duplex(1024 * 1024);
    let result = owner
        .drive_with(source, || Ok(pipes(reply.as_bytes(), writer)))
        .await;
    let mut bytes = Vec::new();
    capture.read_to_end(&mut bytes).await.expect("outbound EOF");
    (result, bytes)
}

pub fn valid() -> String {
    format!("{READY}{CHUNK}{END}")
}

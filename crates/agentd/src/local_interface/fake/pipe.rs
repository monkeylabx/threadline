use crate::{
    acp::{stdio, Initialization, StopReason},
    local_interface::InputError as Error,
};
use std::time::Duration;
use tokio::io::{AsyncRead, AsyncWrite, Sink};

const DEADLINE: Duration = Duration::from_secs(10);
const REPLIES: &[u8] = concat!(
    "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":1}}\n",
    "{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"sessionId\":\"private-demo-session\"}}\n",
    "{\"jsonrpc\":\"2.0\",\"method\":\"session/update\",\"params\":{\"sessionId\":\"private-demo-session\",\"update\":{\"sessionUpdate\":\"agent_message_chunk\",\"content\":{\"type\":\"text\",\"text\":\"Demonstration response.\"},\"_meta\":{\"diagnostic\":\"PRIVATE-WORKER-DIAGNOSTIC\"}}}}\n",
    "{\"jsonrpc\":\"2.0\",\"id\":3,\"result\":{\"stopReason\":\"end_turn\"}}\n"
).as_bytes();

pub(super) fn fixed() -> Result<Initialization<&'static [u8], Sink>, Error> {
    let (reader, writer) =
        stdio(REPLIES, tokio::io::sink(), DEADLINE).map_err(|_| Error::RunInputOutcomeUncertain)?;
    Initialization::new(reader, writer, DEADLINE).map_err(|_| Error::RunInputOutcomeUncertain)
}

pub(super) async fn turn<R: AsyncRead + Unpin, W: AsyncWrite + Unpin>(
    mut client: Initialization<R, W>,
    text: &str,
) -> Result<String, Error> {
    // Collapse protocol failures to a coarse uncertain outcome; no worker details escape.
    client
        .initialize()
        .await
        .map_err(|_| Error::RunInputOutcomeUncertain)?;
    let workspace = if cfg!(windows) {
        "C:\\synthetic"
    } else {
        "/synthetic"
    };
    client
        .create_session(workspace, DEADLINE)
        .await
        .map_err(|_| Error::RunInputOutcomeUncertain)?;
    let result = client
        .prompt(text, DEADLINE)
        .await
        .map_err(|_| Error::RunInputOutcomeUncertain)?;
    if result.stop_reason() == StopReason::Cancelled {
        return Err(Error::RunInputOutcomeUncertain);
    }
    Ok(result.text().to_owned())
}

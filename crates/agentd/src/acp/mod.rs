//! Internal pipe framing, not an ACP Session client or a source of authority.

mod connection;
mod framing;
mod message;

use connection::Connection;
use std::{fmt, time::Duration};
use tokio::io::{AsyncRead, AsyncWrite, BufReader};

pub use message::RpcMessage;
pub const MAX_FRAME_BYTES: usize = 256 * 1024;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum TransportError {
    Closed,
    Truncated,
    TooLarge,
    Malformed,
    Io,
    Timeout,
    InvalidDeadline,
}

impl fmt::Display for TransportError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "ACP transport {:?}", self)
    }
}

impl std::error::Error for TransportError {}

pub struct AcpReader<R> {
    reader: BufReader<R>,
    connection: Connection,
}

pub struct AcpWriter<W> {
    writer: W,
    connection: Connection,
}

/// Bind already owned worker pipes. The trusted owner supplies the deadline.
/// This does not launch a process, validate Run authority, or admit any ACP tools.
pub fn stdio<R, W>(
    reader: R,
    writer: W,
    deadline: Duration,
) -> Result<(AcpReader<R>, AcpWriter<W>), TransportError>
where
    R: AsyncRead + Unpin,
    W: AsyncWrite + Unpin,
{
    let connection = Connection::new(deadline)?;
    Ok((
        AcpReader {
            reader: BufReader::new(reader),
            connection: connection.clone(),
        },
        AcpWriter { writer, connection },
    ))
}

impl<R: AsyncRead + Unpin> AcpReader<R> {
    pub async fn receive(&mut self) -> Result<RpcMessage, TransportError> {
        self.connection.run(framing::read(&mut self.reader)).await
    }
}

impl<W: AsyncWrite + Unpin> AcpWriter<W> {
    pub async fn send(&mut self, message: &RpcMessage) -> Result<(), TransportError> {
        self.connection
            .run(framing::write(&mut self.writer, message))
            .await
    }
}

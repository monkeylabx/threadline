use super::{RpcMessage, TransportError, MAX_FRAME_BYTES};
use std::io::{self, Write};
use tokio::io::{AsyncBufRead, AsyncBufReadExt, AsyncReadExt, AsyncWrite, AsyncWriteExt};

pub(super) async fn read<R: AsyncBufRead + Unpin>(
    reader: &mut R,
) -> Result<RpcMessage, TransportError> {
    let mut bytes = Vec::with_capacity(MAX_FRAME_BYTES);
    reader
        .take(MAX_FRAME_BYTES as u64)
        .read_until(b'\n', &mut bytes)
        .await
        .map_err(|_| TransportError::Io)?;
    if bytes.last() == Some(&b'\n') {
        bytes.pop();
        let value = serde_json::from_slice(&bytes).map_err(|_| TransportError::Malformed)?;
        return RpcMessage::new(value);
    }
    match bytes.len() {
        0 => Err(TransportError::Closed),
        MAX_FRAME_BYTES => Err(TransportError::TooLarge),
        _ => Err(TransportError::Truncated),
    }
}

pub(super) async fn write<W: AsyncWrite + Unpin>(
    writer: &mut W,
    message: &RpcMessage,
) -> Result<(), TransportError> {
    let mut buffer = FrameBuffer(Vec::with_capacity(MAX_FRAME_BYTES));
    serde_json::to_writer(&mut buffer, message.value()).map_err(|_| TransportError::TooLarge)?;
    buffer.0.push(b'\n');
    writer
        .write_all(&buffer.0)
        .await
        .map_err(|_| TransportError::Io)?;
    writer.flush().await.map_err(|_| TransportError::Io)
}

struct FrameBuffer(Vec<u8>);

impl Write for FrameBuffer {
    fn write(&mut self, bytes: &[u8]) -> io::Result<usize> {
        if bytes.len() > MAX_FRAME_BYTES - 1 - self.0.len() {
            return Err(io::ErrorKind::FileTooLarge.into());
        }
        self.0.extend_from_slice(bytes);
        Ok(bytes.len())
    }

    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}

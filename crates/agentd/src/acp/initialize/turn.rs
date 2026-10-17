use super::{Initialization, Phase, RpcMessage};
use std::time::Duration;
use tokio::io::{AsyncRead, AsyncWrite};

mod payload;
mod result;
use payload::{request, TextTurn, MAX_UPDATES};
pub use result::{StopReason, TurnError, TurnResult};

impl<R: AsyncRead + Unpin, W: AsyncWrite + Unpin> Initialization<R, W> {
    /// Internal protocol data only. The future product owner must authorize input.
    /// No partial text escapes, and a polled exchange never permits another turn.
    pub async fn prompt(
        &mut self,
        text: &str,
        deadline: Duration,
    ) -> Result<TurnResult, TurnError> {
        let Phase::Session(session) = &self.phase else {
            return Err(TurnError::Unavailable);
        };
        let request = request(session.id(), text, deadline)?;
        let session = session.id().to_owned();
        self.phase = Phase::Unavailable;
        tokio::time::timeout(deadline, self.exchange_turn(&session, &request))
            .await
            .map_err(|_| TurnError::Timeout)?
    }

    async fn exchange_turn(
        &mut self,
        session: &str,
        request: &RpcMessage,
    ) -> Result<TurnResult, TurnError> {
        self.writer
            .send(request)
            .await
            .map_err(TurnError::Transport)?;
        let mut turn = TextTurn::new(session);
        for _ in 0..=MAX_UPDATES {
            let message = self.reader.receive().await.map_err(TurnError::Transport)?;
            if let Some(result) = turn.accept(message.value())? {
                return Ok(result);
            }
        }
        Err(TurnError::LimitExceeded)
    }
}

use super::{attempt::Attempt, pipe, FakeRun};
use crate::{
    acp::Initialization,
    local_interface::{admission, ActivityUpdate, AuthoritySource, InputError as Error},
};
use tokio::io::{AsyncRead, AsyncWrite};

impl FakeRun {
    /// Trusted owner operation, not an additional local RPC or a durable Run result.
    pub async fn drive(&mut self, source: &dyn AuthoritySource) -> Result<(), Error> {
        self.drive_with(source, pipe::fixed).await
    }

    // Private hermetic transport seam; the public path always uses the fixed fake.
    pub(super) async fn drive_with<R, W, F>(
        &mut self,
        source: &dyn AuthoritySource,
        factory: F,
    ) -> Result<(), Error>
    where
        R: AsyncRead + Unpin,
        W: AsyncWrite + Unpin,
        F: FnOnce() -> Result<Initialization<R, W>, Error>,
    {
        let pending = self.pending.as_ref().ok_or(Error::InvalidStateTransition)?;
        admission::check_input(source, &pending.run_id)?;
        let pending = self.pending.take().ok_or(Error::InvalidStateTransition)?;
        let attempt = Attempt::new(&mut self.intake);
        let client = factory().map_err(|_| Error::RunInputOutcomeUncertain)?;
        let text = pipe::turn(client, &pending.text).await?;
        self.activity
            .append(&pending.input_id, ActivityUpdate::Text(&text))?;
        attempt.complete();
        Ok(())
    }
}

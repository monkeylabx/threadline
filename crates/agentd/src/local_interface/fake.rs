use super::{
    Activity, AuthoritySource, InputError as Error, InputReceipt as Receipt, Intake, RunInput,
    Watch,
};
use std::fmt;
use threadline_client_proto::threadline::local_agent::v1::WatchRunActivityRequest;

mod attempt;
mod drive;
mod pipe;

/// NOT PRODUCTION: one dispatched demonstration Run, with no process or listener.
pub struct FakeRun {
    intake: Intake,
    activity: Activity,
    pending: Option<RunInput>,
}

impl FakeRun {
    pub fn new(run_id: String, incarnation: String) -> Result<Self, Error> {
        Ok(Self {
            activity: Activity::new(run_id.clone(), incarnation)?,
            intake: Intake::new(run_id),
            pending: None,
        })
    }

    /// The receipt precedes dispatch; duplicate/conflicting bytes cannot replace pending input.
    pub fn submit(
        &mut self,
        source: &dyn AuthoritySource,
        input: &RunInput,
    ) -> Result<Receipt, Error> {
        let receipt = self.intake.accept(source, input)?;
        if receipt == Receipt::Accepted {
            self.pending = Some(input.clone());
        }
        Ok(receipt)
    }

    pub fn watch(
        &mut self,
        source: &dyn AuthoritySource,
        request: &WatchRunActivityRequest,
    ) -> Result<Watch, Error> {
        self.activity.watch(source, request)
    }

    /// Immutable view access for already scoped Watch emission/reconnect only.
    pub fn activity(&self) -> &Activity {
        &self.activity
    }
}

impl fmt::Debug for FakeRun {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("FakeRun").finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests;

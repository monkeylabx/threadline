use crate::{ContainmentState, StopError};

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub enum StepResult {
    #[default]
    NotAttempted,
    Succeeded,
    Failed(StopError),
}

impl From<Result<(), StopError>> for StepResult {
    fn from(result: Result<(), StopError>) -> Self {
        match result {
            Ok(()) => Self::Succeeded,
            Err(error) => Self::Failed(error),
        }
    }
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub enum CleanupResult {
    #[default]
    NotObserved,
    LiveMembers,
    SealedAndEmpty,
    Unknown(StopError),
}

impl From<Result<ContainmentState, StopError>> for CleanupResult {
    fn from(result: Result<ContainmentState, StopError>) -> Self {
        match result {
            Ok(ContainmentState::LiveMembers) => Self::LiveMembers,
            Ok(ContainmentState::SealedAndEmpty) => Self::SealedAndEmpty,
            Err(error) => Self::Unknown(error),
        }
    }
}

/// Local observations only: no field is a durable Core Run state or authorization.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct StopReport {
    pub requested: bool,
    pub revocation: StepResult,
    pub interruption: StepResult,
    pub termination: StepResult,
    pub cleanup: CleanupResult,
}

impl StopReport {
    /// Local proof only; durable cancellation still needs owner reconciliation.
    pub fn cleanup_confirmed(&self) -> bool {
        self.requested
            && self.revocation == StepResult::Succeeded
            && self.cleanup == CleanupResult::SealedAndEmpty
    }
}

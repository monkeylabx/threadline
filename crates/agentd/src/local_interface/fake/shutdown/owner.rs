use crate::{
    local_interface::{AuthoritySource, InputError as Error, LookupError},
    CleanupResult, StepResult, StopReport,
};

/// Evidence submitted to Core, never a locally invented Run terminal state.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum CleanupConclusion {
    Verified,
    Failed,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct OwnerReceipt {
    pub conclusion: CleanupConclusion,
    pub acknowledged: bool,
}

/// NOT PRODUCTION: trusted harness replacement for the owner/Core reporting path.
/// True must acknowledge this exact Run/conclusion durably; false is still pending.
/// Implementations must be idempotent and reject changing an acknowledged conclusion.
/// No protected tool or Approval decision delivery is provided by this seam.
pub trait OwnerReporter {
    fn report(&mut self, run_id: &str, conclusion: CleanupConclusion) -> Result<bool, Error>;
}

pub(super) fn conclusion(report: StopReport) -> Result<CleanupConclusion, Error> {
    if report.cleanup_confirmed() {
        return Ok(CleanupConclusion::Verified);
    }
    let failed = matches!(report.revocation, StepResult::Failed(_))
        || matches!(report.termination, StepResult::Failed(_))
        || matches!(report.cleanup, CleanupResult::Unknown(_));
    if report.requested && failed {
        return Ok(CleanupConclusion::Failed);
    }
    Err(Error::InvalidStateTransition)
}

/// Supervisor identity comes from trusted current Run facts, independently of UI lifetime.
pub(super) fn check_owner(source: &dyn AuthoritySource, run_id: &str) -> Result<(), Error> {
    let run = source.run(run_id).map_err(|error| match error {
        LookupError::Unknown => Error::PermissionDenied,
        LookupError::Unavailable => Error::Unavailable,
    })?;
    if run.id != run_id {
        return Err(Error::PermissionDenied);
    }
    if !run.owner {
        return Err(Error::NotExecutionOwner);
    }
    if !run.lease {
        return Err(Error::LeaseLost);
    }
    if !run.fencing {
        return Err(Error::FencingTokenStale);
    }
    Ok(())
}

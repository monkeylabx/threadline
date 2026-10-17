use crate::{CleanupResult, StepResult, StopDriver, StopReport};

/// Coordinates explicit shutdown of one worker; never restarts or dispatches it.
///
/// The owner must retain this object and poll pending cleanup to resolution.
/// Drop performs no blocking I/O and is not a shutdown guarantee. A future process
/// owner must separately enforce cleanup when agentd crashes or exits.
pub struct WorkerStop<D> {
    driver: D,
    report: StopReport,
}

impl<D: StopDriver> WorkerStop<D> {
    pub fn new(driver: D) -> Self {
        Self {
            driver,
            report: StopReport::default(),
        }
    }

    /// Latch before any adapter call. The caller must fence dispatch on this latch.
    /// Repeated requests never reset progress or reopen execution.
    pub fn request_stop(&mut self) -> StopReport {
        self.report.requested = true;
        self.report
    }

    pub fn snapshot(&self) -> StopReport {
        self.report
    }

    /// One explicit bounded attempt; no sleeping, automatic replay, or retry loop.
    pub fn poll_stop(&mut self) -> StopReport {
        if !self.report.requested || self.report.cleanup_confirmed() {
            return self.report;
        }
        if self.report.revocation != StepResult::Succeeded {
            self.report.revocation = self.driver.revoke_effects().into();
        }
        if self.report.interruption == StepResult::NotAttempted {
            self.report.interruption = self.driver.interrupt().into();
        }
        if self.report.cleanup != CleanupResult::SealedAndEmpty {
            self.report.termination = self.driver.terminate().into();
            self.report.cleanup = self.driver.observe_containment().into();
        }
        self.report
    }
}

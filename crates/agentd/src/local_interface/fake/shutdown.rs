//! Fake-only lifecycle composition; Core remains the durable state authority.
use super::FakeRun;
use crate::{
    local_interface::{binding, AuthoritySource, InputError as Error},
    StopDriver, StopReport, WorkerStop,
};

mod owner;
pub use owner::{CleanupConclusion, OwnerReceipt, OwnerReporter};

/// Exclusive shutdown capability for this Run and its trusted, exclusively owned driver.
/// No process adapter exists. Denied requests perform no erase, dispatch or driver I/O.
pub struct Shutdown<'a, D> {
    run: &'a mut FakeRun,
    worker: WorkerStop<D>,
}

impl<'a, D: StopDriver> Shutdown<'a, D> {
    /// Retain until cleanup resolves; Drop is not cleanup. The driver must belong
    /// exclusively to this Run. Cancel/drop any polled drive future before borrowing.
    pub fn new(run: &'a mut FakeRun, driver: D) -> Self {
        Self {
            run,
            worker: WorkerStop::new(driver),
        }
    }

    /// Immediate local receipt only. Cleanup and Core acknowledgement happen separately.
    pub fn request(&mut self, source: &dyn AuthoritySource) -> Result<StopReport, Error> {
        let run = binding::controlled_run(source, &self.run.run_id)?;
        if !run.owner {
            return Err(Error::NotExecutionOwner);
        }
        Ok(self.invalidate())
    }

    /// Trusted revocation/terminal notification; not callable from serialized UI fields.
    /// Fencing survives dropping this handle or a subsequently restored Grant/lease.
    pub fn invalidate(&mut self) -> StopReport {
        let receipt = self.worker.request_stop();
        self.run.intake.clear();
        self.run.pending = None;
        self.run.activity.clear();
        receipt
    }

    /// Explicit bounded cleanup, also after UI/control/owner authority has been lost.
    pub fn poll(&mut self) -> StopReport {
        self.worker.poll_stop()
    }

    /// Trusted owner path, not a UI RPC. No cached UI identity or local Core state.
    pub fn report(
        &self,
        source: &dyn AuthoritySource,
        reporter: &mut dyn OwnerReporter,
    ) -> Result<OwnerReceipt, Error> {
        let conclusion = owner::conclusion(self.worker.snapshot())?;
        owner::check_owner(source, &self.run.run_id)?;
        Ok(OwnerReceipt {
            conclusion,
            acknowledged: reporter.report(&self.run.run_id, conclusion)?,
        })
    }
}

//! W1 cooperative launch probe; no production admission or sealed-cleanup receipt.
#[path = "windows_launch_attributes.rs"]
mod attributes;
#[path = "windows_launch_cases.rs"]
mod cases;
#[path = "windows_launch_faults.rs"]
mod faults;
#[path = "windows_launch_process.rs"]
mod process;

use super::{api as job_api, Job, JobError};
use attributes::Attributes;
use process::Handles;
use std::time::{Duration, Instant};
use windows_sys::Win32::Foundation::HANDLE;

const CHILD: &str = "probe::launch::cases::windows_launch_fixture_entry";
const BUDGET: Duration = Duration::from_secs(10);

#[derive(PartialEq, Eq)]
enum Phase {
    Suspended,
    Blocked,
    Running,
    Reaped(u32),
}

struct Child<'job> {
    handles: Handles,
    job: &'job Job,
    phase: Phase,
}

impl<'job> Child<'job> {
    fn new(job: &'job Job) -> Result<Self, JobError> {
        Self::create_with(job, process::create)
    }

    fn create_with(
        job: &'job Job,
        create: impl FnOnce(&mut Attributes<'_>) -> Result<Handles, JobError>,
    ) -> Result<Self, JobError> {
        let mut attributes = Attributes::new(job)?;
        let handles = create(&mut attributes)?;
        Ok(Self {
            handles,
            job,
            phase: Phase::Suspended,
        })
    }

    fn resume(&mut self) -> Result<(), JobError> {
        self.resume_with(process::member, process::resume)
    }

    fn resume_with(
        &mut self,
        member: impl FnOnce(HANDLE, HANDLE) -> Result<bool, JobError>,
        resume: impl FnOnce(HANDLE) -> Result<(), JobError>,
    ) -> Result<(), JobError> {
        if self.phase != Phase::Suspended {
            return Err(JobError::Rejected);
        }
        self.phase = Phase::Blocked; // Latch before any observation/effect; no retry.
        let job = self.job.handle.ok_or(JobError::Rejected)?.as_ptr();
        if !member(self.handles.process, job)? {
            return Err(JobError::Rejected);
        }
        resume(self.handles.thread)?;
        self.phase = Phase::Running;
        Ok(())
    }

    fn wait(&mut self, millis: u32) -> Result<u32, JobError> {
        if let Phase::Reaped(code) = self.phase {
            return Ok(code);
        }
        let code = process::wait(self.handles.process, millis)?;
        self.phase = Phase::Reaped(code);
        Ok(code)
    }

    fn stop(&mut self) -> Result<(), JobError> {
        if matches!(self.phase, Phase::Reaped(_)) {
            return Ok(());
        }
        let start = Instant::now();
        self.phase = Phase::Blocked;
        // An unavailable poll is not exit evidence; termination and final wait remain required.
        if self.wait(0).is_ok() {
            return Ok(());
        }
        process::terminate(self.handles.process)?;
        let remaining = BUDGET
            .checked_sub(start.elapsed())
            .ok_or(JobError::Unavailable)?;
        self.wait(remaining.as_millis() as u32)?;
        Ok(())
    }
}

impl Drop for Child<'_> {
    fn drop(&mut self) {
        // Synthetic harness boundary: never silently abandon a created subprocess.
        self.stop()
            .expect("synthetic child termination/wait unavailable");
    }
}

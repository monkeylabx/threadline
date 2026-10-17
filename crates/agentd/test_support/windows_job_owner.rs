//! Private empty-job experiment; never compiled into the product library/binary.

#[path = "windows_job_api.rs"]
mod api;
#[path = "windows_job_cases.rs"]
mod cases;
#[path = "windows_job_faults.rs"]
mod faults;
#[path = "windows_launch_owner.rs"]
mod launch;

use std::{ffi::c_void, ptr::NonNull};
use windows_sys::Win32::Foundation::HANDLE;

#[derive(Debug, PartialEq, Eq)]
enum JobError {
    Rejected,
    Unavailable,
}

#[derive(Debug, PartialEq, Eq)]
struct JobProfile {
    process_limit: u32,
    limits_processes: bool,
    kills_on_close: bool,
    allows_breakaway: bool,
    inheritable: bool,
}

struct Job {
    handle: Option<NonNull<c_void>>,
}

impl Job {
    fn new() -> Result<Self, JobError> {
        Self::create_with(api::configure)
    }

    fn create_with(
        configure: impl FnOnce(HANDLE) -> Result<(), JobError>,
    ) -> Result<Self, JobError> {
        let handle = api::create()?;
        let mut job = Self {
            handle: Some(handle),
        };
        if let Err(error) = configure(handle.as_ptr()) {
            job.close()?;
            return Err(error);
        }
        Ok(job)
    }

    fn inspect(&self) -> Result<JobProfile, JobError> {
        self.inspect_with(api::inspect)
    }

    fn inspect_with(
        &self,
        inspect: impl FnOnce(HANDLE) -> Result<JobProfile, JobError>,
    ) -> Result<JobProfile, JobError> {
        inspect(self.handle.ok_or(JobError::Rejected)?.as_ptr())
    }

    fn close(&mut self) -> Result<(), JobError> {
        self.close_with(api::close)
    }

    fn close_with(
        &mut self,
        close: impl FnOnce(HANDLE) -> Result<(), JobError>,
    ) -> Result<(), JobError> {
        let Some(handle) = self.handle else {
            return Ok(());
        };
        close(handle.as_ptr())?;
        self.handle = None;
        Ok(())
    }
}

impl Drop for Job {
    fn drop(&mut self) {
        // Synthetic harness boundary: fallback failure must fail the test visibly.
        // Dropping a reference supplies no process-cleanup observation or receipt.
        self.close()
            .expect("synthetic job reference release unavailable");
    }
}

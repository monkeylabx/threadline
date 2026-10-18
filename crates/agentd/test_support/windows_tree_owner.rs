#[path = "windows_tree_api.rs"]
mod api;
#[path = "windows_tree_cases.rs"]
mod cases;
#[path = "windows_tree_fixture.rs"]
mod fixture;
#[path = "windows_tree_pipes.rs"]
mod pipes;
#[path = "windows_tree_reader.rs"]
mod reader;
#[path = "windows_tree_startup.rs"]
mod startup;
use super::{job_api, process, Job, JobError};
use api::Reference;
use std::time::{Duration, Instant};
use windows_sys::Win32::Foundation::HANDLE;
pub(super) const ROOT: &str = "probe::launch::tree::fixture::windows_tree_root_entry";
const LEAF: &str = "probe::launch::tree::fixture::windows_tree_descendant_entry";

struct Tree<'job> {
    job: &'job Job,
    descendant: Option<Reference>,
}
impl<'job> Tree<'job> {
    fn new(job: &'job Job) -> Result<Self, JobError> {
        let descendant = startup::retain(job)?;
        Ok(Self {
            job,
            descendant: Some(descendant),
        })
    }

    fn cleanup(&mut self) -> Result<u32, JobError> {
        self.cleanup_with(api::terminate, process::wait, Reference::close)
    }

    fn cleanup_with(
        &mut self,
        terminate: impl FnOnce(HANDLE) -> Result<(), JobError>,
        observe: impl FnOnce(HANDLE, u32) -> Result<u32, JobError>,
        close: impl FnOnce(&mut Reference) -> Result<(), JobError>,
    ) -> Result<u32, JobError> {
        let start = Instant::now();
        let job = self.job.handle.ok_or(JobError::Rejected)?.as_ptr();
        if let Some(descendant) = &mut self.descendant {
            terminate(job)?;
            let remaining = Duration::from_secs(10)
                .checked_sub(start.elapsed())
                .ok_or(JobError::Unavailable)?;
            let code = observe(descendant.0, remaining.as_millis() as u32)?;
            if code != 99 {
                return Err(JobError::Unavailable);
            }
            close(descendant)?;
            self.descendant = None;
        }
        api::active(job) // A raw observation, never SealedAndEmpty or durable cancellation.
    }
}
impl Drop for Tree<'_> {
    fn drop(&mut self) {
        assert_eq!(self.cleanup(), Ok(0), "synthetic tree cleanup unavailable");
    }
}

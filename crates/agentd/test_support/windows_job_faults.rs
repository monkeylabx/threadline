use super::{api, Job, JobError};
use windows_sys::Win32::System::Threading::{GetCurrentProcess, GetProcessHandleCount};

#[test]
fn windows_job_configuration_failure_releases_the_new_owner() {
    let warmup = Job::new().expect("initialize native job APIs");
    let with_warmup = handle_count().expect("live warmup reference");
    drop(warmup);
    let before = handle_count().expect("serial harness handle accounting");
    assert_eq!(
        before + 1,
        with_warmup,
        "Drop must release its sole reference"
    );
    let result = Job::create_with(|_| Err(JobError::Rejected)); // Synthetic configuration fault.
    assert!(matches!(result, Err(JobError::Rejected)));
    assert_eq!(handle_count().expect("post-failure accounting"), before);
}

#[test]
fn windows_job_query_failure_preserves_explicit_release() {
    let mut job = Job::new().expect("empty synthetic job");
    assert_eq!(
        job.inspect_with(|_| Err(JobError::Unavailable)),
        Err(JobError::Unavailable)
    );
    assert!(job.inspect().is_ok());
    job.close().expect("release after synthetic query fault");
}

#[test]
fn windows_job_close_failure_retains_ownership_until_explicit_retry() {
    let mut job = Job::new().expect("empty synthetic job");
    assert_eq!(
        job.close_with(|_| Err(JobError::Unavailable)),
        Err(JobError::Unavailable)
    );
    assert!(job.inspect().is_ok());
    job.close()
        .expect("explicit retry after synthetic release fault");
    assert_eq!(job.inspect(), Err(JobError::Rejected));
}

#[test]
fn windows_job_api_status_failures_are_coarse() {
    assert_eq!(api::checked(0), Err(JobError::Unavailable));
    assert_eq!(api::checked(1), Ok(()));
    assert_eq!(api::checked(-1), Ok(()));
    assert_eq!(
        api::created(std::ptr::null_mut()),
        Err(JobError::Unavailable)
    );
}

fn handle_count() -> Result<u32, JobError> {
    let mut count = 0;
    // SAFETY: current-process pseudo-handle is queried, never owned/closed; output is writable.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #245")]
    let status = unsafe { GetProcessHandleCount(GetCurrentProcess(), &mut count) };
    api::checked(status)?;
    Ok(count)
}

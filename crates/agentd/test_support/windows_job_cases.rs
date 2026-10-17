use super::{Job, JobError, JobProfile};

#[test]
fn windows_job_profile_is_bounded_and_non_inheritable() {
    let mut job = Job::new().expect("empty synthetic job");
    assert_eq!(
        job.inspect(),
        Ok(JobProfile {
            process_limit: 8,
            limits_processes: true,
            kills_on_close: true,
            allows_breakaway: false,
            inheritable: false,
        })
    );
    job.close().expect("release synthetic reference");
}

#[test]
fn windows_jobs_are_independent_unnamed_owners() {
    let mut first = Job::new().expect("first fresh job");
    let mut second = Job::new().expect("second fresh job");
    first.close().expect("release first reference");
    assert_eq!(first.inspect(), Err(JobError::Rejected));
    assert_eq!(
        second
            .inspect()
            .expect("independent reference")
            .process_limit,
        8
    );
    second.close().expect("release second reference");
}

#[test]
fn windows_job_close_is_terminal_and_idempotent() {
    let mut job = Job::new().expect("empty synthetic job");
    job.close().expect("release reference");
    assert_eq!(job.close_with(|_| panic!("second close effect")), Ok(()));
    assert_eq!(
        job.inspect_with(|_| panic!("closed query effect")),
        Err(JobError::Rejected)
    );
}

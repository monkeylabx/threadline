use super::{cases::assert_blocked, process, Child, Job, JobError};

#[test]
fn windows_launch_creation_failure_preserves_job() {
    let job = Job::new().expect("configured job");
    let result = Child::create_with(&job, |_| Err(JobError::Unavailable));
    assert!(matches!(result, Err(JobError::Unavailable)));
    assert!(job.inspect().is_ok());
    let mut child = Child::new(&job).expect("setup released; original Job remains usable");
    child.stop().expect("fixed fixture cleanup");
}

#[test]
fn windows_launch_membership_refusal_never_resumes() {
    let job = Job::new().expect("configured job");
    let mut child = Child::new(&job).expect("suspended child");
    assert_eq!(
        child.resume_with(|_, _| Ok(false), |_| panic!("resume outside Job")),
        Err(JobError::Rejected)
    );
    assert_blocked(&mut child);
}

#[test]
fn windows_launch_query_failure_never_resumes() {
    let job = Job::new().expect("configured job");
    let mut child = Child::new(&job).expect("suspended child");
    assert_eq!(
        child.resume_with(
            |_, _| Err(JobError::Unavailable),
            |_| panic!("resume without proof")
        ),
        Err(JobError::Unavailable)
    );
    assert_blocked(&mut child);
}

#[test]
fn windows_launch_resume_failure_is_terminal() {
    assert_eq!(process::verified_resume(1), Ok(()));
    for prior in [0, 2, u32::MAX] {
        assert_eq!(process::verified_resume(prior), Err(JobError::Unavailable));
    }
    let job = Job::new().expect("configured job");
    let mut child = Child::new(&job).expect("suspended child");
    assert_eq!(
        child.resume_with(process::member, |_| Err(JobError::Unavailable)),
        Err(JobError::Unavailable)
    );
    assert_blocked(&mut child);
}

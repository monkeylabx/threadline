use super::{process, Child, Job, JobError, CHILD};

#[test]
fn windows_launch_fixture_entry() {
    // Only the fixed subprocess invocation selects this exact case; ordinary suite is inert.
    if std::env::args().skip(1).take(2).eq(["--exact", CHILD]) {
        std::process::exit(42);
    }
}

#[test]
fn windows_launch_is_suspended_and_in_job_before_resume() {
    let job = Job::new().expect("configured job");
    let mut child = Child::new(&job).expect("creation-time Job association");
    assert!(process::member(
        child.handles.process,
        job.handle.expect("live job").as_ptr()
    )
    .expect("native membership"));
    assert_eq!(child.wait(0), Err(JobError::Unavailable));
    child
        .resume()
        .expect("native prior suspend count exactly one");
    assert_eq!(process::wait(child.handles.process, 10_000), Ok(42));
    assert_eq!(
        child.resume_with(|_, _| panic!("terminal query"), |_| panic!("second resume")),
        Err(JobError::Rejected)
    );
    child
        .stop()
        .expect("observe already-exited fixture and latch reaped");
    assert_eq!(child.wait(0), Ok(42));
    child.stop().expect("idempotent stop");
}

#[test]
fn windows_launch_rejects_a_closed_job() {
    let mut job = Job::new().expect("configured job");
    job.close().expect("closed before launch");
    let result = Child::create_with(&job, |_| panic!("creation after closed Job"));
    assert!(matches!(result, Err(JobError::Rejected)));
}

#[test]
fn windows_launch_pending_wait_retains_explicit_stop() {
    let job = Job::new().expect("configured job");
    let mut child = Child::new(&job).expect("suspended child");
    assert_eq!(child.wait(0), Err(JobError::Unavailable));
    child
        .stop()
        .expect("terminate and wait after pending observation");
    assert_eq!(child.wait(0), Ok(99));
    assert_eq!(child.resume(), Err(JobError::Rejected));
    child.stop().expect("terminal stop has no new effect");
}

pub(super) fn assert_blocked(child: &mut Child<'_>) {
    assert_eq!(child.wait(0), Err(JobError::Unavailable)); // Primary thread still suspended.
    assert_eq!(
        child.resume_with(
            |_, _| panic!("repeated query"),
            |_| panic!("repeated resume")
        ),
        Err(JobError::Rejected)
    );
    child
        .stop()
        .expect("retained reference permits bounded cleanup");
    assert_eq!(child.wait(0), Ok(99));
}

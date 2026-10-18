use super::{api, process, startup::Startup, Job, JobError};

#[test]
fn windows_tree_handoff_refuses_foreign_or_unverified_membership() {
    for query_fails in [false, true] {
        let actual = Job::new().expect("root's exact owned Job");
        let foreign = Job::new().expect("distinct fresh Job");
        let mut startup = Startup::new(&actual).expect("fixed suspended root");
        let result = startup.connect_with(
            &foreign,
            |process, job| {
                if query_fails {
                    Err(JobError::Unavailable)
                } else {
                    process::member(process, job)
                }
            },
            |_| panic!("acknowledgement without exact membership proof"),
        );
        let expected = if query_fails {
            JobError::Unavailable
        } else {
            JobError::Rejected
        };
        assert!(matches!(result, Err(error) if error == expected));
        let copy = startup
            .descendant
            .as_ref()
            .expect("rejected custody remains owned")
            .0;
        assert_eq!(process::wait(copy, 0), Err(JobError::Unavailable));
        api::terminate(actual.handle.expect("live actual Job").as_ptr()).expect("failure cleanup");
        assert_eq!(process::wait(copy, 10_000), Ok(99));
        assert_eq!(startup.root.wait(10_000), Ok(99));
        drop(startup); // Both exact process references close, then reader/stdio resources.
        assert_eq!(
            api::active(actual.handle.expect("retained actual Job").as_ptr()),
            Ok(0)
        );
    }
}

use super::{api, process, reader, Job, JobError, Tree};
#[test]
fn windows_tree_root_exit_leaves_a_live_owned_descendant() {
    let job = Job::new().expect("configured job");
    let mut tree = Tree::new(&job).expect("root42 with retained descendant");
    let handle = tree.descendant.as_ref().expect("retained identity").0;
    assert!(
        process::member(handle, job.handle.expect("live job").as_ptr())
            .expect("native child membership")
    );
    assert_eq!(process::wait(handle, 0), Err(JobError::Unavailable));
    assert_eq!(process::terminate(handle), Err(JobError::Unavailable));
    assert_eq!(process::wait(handle, 0), Err(JobError::Unavailable));
    assert!(api::active(job.handle.expect("job").as_ptr()).expect("live member count") > 0);
    assert_eq!(tree.cleanup(), Ok(0));
    assert!(tree.descendant.is_none());
    assert_eq!(
        tree.cleanup_with(
            |_| panic!("second termination"),
            |_, _| panic!("second wait"),
            |_| panic!("second close")
        ),
        Ok(0)
    );
}
#[test]
fn windows_tree_failed_termination_retains_explicit_retry() {
    #[derive(Clone, Copy, PartialEq)]
    enum Fault {
        Terminate,
        Wait,
        WrongExit,
        Close,
    }
    for fault in [
        Fault::Terminate,
        Fault::Wait,
        Fault::WrongExit,
        Fault::Close,
    ] {
        let job = Job::new().expect("configured job");
        let mut tree = Tree::new(&job).expect("retained live descendant");
        let result = tree.cleanup_with(
            |job| {
                if fault == Fault::Terminate {
                    Err(JobError::Unavailable)
                } else {
                    api::terminate(job)
                }
            },
            |handle, millis| match fault {
                Fault::Wait => Err(JobError::Unavailable),
                Fault::WrongExit => Ok(77),
                _ => process::wait(handle, millis),
            },
            |reference| {
                if fault == Fault::Close {
                    reference.close_with(|_| Err(JobError::Unavailable))
                } else {
                    reference.close()
                }
            },
        );
        assert_eq!(result, Err(JobError::Unavailable));
        let reference = tree.descendant.as_ref().expect("ownership retained");
        assert!(!reference.0.is_null());
        if fault == Fault::Terminate {
            assert_eq!(process::wait(reference.0, 0), Err(JobError::Unavailable));
        }
        assert_eq!(tree.cleanup(), Ok(0));
    }
}

#[test]
fn windows_tree_handoff_rejects_invalid_handles() {
    let job = Job::new().expect("configured job");
    let (ends, _, _) = super::pipes::RootEnds::create().expect("trusted fixture pipe handles");
    let mut attributes = super::super::Attributes::new(&job).expect("initialized attribute list");
    attributes.inherit(ends.handles()).expect("first allowlist");
    assert_eq!(attributes.inherit(ends.handles()), Err(JobError::Rejected));
    assert_eq!(reader::read_handle(&42_u64.to_le_bytes()[..]), Ok(42));
    for value in [0, u64::MAX] {
        assert_eq!(
            reader::read_handle(&value.to_le_bytes()[..]),
            Err(JobError::Rejected)
        );
    }
    assert_eq!(
        reader::read_handle(&[0_u8; 7][..]),
        Err(JobError::Unavailable)
    );
}

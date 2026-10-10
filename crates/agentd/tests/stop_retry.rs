mod support;

use support::{fixture, Call, Script};
use threadline_agentd::{CleanupResult, ContainmentState, StepResult, StopError};

fn repeated_failure_script() -> Script {
    Script {
        revocations: [
            Err(StopError::Unavailable),
            Err(StopError::Rejected),
            Ok(()),
        ]
        .into(),
        interruptions: [Err(StopError::Unavailable)].into(),
        terminations: [
            Err(StopError::Unavailable),
            Err(StopError::Rejected),
            Ok(()),
        ]
        .into(),
        observations: [
            Err(StopError::Unavailable),
            Ok(ContainmentState::LiveMembers),
            Ok(ContainmentState::SealedAndEmpty),
        ]
        .into(),
    }
}

#[test]
fn repeated_failure_stays_pending_until_both_revocation_and_cleanup_recover() {
    let (mut worker, trace) = fixture(repeated_failure_script());
    worker.request_stop();
    let first = worker.poll_stop();
    assert!(!first.cleanup_confirmed());
    assert_eq!(
        first.cleanup,
        CleanupResult::Unknown(StopError::Unavailable)
    );
    assert_eq!(worker.request_stop(), first);
    let second = worker.poll_stop();
    assert!(second.requested);
    assert!(!second.cleanup_confirmed());
    assert_eq!(second.revocation, StepResult::Failed(StopError::Rejected));
    assert_eq!(second.termination, StepResult::Failed(StopError::Rejected));
    assert_eq!(second.cleanup, CleanupResult::LiveMembers);
    let confirmed = worker.poll_stop();
    assert!(confirmed.requested && confirmed.cleanup_confirmed());
    assert_eq!(confirmed.revocation, StepResult::Succeeded);
    assert_eq!(confirmed.termination, StepResult::Succeeded);
    assert_eq!(
        confirmed.interruption,
        StepResult::Failed(StopError::Unavailable)
    );
    assert_eq!(worker.poll_stop(), confirmed);
    assert_eq!(
        trace.borrow().as_slice(),
        [
            Call::Revoke,
            Call::Interrupt,
            Call::Terminate,
            Call::Observe,
            Call::Revoke,
            Call::Terminate,
            Call::Observe,
            Call::Revoke,
            Call::Terminate,
            Call::Observe,
        ]
    );
}

mod support;

use support::{fixture, Call, Script};
use threadline_agentd::{CleanupResult, ContainmentState, StepResult, StopReport};

#[test]
fn polling_before_a_request_has_no_effect() {
    let (mut worker, trace) = fixture(Script::default());
    assert_eq!(worker.snapshot(), StopReport::default());
    assert_eq!(worker.poll_stop(), StopReport::default());
    assert!(!worker.snapshot().cleanup_confirmed());
    assert!(trace.borrow().is_empty());
}

#[test]
fn request_latches_before_any_driver_effect() {
    let (mut worker, trace) = fixture(Script::default());
    let receipt = worker.request_stop();
    assert!(receipt.requested);
    assert_eq!(receipt.revocation, StepResult::NotAttempted);
    assert_eq!(receipt.cleanup, CleanupResult::NotObserved);
    assert!(!receipt.cleanup_confirmed());
    assert_eq!(worker.snapshot(), receipt);
    assert!(trace.borrow().is_empty());
    let report = worker.poll_stop();
    assert!(report.cleanup_confirmed());
    assert_eq!(report.interruption, StepResult::Succeeded);
    assert_eq!(report.termination, StepResult::Succeeded);
    assert_eq!(
        trace.borrow().as_slice(),
        [
            Call::Revoke,
            Call::Interrupt,
            Call::Terminate,
            Call::Observe
        ]
    );
}

#[test]
fn repeated_requests_and_confirmed_stop_are_idempotent() {
    let (mut worker, trace) = fixture(Script::default());
    let first = worker.request_stop();
    assert_eq!(worker.request_stop(), first);
    let confirmed = worker.poll_stop();
    assert_eq!(worker.request_stop(), confirmed);
    assert_eq!(worker.poll_stop(), confirmed);
    assert_eq!(worker.snapshot(), confirmed);
    assert_eq!(trace.borrow().len(), 4);
}

#[test]
fn parent_exit_and_cancel_ack_do_not_confirm_a_surviving_child() {
    // The fake parent has acknowledged cancel and termination, but its child lives.
    let script = Script {
        terminations: [Ok(()), Ok(())].into(),
        observations: [
            Ok(ContainmentState::LiveMembers),
            Ok(ContainmentState::SealedAndEmpty),
        ]
        .into(),
        ..Script::default()
    };
    let (mut worker, trace) = fixture(script);
    worker.request_stop();
    let pending = worker.poll_stop();
    assert_eq!(pending.interruption, StepResult::Succeeded);
    assert_eq!(pending.termination, StepResult::Succeeded);
    assert_eq!(pending.cleanup, CleanupResult::LiveMembers);
    assert!(!pending.cleanup_confirmed());
    assert_eq!(worker.request_stop(), pending);
    assert!(worker.poll_stop().cleanup_confirmed());
    assert_eq!(
        trace.borrow().as_slice(),
        [
            Call::Revoke,
            Call::Interrupt,
            Call::Terminate,
            Call::Observe,
            Call::Terminate,
            Call::Observe
        ]
    );
}

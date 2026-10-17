mod support;

use support::{fixture, Call, Script};
use threadline_agentd::{CleanupResult, ContainmentState, StepResult, StopError};

#[test]
fn failures_never_skip_containment_or_substitute_for_proof() {
    for revoke in [Ok(()), Err(StopError::Unavailable)] {
        for interrupt in [Ok(()), Err(StopError::Rejected)] {
            for terminate in [Ok(()), Err(StopError::Unavailable)] {
                check_failure_case(revoke, interrupt, terminate);
            }
        }
    }
}

fn check_failure_case(
    revoke: Result<(), StopError>,
    interrupt: Result<(), StopError>,
    terminate: Result<(), StopError>,
) {
    for observation in [
        Ok(ContainmentState::SealedAndEmpty),
        Ok(ContainmentState::LiveMembers),
        Err(StopError::Unavailable),
    ] {
        let script = Script {
            revocations: [revoke].into(),
            interruptions: [interrupt].into(),
            terminations: [terminate].into(),
            observations: [observation].into(),
        };
        let (mut worker, trace) = fixture(script);
        worker.request_stop();
        let report = worker.poll_stop();
        assert!(report.requested);
        assert_eq!(report.revocation, StepResult::from(revoke));
        assert_eq!(report.interruption, StepResult::from(interrupt));
        assert_eq!(report.termination, StepResult::from(terminate));
        assert_eq!(report.cleanup, CleanupResult::from(observation));
        assert_eq!(
            report.cleanup_confirmed(),
            revoke.is_ok() && observation == Ok(ContainmentState::SealedAndEmpty)
        );
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
}

#[test]
fn failed_revocation_is_retried_without_repeating_verified_cleanup() {
    let script = Script {
        revocations: [Err(StopError::Unavailable), Ok(())].into(),
        ..Script::default()
    };
    let (mut worker, trace) = fixture(script);
    worker.request_stop();
    let pending = worker.poll_stop();
    assert_eq!(pending.cleanup, CleanupResult::SealedAndEmpty);
    assert!(!pending.cleanup_confirmed());
    assert!(worker.poll_stop().cleanup_confirmed());
    assert_eq!(
        trace.borrow().as_slice(),
        [
            Call::Revoke,
            Call::Interrupt,
            Call::Terminate,
            Call::Observe,
            Call::Revoke
        ]
    );
}

#[test]
fn failed_interrupt_is_retained_and_not_replayed_during_cleanup_retry() {
    let script = Script {
        interruptions: [Err(StopError::Rejected)].into(),
        terminations: [Ok(()), Ok(())].into(),
        observations: [
            Err(StopError::Unavailable),
            Ok(ContainmentState::SealedAndEmpty),
        ]
        .into(),
        ..Script::default()
    };
    let (mut worker, trace) = fixture(script);
    worker.request_stop();
    let pending = worker.poll_stop();
    assert_eq!(
        pending.cleanup,
        CleanupResult::Unknown(StopError::Unavailable)
    );
    assert!(!pending.cleanup_confirmed());
    let report = worker.poll_stop();
    assert!(report.cleanup_confirmed());
    assert_eq!(report.interruption, StepResult::Failed(StopError::Rejected));
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

#[test]
fn failed_termination_remains_visible_when_independent_proof_is_available() {
    let script = Script {
        terminations: [Err(StopError::Unavailable)].into(),
        ..Script::default()
    };
    let (mut worker, trace) = fixture(script);
    worker.request_stop();
    let report = worker.poll_stop();
    assert!(report.cleanup_confirmed());
    assert_eq!(
        report.termination,
        StepResult::Failed(StopError::Unavailable)
    );
    assert_eq!(worker.poll_stop(), report);
    assert_eq!(trace.borrow().len(), 4);
}

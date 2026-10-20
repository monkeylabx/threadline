use super::{cleanup_driver::Driver, core::Core, flow::owner};
use crate::{
    local_interface::{
        tests::authority::Source, CleanupConclusion as Conclusion, InputError as Error, Shutdown,
    },
    ContainmentState, StopError,
};
use serde_json::{json, Value};

#[test]
fn cleanup_failure_reports_failure_and_retry_never_reopens_dispatch() {
    for (revoke, terminate, observation) in [
        (true, false, Ok(ContainmentState::SealedAndEmpty)),
        (false, true, Ok(ContainmentState::LiveMembers)),
        (false, false, Err(StopError::Unavailable)),
    ] {
        let source = Source::base();
        let mut owner = owner();
        let driver = Driver {
            fail_revocation: revoke,
            fail_termination: terminate,
            observations: [observation, Ok(ContainmentState::SealedAndEmpty)].into(),
            ..Driver::default()
        };
        let state = driver.state.clone();
        let mut shutdown = Shutdown::new(&mut owner, driver);
        shutdown.request(&source).expect("receipt");
        assert!(!shutdown.poll().cleanup_confirmed());
        let mut core = Core::default();
        assert_eq!(shutdown.report(&source, &mut core), Err(Error::Unavailable));
        core.available = true;
        let pending = shutdown
            .report(&source, &mut core)
            .expect("failure pending report");
        assert_eq!(pending.conclusion, Conclusion::Failed);
        assert!(!pending.acknowledged);
        assert_eq!(core.terminal, None);
        core.acknowledge = true;
        assert!(
            shutdown
                .report(&source, &mut core)
                .expect("reported failure")
                .acknowledged
        );
        assert_eq!(core.terminal, Some(("run-a".into(), Conclusion::Failed)));
        let cleaned = shutdown.poll();
        assert!(cleaned.cleanup_confirmed());
        assert!(!state.borrow().queued && !state.borrow().pending_approval);
        assert_eq!(shutdown.poll(), cleaned);
        assert_eq!(
            shutdown.report(&source, &mut core),
            Err(Error::InvalidStateTransition)
        );
        assert_eq!(core.terminal, Some(("run-a".into(), Conclusion::Failed)));
    }
}

#[test]
fn live_descendants_are_pending_even_after_successful_interrupt_and_terminate() {
    let source = Source::base();
    let mut owner = owner();
    let driver = Driver {
        observations: [
            Ok(ContainmentState::LiveMembers),
            Ok(ContainmentState::SealedAndEmpty),
        ]
        .into(),
        ..Driver::default()
    };
    let mut shutdown = Shutdown::new(&mut owner, driver);
    shutdown.request(&source).expect("receipt");
    assert!(!shutdown.poll().cleanup_confirmed());
    let mut core = Core {
        available: true,
        acknowledge: true,
        ..Core::default()
    };
    assert_eq!(
        shutdown.report(&source, &mut core),
        Err(Error::InvalidStateTransition)
    );
    assert_eq!(core.calls, 0);
    assert!(shutdown.poll().cleanup_confirmed());
    assert!(
        shutdown
            .report(&source, &mut core)
            .expect("verified proof")
            .acknowledged
    );
}

#[test]
fn current_owner_lease_and_fencing_are_required_for_reporting_not_ui_identity() {
    let source = Source::base();
    let mut owner = owner();
    let mut shutdown = Shutdown::new(&mut owner, Driver::default());
    shutdown.request(&source).expect("receipt");
    assert!(shutdown.poll().cleanup_confirmed());
    let mut core = Core {
        available: true,
        acknowledge: true,
        ..Core::default()
    };
    for (field, value, error) in [
        ("id", json!("other-run"), Error::PermissionDenied),
        ("owner", json!(false), Error::NotExecutionOwner),
        ("lease", json!(false), Error::LeaseLost),
        ("fencing", json!(false), Error::FencingTokenStale),
    ] {
        let mut changed = Source::base();
        changed.facts["run"][field] = value;
        assert_eq!(shutdown.report(&changed, &mut core), Err(error));
    }
    for (facts, error) in [
        (Value::Null, Error::PermissionDenied),
        (json!({"available":false}), Error::Unavailable),
    ] {
        let mut changed = Source::base();
        changed.facts["run"] = facts;
        assert_eq!(shutdown.report(&changed, &mut core), Err(error));
    }
    assert_eq!(core.calls, 0);
    let mut host = Source::base();
    host.facts["principal"] = Value::Null;
    host.facts["run"]["control"] = json!(false);
    host.facts["run"]["grant"] = json!(false);
    assert!(
        shutdown
            .report(&host, &mut core)
            .expect("owner path without UI")
            .acknowledged
    );
}

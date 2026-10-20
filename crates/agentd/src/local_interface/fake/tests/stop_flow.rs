use super::{cleanup_driver::Driver, core::Core, flow::owner, io, run, view, wire};
use crate::local_interface::{
    tests::{authority::Source, fixtures::request},
    CleanupConclusion, FakeRun, InputError as Error, Shutdown,
};
use serde_json::json;

fn acknowledged(shutdown: &Shutdown<'_, Driver>, source: &Source) {
    let mut core = Core::default();
    assert_eq!(shutdown.report(source, &mut core), Err(Error::Unavailable));
    core.available = true;
    let pending = shutdown.report(source, &mut core).expect("pending");
    assert!(!pending.acknowledged);
    assert_eq!(core.terminal, None);
    core.acknowledge = true;
    let ack = shutdown
        .report(source, &mut core)
        .expect("Core acknowledgement");
    assert_eq!(ack.conclusion, CleanupConclusion::Verified);
    assert!(ack.acknowledged);
    assert_eq!(
        core.terminal,
        Some(("run-a".into(), CleanupConclusion::Verified))
    );
    assert_eq!(shutdown.report(source, &mut core), Ok(ack));
}

pub(super) fn verify_stop(owner: &mut FakeRun, source: &Source) {
    let mut shutdown = Shutdown::new(owner, Driver::default());
    let receipt = shutdown
        .request(source)
        .expect("stop after cancelled/unknown turn");
    assert!(!receipt.cleanup_confirmed());
    let mut core = Core::default();
    assert_eq!(
        shutdown.report(source, &mut core),
        Err(Error::InvalidStateTransition)
    );
    assert_eq!(core.calls, 0);
    assert!(shutdown.poll().cleanup_confirmed());
}

pub(super) async fn fenced(owner: &mut FakeRun) {
    let source = Source::base();
    assert!(view::text(owner, &source).is_empty());
    for input in [
        request("input-a", "original"),
        request("late-approval", "allow"),
    ] {
        assert_eq!(
            owner.submit(&source, &input),
            Err(Error::RunInputOutcomeUncertain)
        );
    }
    let (result, bytes) = io::drive_reply(owner, &source, &io::valid()).await;
    assert_eq!(result, Err(Error::InvalidStateTransition));
    assert!(bytes.is_empty());
}

#[test]
fn turn_activity_stop_cleanup_and_exact_core_ack_are_distinct() {
    run(async {
        let source = Source::base();
        let mut owner = owner();
        owner
            .submit(&source, &request("input-a", "original"))
            .expect("intake");
        let (result, bytes) = io::drive_reply(&mut owner, &source, &io::valid()).await;
        assert_eq!(result, Ok(()));
        wire::assert_turn(&bytes, "original");
        assert_eq!(view::text(&mut owner, &source), "safe response");
        owner
            .submit(&source, &request("input-b", "queued"))
            .expect("queued intake");
        let driver = Driver::default();
        let state = driver.state.clone();
        let mut core = Core::default();
        let mut shutdown = Shutdown::new(&mut owner, driver);
        assert!(!shutdown.poll().requested);
        assert_eq!(
            shutdown.report(&source, &mut core),
            Err(Error::InvalidStateTransition)
        );
        let receipt = shutdown.request(&source).expect("stop receipt");
        assert!(receipt.requested);
        assert!(!receipt.cleanup_confirmed());
        assert_eq!(shutdown.request(&source), Ok(receipt));
        assert!(state.borrow().calls.is_empty());
        assert_eq!(
            shutdown.report(&source, &mut core),
            Err(Error::InvalidStateTransition)
        );
        let cleaned = shutdown.poll();
        assert!(cleaned.cleanup_confirmed());
        assert!(!state.borrow().queued && !state.borrow().pending_approval);
        assert!(!state.borrow_mut().allow_pending());
        assert_eq!(state.borrow().synthetic_effects, 0);
        assert_eq!(
            state.borrow().calls,
            ["revoke", "interrupt", "terminate", "observe"]
        );
        acknowledged(&shutdown, &source);
        assert_eq!(shutdown.request(&source), Ok(cleaned));
        assert_eq!(shutdown.poll(), cleaned);
        assert_eq!(state.borrow().calls.len(), 4);
        drop(shutdown);
        fenced(&mut owner).await;
    });
}

#[test]
fn stop_and_cleanup_survive_effect_and_ui_authority_loss() {
    run(async {
        let mut source = Source::base();
        let mut owner = owner();
        owner
            .submit(&source, &request("input-a", "original"))
            .expect("intake");
        for field in ["grant", "lease", "fencing", "route", "budget", "live"] {
            source.facts["run"][field] = json!(false);
        }
        let mut shutdown = Shutdown::new(&mut owner, Driver::default());
        assert!(
            shutdown
                .request(&source)
                .expect("stop despite lost effects")
                .requested
        );
        source.facts["principal"]["present"] = json!(false);
        source.facts["run"]["owner"] = json!(false);
        source.forbid_lookup = true;
        assert!(shutdown.poll().cleanup_confirmed());
        drop(shutdown);
        fenced(&mut owner).await;
    });
}

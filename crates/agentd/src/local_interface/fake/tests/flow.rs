use super::{io, run, view, wire};
use crate::local_interface::{
    tests::{authority::Source, fixtures::request},
    FakeRun, InputError as Error, InputReceipt as Receipt,
};
use serde_json::json;

pub(super) fn owner() -> FakeRun {
    FakeRun::new("run-a".into(), "incarnation-a".into()).expect("trusted fake Run")
}

#[test]
fn receipt_precedes_one_exact_exchange_and_conflict_cannot_replace_pending_bytes() {
    run(async {
        let source = Source::base();
        let mut owner = owner();
        let mut input = request("input-a", "EXACT-INPUT-SENTINEL");
        assert_eq!(owner.submit(&source, &input), Ok(Receipt::Accepted));
        assert!(view::text(&mut owner, &source).is_empty());
        assert_eq!(owner.submit(&source, &input), Ok(Receipt::Duplicate));
        let original = input.clone();
        input.text = "changed bytes".into();
        assert_eq!(
            owner.submit(&source, &input),
            Err(Error::IdempotencyConflict)
        );
        assert_eq!(
            owner.submit(&source, &request("input-b", "parallel")),
            Err(Error::InvalidStateTransition)
        );
        let (result, bytes) = io::drive_reply(&mut owner, &source, &io::valid()).await;
        assert_eq!(result, Ok(()));
        wire::assert_turn(&bytes, &original.text);
        assert_eq!(view::text(&mut owner, &source), "safe response");
        assert_eq!(owner.submit(&source, &original), Ok(Receipt::Duplicate));
        let (result, bytes) = io::drive_reply(&mut owner, &source, &io::valid()).await;
        assert_eq!(result, Err(Error::InvalidStateTransition));
        assert!(bytes.is_empty());
    });
}

#[test]
fn public_fixed_fake_keeps_engine_metadata_private_and_end_turn_is_not_core_completion() {
    run(async {
        let mut source = Source::base();
        let mut owner = owner();
        assert_eq!(
            owner.submit(&source, &request("input-a", "PRIVATE-INPUT")),
            Ok(Receipt::Accepted)
        );
        drop(owner.drive(&source)); // An unpolled future makes no engine attempt.
        owner.drive(&source).await.expect("fixed fake turn");
        assert_eq!(view::text(&mut owner, &source), "Demonstration response.");
        let debug = format!("{owner:?} {}", Error::RunInputOutcomeUncertain);
        for private in [
            "PRIVATE-INPUT",
            "run-a",
            "incarnation-a",
            "private-demo-session",
            "PRIVATE-WORKER-DIAGNOSTIC",
        ] {
            assert!(!debug.contains(private));
        }
        source.facts["run"]["live"] = json!(false);
        assert_eq!(
            owner.submit(&source, &request("input-b", "later")),
            Err(Error::InvalidStateTransition)
        );
        source.facts["run"]["live"] = json!(true);
        assert_eq!(
            owner.submit(&source, &request("input-b", "later")),
            Ok(Receipt::Accepted)
        );
        let (result, bytes) = io::drive_reply(&mut owner, &source, &io::valid()).await;
        assert_eq!(result, Ok(()));
        wire::assert_turn(&bytes, "later");
    });
}

#[test]
fn denied_intake_never_creates_an_engine_and_dispatch_rechecks_current_authority() {
    run(async {
        for (section, field, value, expected) in [
            ("principal", "present", json!(false), Error::Unauthenticated),
            (
                "principal",
                "unexpired",
                json!(false),
                Error::Unauthenticated,
            ),
            (
                "binding",
                "window",
                json!("other-window"),
                Error::PermissionDenied,
            ),
        ] {
            let mut source = Source::base();
            source.facts[section][field] = value;
            source.forbid_lookup = true;
            let mut owner = owner();
            assert_eq!(
                owner.submit(&source, &request("input-a", "private")),
                Err(expected)
            );
            let (_, bytes) = io::drive_reply(&mut owner, &source, &io::valid()).await;
            assert!(bytes.is_empty());
        }
        let mut owner = owner();
        let source = Source::base();
        owner
            .submit(&source, &request("input-a", "original"))
            .expect("accepted");
        let mut revoked = Source::base();
        revoked.facts["run"]["grant"] = json!(false);
        let (result, bytes) = io::drive_reply(&mut owner, &revoked, &io::valid()).await;
        assert_eq!(result, Err(Error::GrantRevoked));
        assert!(bytes.is_empty());
        let (result, bytes) = io::drive_reply(&mut owner, &source, &io::valid()).await;
        assert_eq!(result, Ok(()));
        wire::assert_turn(&bytes, "original");
    });
}

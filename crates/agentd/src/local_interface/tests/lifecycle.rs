use super::{authority::Source, fixtures::request};
use crate::local_interface::{InputError as Error, InputReceipt as Receipt, Intake};

#[test]
fn active_turn_keeps_duplicates_and_conflicts_without_another_acceptance() {
    let source = Source::base();
    let mut intake = Intake::new("run-a".to_owned());
    assert_eq!(
        intake.accept(&source, &request("a", "first")),
        Ok(Receipt::Accepted)
    );
    assert_eq!(
        intake.accept(&source, &request("a", "first")),
        Ok(Receipt::Duplicate)
    );
    assert_eq!(
        intake.accept(&source, &request("a", "changed")),
        Err(Error::IdempotencyConflict)
    );
    assert_eq!(
        intake.accept(&source, &request("b", "second")),
        Err(Error::InvalidStateTransition)
    );
    intake.finish_turn();
    assert_eq!(
        intake.accept(&source, &request("b", "second")),
        Ok(Receipt::Accepted)
    );
}

#[test]
fn completion_and_new_ids_cannot_reset_uncertain_intake() {
    let source = Source::base();
    let mut intake = Intake::new("run-a".to_owned());
    intake.finish_turn();
    assert_eq!(
        intake.accept(&source, &request("a", "text")),
        Ok(Receipt::Accepted)
    );
    intake.mark_uncertain();
    intake.finish_turn();
    for id in ["a", "new-id"] {
        assert_eq!(
            intake.accept(&source, &request(id, "text")),
            Err(Error::RunInputOutcomeUncertain)
        );
    }
}

#[test]
fn trusted_clear_removes_content_and_permanently_fences_replay() {
    let source = Source::base();
    let mut intake = Intake::new("run-a".to_owned());
    assert_eq!(
        intake.accept(&source, &request("a", "private-text")),
        Ok(Receipt::Accepted)
    );
    intake.clear();
    intake.clear();
    intake.finish_turn();
    assert!(format!("{intake:?}").contains("accepted_inputs: 0"));
    assert_eq!(
        intake.accept(&source, &request("a", "private-text")),
        Err(Error::RunInputOutcomeUncertain)
    );
}

#[test]
fn input_id_and_utf8_limits_are_exact() {
    let source = Source::base();
    for (id, text, expected) in [
        ("!".repeat(128), "é".repeat(32768), Ok(Receipt::Accepted)),
        (
            "!".repeat(129),
            "text".to_owned(),
            Err(Error::InvalidArgument),
        ),
        (
            "é".to_owned(),
            "text".to_owned(),
            Err(Error::InvalidArgument),
        ),
        (
            "a\0b".to_owned(),
            "text".to_owned(),
            Err(Error::InvalidArgument),
        ),
        (
            "".to_owned(),
            "text".to_owned(),
            Err(Error::InvalidArgument),
        ),
    ] {
        let mut intake = Intake::new("run-a".to_owned());
        assert_eq!(intake.accept(&source, &request(&id, &text)), expected);
    }
}

#[test]
fn debug_and_errors_never_include_input_or_identifiers() {
    let source = Source::base();
    let mut intake = Intake::new("run-a".to_owned());
    let input = request("secret-id", "secret-payload");
    assert_eq!(intake.accept(&source, &input), Ok(Receipt::Accepted));
    let debug = format!("{intake:?} {input:?} {}", Error::PermissionDenied);
    for value in ["secret-id", "secret-payload", "run-a"] {
        assert!(!debug.contains(value));
    }
}

use super::{authority::Source, fixtures::request};
use crate::local_interface::{InputError as Error, InputReceipt as Receipt, Intake};
use serde_json::{json, Value};

#[test]
fn invalid_principal_facts_never_read_a_run() {
    let failures = [
        ("present", json!(false)),
        ("active", json!(false)),
        ("unexpired", json!(false)),
        ("tenant", json!("")),
        ("actor", json!("")),
        ("device", json!("")),
        ("session", Value::Null),
        ("session", json!("")),
        ("window", Value::Null),
        ("window", json!("")),
    ];
    for (field, value) in failures {
        let mut source = Source::base();
        source.facts["principal"][field] = value;
        source.forbid_lookup = true;
        let mut intake = Intake::new("run-a".to_owned());
        assert_eq!(
            intake.accept(&source, &request("a", "text")),
            Err(Error::Unauthenticated)
        );
        assert_eq!(source.lookups.get(), 0);
    }
}

#[test]
fn peer_and_host_binding_failures_precede_run_lookup() {
    for (field, value, error) in [
        ("peerAuthenticated", json!(false), Error::Unauthenticated),
        ("session", json!("other"), Error::PermissionDenied),
        ("window", json!("other"), Error::PermissionDenied),
    ] {
        let mut source = Source::base();
        source.facts["binding"][field] = value;
        source.forbid_lookup = true;
        let mut intake = Intake::new("run-a".to_owned());
        assert_eq!(intake.accept(&source, &request("a", "text")), Err(error));
        assert_eq!(source.lookups.get(), 0);
    }
}

#[test]
fn missing_and_unavailable_run_facts_fail_closed() {
    for (run, error) in [
        (Value::Null, Error::PermissionDenied),
        (json!({"available": false}), Error::Unavailable),
    ] {
        let mut source = Source::base();
        source.facts["run"] = run;
        let mut intake = Intake::new("run-a".to_owned());
        assert_eq!(intake.accept(&source, &request("a", "text")), Err(error));
        assert_eq!(source.lookups.get(), 1);
    }
}

#[test]
fn empty_run_and_a_different_intake_run_are_denied() {
    let mut source = Source::base();
    source.forbid_lookup = true;
    let mut empty = request("a", "text");
    empty.run_id.clear();
    let mut intake = Intake::new("run-a".to_owned());
    assert_eq!(intake.accept(&source, &empty), Err(Error::PermissionDenied));
    source.forbid_lookup = false;
    let mut other = Intake::new("run-b".to_owned());
    assert_eq!(
        other.accept(&source, &request("a", "text")),
        Err(Error::PermissionDenied)
    );
}

#[test]
fn duplicates_recheck_authority_and_denied_calls_preserve_receipts() {
    let mut source = Source::base();
    let input = request("a", "text");
    let mut intake = Intake::new("run-a".to_owned());
    assert_eq!(intake.accept(&source, &input), Ok(Receipt::Accepted));
    source.facts["run"]["grant"] = json!(false);
    assert_eq!(intake.accept(&source, &input), Err(Error::GrantRevoked));
    source.facts["run"]["grant"] = json!(true);
    source.facts["binding"]["window"] = json!("other");
    assert_eq!(intake.accept(&source, &input), Err(Error::PermissionDenied));
    source.facts["binding"]["window"] = json!("window-a");
    assert_eq!(intake.accept(&source, &input), Ok(Receipt::Duplicate));
    assert_eq!(source.lookups.get(), 3);
}

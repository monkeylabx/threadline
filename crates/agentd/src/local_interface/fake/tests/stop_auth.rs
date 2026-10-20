use super::{cleanup_driver::Driver, core::Core, flow::owner, io, run, view, wire};
use crate::local_interface::{
    tests::{
        authority::Source,
        fixtures::{expected, fixture, materialize, request},
    },
    ActivityUpdate, InputError as Error, Shutdown,
};
use serde_json::{json, Value};

async fn denied(source: Source, error: Error) {
    let trusted = Source::base();
    let mut owner = owner();
    owner
        .submit(&trusted, &request("input-a", "original"))
        .expect("intake");
    owner
        .activity
        .append("input-a", ActivityUpdate::Text("preserved"))
        .expect("prior view");
    let driver = Driver::default();
    let state = driver.state.clone();
    let mut shutdown = Shutdown::new(&mut owner, driver);
    assert_eq!(shutdown.request(&source), Err(error));
    assert!(!shutdown.poll().requested);
    assert_eq!(
        shutdown.report(&trusted, &mut Core::default()),
        Err(Error::InvalidStateTransition)
    );
    assert!(state.borrow().calls.is_empty());
    drop(shutdown);
    assert!(!owner.effect_fence().closed());
    assert_eq!(view::text(&mut owner, &trusted), "preserved");
    let (result, bytes) = io::drive_reply(&mut owner, &trusted, &io::valid()).await;
    assert_eq!(result, Ok(()));
    wire::assert_turn(&bytes, "original");
}

#[test]
fn canonical_stop_decisions_and_denial_preservation() {
    run(async {
        let fixture = fixture();
        for case in fixture["cases"].as_array().expect("C1 cases") {
            if case["operation"] != "stop" {
                continue;
            }
            let source = Source::new(materialize(&fixture["base"], case));
            if case["expected"] == "STOP_RECEIPT" {
                assert!(
                    Shutdown::new(&mut owner(), Driver::default())
                        .request(&source)
                        .expect("receipt")
                        .requested
                );
            } else {
                denied(
                    source,
                    expected(case["expected"].as_str().expect("status")).expect_err("denial"),
                )
                .await;
            }
        }
    });
}

#[test]
fn missing_identity_and_wrong_host_never_lookup_or_destroy() {
    run(async {
        for (section, field, value, error) in [
            ("principal", "present", json!(false), Error::Unauthenticated),
            ("principal", "tenant", json!(""), Error::Unauthenticated),
            ("principal", "actor", json!(""), Error::Unauthenticated),
            ("principal", "device", json!(""), Error::Unauthenticated),
            ("principal", "session", json!(""), Error::Unauthenticated),
            ("binding", "window", json!("wrong"), Error::PermissionDenied),
        ] {
            let mut source = Source::base();
            source.facts[section][field] = value;
            source.forbid_lookup = true;
            denied(source, error).await;
        }
    });
}

#[test]
fn stop_rechecks_scoped_current_run_facts() {
    run(async {
        for (field, value, error) in [
            ("id", json!("other-run"), Error::PermissionDenied),
            ("tenant", json!("other-tenant"), Error::TenantMismatch),
            ("authorizedActor", json!("other"), Error::PermissionDenied),
            ("authorizedDevice", json!("other"), Error::PermissionDenied),
        ] {
            let mut source = Source::base();
            source.facts["run"][field] = value;
            denied(source, error).await;
        }
        for (facts, error) in [
            (Value::Null, Error::PermissionDenied),
            (json!({"available":false}), Error::Unavailable),
        ] {
            let mut source = Source::base();
            source.facts["run"] = facts;
            denied(source, error).await;
        }
    });
}

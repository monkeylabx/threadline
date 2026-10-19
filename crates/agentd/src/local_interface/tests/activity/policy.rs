use super::*;
use serde_json::json;

#[test]
fn canonical_watch_decisions_cross_owned_handle_seam() {
    let fixture = fixtures::fixture();
    let cases = fixture["cases"].as_array().expect("cases");
    let base = Source::base();
    let mut count = 0;
    for case in cases.iter().filter(|case| case["operation"] == "watch") {
        let mut activity = activity();
        append(&mut activity, "discarded-1");
        append(&mut activity, "discarded-2");
        activity.clear();
        append(&mut activity, "retained-3");
        append(&mut activity, "retained-4");
        let mut origin = activity.watch(&base, &first()).expect("origin");
        let (stream, floor, head) = ready(&mut origin, &base, &activity);
        assert_eq!((floor, head), (3, 4));
        let facts = fixtures::materialize(&fixture["base"], case);
        let requested = &facts["request"];
        let mut request = Request {
            run_id: requested["run"].as_str().expect("Run").to_owned(),
            stream_id: requested["stream"].as_str().expect("stream").to_owned(),
            after_sequence: requested["after"].as_u64().expect("cursor"),
        };
        if request.stream_id == "stream-a" {
            request.stream_id = stream;
        }
        let source = Source::new(facts);
        let result = if request.stream_id.is_empty() {
            activity.watch(&source, &request)
        } else {
            origin.reconnect(&source, &activity, &request)
        };
        let expected = case["expected"].as_str().expect("status");
        if expected == "WATCH" {
            let mut watch = result.expect("authorized watch");
            assert_eq!(ready(&mut watch, &source, &activity).1, 3);
            assert_eq!(frame(&mut watch, &source, &activity).sequence, 3);
        } else {
            assert_eq!(
                result.err(),
                fixtures::expected(expected).err(),
                "{}",
                case["id"]
            );
        }
        count += 1;
    }
    assert_eq!(count, 24);
}

#[test]
fn observer_does_not_need_execution_permissions() {
    let mut source = Source::base();
    for field in [
        "control", "owner", "lease", "fencing", "route", "budget", "live",
    ] {
        source.facts["run"][field] = json!(false);
    }
    let mut activity = activity();
    append(&mut activity, "observed");
    let mut watch = activity.watch(&source, &first()).expect("observer");
    ready(&mut watch, &source, &activity);
    assert_eq!(
        frame(&mut watch, &source, &activity).view,
        Some(View::AgentText("observed".into()))
    );
}

#[test]
fn every_emission_rechecks_authority_and_failure_ends_handle() {
    let base = Source::base();
    for emit_ready in [true, false] {
        for (section, field, value, expected) in [
            ("run", "observe", json!(false), Error::PermissionDenied),
            ("run", "grant", json!(false), Error::GrantRevoked),
            ("run", "grantExpired", json!(true), Error::GrantExpired),
            ("run", "grantExpired", json!(null), Error::Unavailable),
            (
                "principal",
                "unexpired",
                json!(false),
                Error::Unauthenticated,
            ),
            (
                "binding",
                "peerAuthenticated",
                json!(false),
                Error::Unauthenticated,
            ),
            (
                "binding",
                "window",
                json!("window-b"),
                Error::PermissionDenied,
            ),
        ] {
            let mut activity = activity();
            append(&mut activity, "must not escape");
            let mut watch = activity.watch(&base, &first()).expect("initial observer");
            if emit_ready {
                ready(&mut watch, &base, &activity);
            }
            let mut changed = Source::base();
            changed.facts[section][field] = value;
            assert_eq!(watch.next(&changed, &activity), Err(expected));
            assert_eq!(watch.next(&base, &activity), Err(Error::CursorInvalid));
            let mut fresh = activity
                .watch(&base, &first())
                .expect("fresh authorization");
            ready(&mut fresh, &base, &activity);
            assert_eq!(frame(&mut fresh, &base, &activity).sequence, 1);
        }
    }
}

#[test]
fn identity_is_checked_before_lookup_and_incarnation_is_scoped() {
    let mut activity = activity();
    for field in ["tenant", "actor", "device", "session", "window"] {
        let mut source = Source::base();
        source.facts["principal"][field] = json!("");
        source.forbid_lookup = true;
        assert_eq!(
            activity.watch(&source, &first()).err(),
            Some(Error::Unauthenticated)
        );
    }
    let base = Source::base();
    let mut watch = activity.watch(&base, &first()).expect("watch");
    let (stream, _, _) = ready(&mut watch, &base, &activity);
    let request = Request {
        stream_id: stream.clone(),
        ..first()
    };
    let restarted = Activity::new("run-a".into(), "incarnation-b".into()).expect("new incarnation");
    assert_eq!(
        watch.reconnect(&base, &restarted, &request).err(),
        Some(Error::CursorInvalid)
    );
    let other_run = Activity::new("run-b".into(), "incarnation-a".into()).expect("other Run");
    assert_eq!(
        watch.reconnect(&base, &other_run, &request).err(),
        Some(Error::CursorInvalid)
    );
    let mut second = activity.watch(&base, &first()).expect("second watch");
    let (second_id, _, _) = ready(&mut second, &base, &activity);
    assert_eq!(stream.len(), second_id.len());
    assert_ne!(stream, second_id);
    assert_eq!(
        second.reconnect(&base, &activity, &request).err(),
        Some(Error::CursorInvalid)
    );
}

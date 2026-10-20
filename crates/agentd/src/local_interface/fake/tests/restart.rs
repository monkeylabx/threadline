use super::{cleanup_driver::Driver, flow::owner, run, stop_flow::fenced, view};
use crate::local_interface::{
    tests::{authority::Source, fixtures::request},
    FakeRun, InputError as Error, Shutdown,
};
use prost::Message;
use serde_json::json;
use threadline_client_proto::threadline::local_agent::v1::{
    WatchRunActivityRequest as WatchRequest, WatchRunActivityResponse as Frame,
};

#[test]
fn trusted_revocation_or_terminal_clears_content_and_fences_late_approval() {
    run(async {
        for (field, value) in [("grant", json!(false)), ("live", json!(false))] {
            let source = Source::base();
            let mut owner = owner();
            owner
                .submit(&source, &request("input-a", "original"))
                .expect("intake");
            owner.drive(&source).await.expect("fixed fake turn");
            assert!(!view::text(&mut owner, &source).is_empty());
            owner
                .submit(&source, &request("queued", "pending approval"))
                .expect("pending");
            let mut revoked = Source::base();
            revoked.facts["run"][field] = value;
            revoked.facts["principal"]["present"] = json!(false);
            revoked.forbid_lookup = true;
            let driver = Driver::default();
            let state = driver.state.clone();
            let fence = owner.effect_fence();
            let mut shutdown = Shutdown::new(&mut owner, driver);
            assert!(shutdown.invalidate().requested); // Trusted notification, not a UI call.
            assert_eq!(shutdown.request(&revoked), Err(Error::Unauthenticated));
            assert!(shutdown.poll().cleanup_confirmed());
            assert!(!state.borrow().queued && !state.borrow().pending_approval);
            assert!(!state.borrow_mut().allow_pending(&fence));
            assert_eq!(state.borrow().synthetic_effects, 0);
            drop(shutdown);
            fenced(&mut owner).await; // Fresh authority/late allow cannot revive it.
        }
    });
}

#[test]
fn recovery_imports_no_old_context_and_retry_requires_fresh_core_run() {
    run(async {
        let source = Source::base();
        let mut previous = owner();
        previous
            .submit(&source, &request("input-a", "original"))
            .expect("intake");
        previous.drive(&source).await.expect("old turn");
        let watch_request = WatchRequest {
            run_id: "run-a".into(),
            ..WatchRequest::default()
        };
        let mut watch = previous.watch(&source, &watch_request).expect("old handle");
        let ready = watch
            .next(&source, previous.activity())
            .expect("ready")
            .expect("frame");
        let frame = Frame::decode(ready.as_slice()).expect("canonical frame");
        let mut recovered =
            FakeRun::recover("run-a".into(), "new-incarnation".into()).expect("recovered");
        assert!(recovered.effect_fence().closed());
        assert_eq!(
            watch.next(&source, recovered.activity()),
            Err(Error::CursorInvalid)
        );
        let reconnect = WatchRequest {
            stream_id: frame.stream_id,
            ..watch_request
        };
        assert!(watch
            .reconnect(&source, recovered.activity(), &reconnect)
            .is_err());
        fenced(&mut recovered).await;
        let mut unknown = Source::base();
        unknown.facts["run"] = json!(null);
        assert_eq!(
            recovered.submit(&unknown, &request("new", "retry")),
            Err(Error::PermissionDenied)
        );
        let mut fresh_source = Source::base();
        fresh_source.facts["run"]["id"] = json!("run-fresh");
        let mut fresh = FakeRun::new("run-fresh".into(), "new-incarnation".into())
            .expect("fresh Core dispatch");
        let mut input = request("input-a", "fresh goal");
        input.run_id = "run-fresh".into();
        fresh.submit(&fresh_source, &input).expect("new Run only");
        fresh
            .drive(&fresh_source)
            .await
            .expect("fresh engine context");
        assert_eq!(
            FakeRun::recover("".into(), "new".into()).err(),
            Some(Error::InvalidArgument)
        );
    });
}

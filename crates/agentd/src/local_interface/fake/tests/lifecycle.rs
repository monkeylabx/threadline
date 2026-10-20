use super::{io, run, view, wire};
use crate::local_interface::{
    tests::{authority::Source, fixtures::request},
    FakeRun, InputError as Error,
};
use std::{future::Future, task::Poll};
use tokio::io::{AsyncReadExt, AsyncWriteExt};

fn uncertain(owner: &mut FakeRun, source: &Source) {
    assert!(view::text(owner, source).is_empty()); // A fresh watch cannot reconcile effects.
    for input in [request("input-a", "original"), request("new-id", "retry")] {
        assert_eq!(
            owner.submit(source, &input),
            Err(Error::RunInputOutcomeUncertain)
        );
    }
}

#[test]
fn failed_setup_partial_turn_and_cancelled_result_never_publish_or_replay() {
    run(async {
        let source = Source::base();
        for reply in [
            "{\"jsonrpc\":\"2.0\",\"id\":1,\"error\":{\"code\":-1,\"message\":\"PRIVATE\"}}\n"
                .to_owned(),
            format!(
                "{}{{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{{\"sessionId\":\"\"}}}}\n",
                io::INIT
            ),
            format!("{}{}", io::READY, io::CHUNK), // EOF after partial text.
            io::valid().replace("end_turn", "cancelled"),
        ] {
            let mut owner = FakeRun::new("run-a".into(), "incarnation-a".into()).expect("owner");
            owner
                .submit(&source, &request("input-a", "original"))
                .expect("accepted");
            let (result, bytes) = io::drive_reply(&mut owner, &source, &reply).await;
            assert_eq!(result, Err(Error::RunInputOutcomeUncertain));
            assert!(!bytes.is_empty());
            uncertain(&mut owner, &source);
            let (result, bytes) = io::drive_reply(&mut owner, &source, &io::valid()).await;
            assert_eq!(result, Err(Error::InvalidStateTransition));
            assert!(bytes.is_empty());
            super::stop_flow::verify_stop(&mut owner, &source);
            uncertain(&mut owner, &source);
        }
    });
}

#[test]
fn failed_client_construction_fences_the_accepted_attempt() {
    run(async {
        let source = Source::base();
        let mut owner = FakeRun::new("run-a".into(), "incarnation-a".into()).expect("owner");
        owner
            .submit(&source, &request("input-a", "original"))
            .expect("accepted");
        assert_eq!(
            owner.drive_with(&source, io::unavailable).await,
            Err(Error::RunInputOutcomeUncertain)
        );
        uncertain(&mut owner, &source);
    });
}

#[test]
fn dropped_or_timed_out_polled_exchange_latches_uncertainty() {
    run(async {
        for cancel in [true, false] {
            let source = Source::base();
            let mut owner = FakeRun::new("run-a".into(), "incarnation-a".into()).expect("owner");
            owner
                .submit(&source, &request("input-a", "original"))
                .expect("accepted");
            let (reader, mut replies) = tokio::io::duplex(4096);
            replies
                .write_all(io::READY.as_bytes())
                .await
                .expect("setup replies");
            let (writer, mut capture) = tokio::io::duplex(1024 * 1024);
            let mut attempt = Box::pin(owner.drive_with(&source, || Ok(io::pipes(reader, writer))));
            if cancel {
                std::future::poll_fn(|cx| {
                    assert!(attempt.as_mut().poll(cx).is_pending());
                    Poll::Ready(())
                })
                .await;
                drop(attempt);
            } else {
                assert_eq!(attempt.await, Err(Error::RunInputOutcomeUncertain));
            }
            drop(replies);
            let mut bytes = Vec::new();
            capture.read_to_end(&mut bytes).await.expect("outbound EOF");
            wire::assert_turn(&bytes, "original");
            uncertain(&mut owner, &source);
            super::stop_flow::verify_stop(&mut owner, &source);
            uncertain(&mut owner, &source);
        }
    });
}

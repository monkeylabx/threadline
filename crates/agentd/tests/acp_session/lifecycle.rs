use super::acp_support::run;
use super::fixtures::{assert_wire, live, workspace, DEADLINE};
use std::{
    future::{poll_fn, Future},
    task::Poll,
    time::Duration,
};
use threadline_agentd::acp::SessionError as Error;
use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt};

#[test]
fn cancelled_partial_send_and_pending_receive_never_restore_initialization() {
    run(async {
        for capacity in [1, 1024] {
            let (mut client, mut peer) = live(capacity).await;
            let prefix = peer.wire.len();
            let mut attempt = Box::pin(client.create_session(workspace(), DEADLINE));
            assert!(poll_fn(|cx| Poll::Ready(attempt.as_mut().poll(cx).is_pending())).await);
            drop(attempt);
            assert_eq!(
                client.create_session(workspace(), DEADLINE).await,
                Err(Error::Unavailable)
            );
            assert!(client.initialize().await.is_err());
            drop(client);
            peer.output
                .read_to_end(&mut peer.wire)
                .await
                .expect("outbound EOF");
            if capacity == 1 {
                assert_eq!(&peer.wire[prefix..], b"{");
            } else {
                assert_wire(&peer.wire, workspace());
            }
        }
    });
}

#[test]
fn session_deadline_includes_delayed_send_and_receive() {
    run(async {
        let (mut client, mut peer) = live(1).await;
        let server = tokio::spawn(async move {
            tokio::time::sleep(Duration::from_secs(6)).await;
            peer.output
                .read_until(b'\n', &mut peer.wire)
                .await
                .expect("Session request");
            assert_wire(&peer.wire, workspace());
            tokio::time::sleep(Duration::from_secs(6)).await;
            peer.input
                .write_all(b"{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"sessionId\":\"late\"}}\n")
                .await
                .expect("late response");
        });
        let start = tokio::time::Instant::now();
        assert_eq!(
            client.create_session(workspace(), DEADLINE).await,
            Err(Error::Timeout)
        );
        assert_eq!(start.elapsed(), DEADLINE);
        assert_eq!(
            client.create_session(workspace(), DEADLINE).await,
            Err(Error::Unavailable)
        );
        server.abort();
        assert!(server.await.expect_err("cancel fixture").is_cancelled());
    });
}

#[test]
fn stalled_session_send_is_bounded_without_retry() {
    run(async {
        let (mut client, _peer) = live(1).await;
        let start = tokio::time::Instant::now();
        assert_eq!(
            client.create_session(workspace(), DEADLINE).await,
            Err(Error::Timeout)
        );
        assert_eq!(start.elapsed(), DEADLINE);
        assert_eq!(
            client.create_session(workspace(), DEADLINE).await,
            Err(Error::Unavailable)
        );
    });
}

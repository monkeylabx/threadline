use super::{acp_support::run, fixtures::*};
use std::{
    future::{poll_fn, Future},
    task::Poll,
    time::Duration,
};
use threadline_agentd::acp::TurnError as Error;
use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt};

struct PendingAfter {
    bytes: Vec<u8>,
    position: usize,
    exhausted: std::sync::Arc<std::sync::atomic::AtomicBool>,
}

impl tokio::io::AsyncRead for PendingAfter {
    fn poll_read(
        self: std::pin::Pin<&mut Self>,
        _: &mut std::task::Context<'_>,
        buffer: &mut tokio::io::ReadBuf<'_>,
    ) -> Poll<std::io::Result<()>> {
        use std::sync::atomic::Ordering;
        let this = self.get_mut();
        if buffer.remaining() == 0 {
            return Poll::Ready(Ok(()));
        }
        if this.position == this.bytes.len() {
            this.exhausted.store(true, Ordering::SeqCst);
            return Poll::Pending;
        }
        let end = (this.position + buffer.remaining()).min(this.bytes.len());
        buffer.put_slice(&this.bytes[this.position..end]);
        this.position = end;
        Poll::Ready(Ok(()))
    }
}

#[test]
fn cancellation_after_accepted_text_discards_the_partial_turn() {
    run(async {
        use std::sync::{
            atomic::{AtomicBool, Ordering},
            Arc,
        };
        use threadline_agentd::acp::{stdio, Initialization};
        let exhausted = Arc::new(AtomicBool::new(false));
        let bytes = format!("{READY}{}", stream(&[chunk("private-partial")])).into_bytes();
        let input = PendingAfter {
            bytes,
            position: 0,
            exhausted: exhausted.clone(),
        };
        let (reader, writer) = stdio(input, tokio::io::sink(), IO_DEADLINE).expect("pipes");
        let mut client = Initialization::new(reader, writer, DEADLINE).expect("owner");
        client.initialize().await.expect("initialized");
        client
            .create_session(workspace(), DEADLINE)
            .await
            .expect("Session");
        assert!(!exhausted.load(Ordering::SeqCst));
        let mut attempt = Box::pin(client.prompt("a", DEADLINE));
        assert!(poll_fn(|cx| Poll::Ready(attempt.as_mut().poll(cx).is_pending())).await);
        assert!(
            exhausted.load(Ordering::SeqCst),
            "pending read occurs after the text update"
        );
        drop(attempt);
        assert_error(client.prompt("retry", DEADLINE).await, Error::Unavailable);
    });
}

#[test]
fn cancelled_send_or_pending_receive_never_allows_replay() {
    run(async {
        for capacity in [1, 1024] {
            let (mut client, mut peer) = live(capacity).await;
            let prefix = peer.wire.len();
            let mut attempt = Box::pin(client.prompt("a", DEADLINE));
            assert!(poll_fn(|cx| Poll::Ready(attempt.as_mut().poll(cx).is_pending())).await);
            drop(attempt);
            assert_error(client.prompt("retry", DEADLINE).await, Error::Unavailable);
            drop(client);
            peer.output
                .read_to_end(&mut peer.wire)
                .await
                .expect("outbound EOF");
            if capacity == 1 {
                assert_eq!(&peer.wire[prefix..], b"{");
            } else {
                assert_wire(&peer.wire, "a");
            }
        }
    });
}

#[test]
fn total_deadline_includes_send_and_all_updates() {
    run(async {
        let (mut client, mut peer) = live(1).await;
        let server = tokio::spawn(async move {
            tokio::time::sleep(Duration::from_secs(6)).await;
            peer.output
                .read_until(b'\n', &mut peer.wire)
                .await
                .expect("prompt");
            assert_wire(&peer.wire, "a");
            peer.input
                .write_all(stream(&[chunk("partial")]).as_bytes())
                .await
                .expect("update");
            tokio::time::sleep(Duration::from_secs(6)).await;
            peer.input
                .write_all(stream(&[terminal("end_turn")]).as_bytes())
                .await
                .expect("late end");
        });
        let start = tokio::time::Instant::now();
        assert_error(client.prompt("a", DEADLINE).await, Error::Timeout);
        assert_eq!(start.elapsed(), DEADLINE);
        assert_error(client.prompt("retry", DEADLINE).await, Error::Unavailable);
        server.abort();
        assert!(server.await.expect_err("cancel fixture").is_cancelled());
    });
}

#[test]
fn stalled_prompt_send_is_bounded() {
    run(async {
        let (mut client, _peer) = live(1).await;
        let start = tokio::time::Instant::now();
        assert_error(client.prompt("a", DEADLINE).await, Error::Timeout);
        assert_eq!(start.elapsed(), DEADLINE);
        assert_error(client.prompt("retry", DEADLINE).await, Error::Unavailable);
    });
}

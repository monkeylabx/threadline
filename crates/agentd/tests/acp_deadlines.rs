mod acp_support;

use acp_support::{run, Broken};
use serde_json::json;
use std::{
    future::{poll_fn, Future},
    task::Poll,
    time::Duration,
};
use threadline_agentd::acp::{stdio, RpcMessage, TransportError};
use tokio::io::AsyncWriteExt;

#[test]
fn stalled_reads_and_writes_timeout_without_wall_clock_assumptions() {
    run(async {
        let (stream, _peer) = tokio::io::duplex(1);
        let (mut reader, _) =
            stdio(stream, tokio::io::sink(), Duration::from_secs(1)).expect("pipes");
        assert_eq!(
            reader.receive().await.expect_err("read timeout"),
            TransportError::Timeout
        );
        let (stream, _peer) = tokio::io::duplex(1);
        let (mut reader, mut writer) =
            stdio(tokio::io::empty(), stream, Duration::from_secs(1)).expect("pipes");
        let frame = RpcMessage::new(json!({"jsonrpc":"2.0","method":"m"})).expect("frame");
        assert_eq!(writer.send(&frame).await, Err(TransportError::Timeout));
        assert_eq!(
            reader.receive().await.expect_err("shared poison"),
            TransportError::Closed
        );
    });
}

#[test]
fn dropping_a_partial_read_poisons_the_connection() {
    run(async {
        let (stream, mut peer) = tokio::io::duplex(8);
        let (mut reader, mut writer) =
            stdio(stream, tokio::io::sink(), Duration::from_secs(1)).expect("pipes");
        peer.write_all(b"{").await.expect("partial frame");
        let mut receive = Box::pin(reader.receive());
        assert!(poll_fn(|cx| Poll::Ready(receive.as_mut().poll(cx).is_pending())).await);
        drop(receive);
        let frame = RpcMessage::new(json!({"jsonrpc":"2.0","method":"m"})).expect("frame");
        assert_eq!(writer.send(&frame).await, Err(TransportError::Closed));
        assert_eq!(
            reader
                .receive()
                .await
                .expect_err("cannot resume partial frame"),
            TransportError::Closed
        );
    });
}

#[test]
fn a_concurrent_failure_discards_an_inflight_read_result() {
    run(async {
        let (stream, mut peer) = tokio::io::duplex(128);
        let (mut reader, mut writer) =
            stdio(stream, Broken, Duration::from_secs(1)).expect("pipes");
        let mut receive = Box::pin(reader.receive());
        assert!(poll_fn(|cx| Poll::Ready(receive.as_mut().poll(cx).is_pending())).await);
        let frame = RpcMessage::new(json!({"jsonrpc":"2.0","method":"m"})).expect("frame");
        assert_eq!(writer.send(&frame).await, Err(TransportError::Io));
        peer.write_all(b"{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n")
            .await
            .expect("reply");
        assert_eq!(
            receive.await.expect_err("discard poisoned result"),
            TransportError::Closed
        );
    });
}

#[test]
fn reading_does_not_block_independent_writing() {
    run(async {
        let (stream, mut peer) = tokio::io::duplex(128);
        let (mut reader, mut writer) =
            stdio(stream, tokio::io::sink(), Duration::from_secs(1)).expect("pipes");
        let mut receive = Box::pin(reader.receive());
        assert!(poll_fn(|cx| Poll::Ready(receive.as_mut().poll(cx).is_pending())).await);
        let frame = RpcMessage::new(json!({"jsonrpc":"2.0","method":"m"})).expect("frame");
        writer.send(&frame).await.expect("independent write");
        peer.write_all(b"{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n")
            .await
            .expect("reply");
        assert_eq!(receive.await.expect("reply after write").value()["id"], 1);
    });
}

#[test]
fn a_flush_failure_closes_both_halves() {
    run(async {
        let buffered = tokio::io::BufWriter::with_capacity(128, Broken);
        let (mut reader, mut writer) =
            stdio(tokio::io::empty(), buffered, Duration::from_secs(1)).expect("pipes");
        let frame = RpcMessage::new(json!({"jsonrpc":"2.0","method":"m"})).expect("frame");
        assert_eq!(writer.send(&frame).await, Err(TransportError::Io));
        assert_eq!(
            reader.receive().await.expect_err("flush poison"),
            TransportError::Closed
        );
    });
}

#[test]
fn rejects_zero_deadline_before_io() {
    assert!(matches!(
        stdio(Broken, Broken, Duration::ZERO),
        Err(TransportError::InvalidDeadline)
    ));
}

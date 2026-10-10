mod acp_support;

use acp_support::{run, Broken};
use serde_json::json;
use std::time::Duration;
use threadline_agentd::acp::{stdio, RpcMessage, TransportError, MAX_FRAME_BYTES};
use tokio::io::AsyncWriteExt;

#[test]
fn fragmented_and_coalesced_messages_preserve_utf8_and_frame_boundaries() {
    run(async {
        let (stream, mut peer) = tokio::io::duplex(8);
        let (mut reader, _) =
            stdio(stream, tokio::io::sink(), Duration::from_secs(1)).expect("pipes");
        let task = tokio::spawn(async move {
            for part in [
                b"{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":\"h".as_slice(),
                b"\xc3",
                b"\xa9\\nllo\"}\n{\"jsonrpc\":\"2.0\",\"method\":\"notice\"}\n",
            ] {
                peer.write_all(part).await.expect("fixture write");
                tokio::task::yield_now().await;
            }
        });
        assert_eq!(
            reader.receive().await.expect("response").value()["result"],
            "hé\nllo"
        );
        assert_eq!(
            reader.receive().await.expect("notification").value()["method"],
            "notice"
        );
        task.await.expect("fixture task");
    });
}

#[test]
fn exact_byte_limit_roundtrips_and_oversized_serialization_writes_nothing() {
    run(async {
        let empty = json!({"jsonrpc":"2.0","id":1,"result":""});
        let overhead = serde_json::to_vec(&empty).expect("fixture encoding").len() + 1;
        let value = json!({"jsonrpc":"2.0","id":1,"result":"x".repeat(MAX_FRAME_BYTES - overhead)});
        let frame = RpcMessage::new(value.clone()).expect("frame");
        let mut bytes = Vec::new();
        let (_, mut writer) =
            stdio(tokio::io::empty(), &mut bytes, Duration::from_secs(1)).expect("pipes");
        writer.send(&frame).await.expect("exact bound");
        drop(writer);
        assert_eq!(bytes.len(), MAX_FRAME_BYTES);
        let (mut reader, _) =
            stdio(bytes.as_slice(), tokio::io::sink(), Duration::from_secs(1)).expect("pipes");
        assert_eq!(reader.receive().await.expect("exact frame").value(), &value);
        let oversized =
            RpcMessage::new(json!({"jsonrpc":"2.0","id":1,"result":"x".repeat(MAX_FRAME_BYTES)}))
                .expect("envelope");
        let mut output = Vec::new();
        let (mut reader, mut writer) =
            stdio(bytes.as_slice(), &mut output, Duration::from_secs(1)).expect("pipes");
        assert_eq!(writer.send(&oversized).await, Err(TransportError::TooLarge));
        assert_eq!(
            reader.receive().await.expect_err("shared poison"),
            TransportError::Closed
        );
        drop(writer);
        assert!(output.is_empty());
    });
}

#[test]
fn malformed_eof_and_read_errors_close_both_halves() {
    run(async {
        for (bytes, expected) in [
            (Vec::new(), TransportError::Closed),
            (b"{\"jsonrpc\":".to_vec(), TransportError::Truncated),
            (b"\xff\n".to_vec(), TransportError::Malformed),
            (b"not-json\n".to_vec(), TransportError::Malformed),
            (b"{}\n".to_vec(), TransportError::Malformed),
            (vec![b'x'; MAX_FRAME_BYTES], TransportError::TooLarge),
        ] {
            let (mut reader, mut writer) =
                stdio(bytes.as_slice(), tokio::io::sink(), Duration::from_secs(1)).expect("pipes");
            assert_eq!(reader.receive().await.expect_err("read failure"), expected);
            let frame = RpcMessage::new(json!({"jsonrpc":"2.0","method":"m"})).expect("frame");
            assert_eq!(writer.send(&frame).await, Err(TransportError::Closed));
            assert_eq!(
                reader.receive().await.expect_err("closed"),
                TransportError::Closed
            );
        }
        let (mut reader, _) =
            stdio(Broken, tokio::io::sink(), Duration::from_secs(1)).expect("pipes");
        assert_eq!(
            reader.receive().await.expect_err("broken pipe"),
            TransportError::Io
        );
    });
}

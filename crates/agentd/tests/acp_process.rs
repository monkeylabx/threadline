use serde_json::json;
use std::{path::PathBuf, process::Stdio, time::Duration};
use threadline_agentd::acp::{stdio, RpcMessage, TransportError};
use tokio::{
    process::{Child, Command},
    runtime::Builder,
};

fn node_path() -> PathBuf {
    let path = std::env::var_os("PATH").expect("pinned Node on test PATH");
    let name = if cfg!(windows) { "node.exe" } else { "node" };
    std::env::split_paths(&path)
        .map(|dir| dir.join(name))
        .find(|candidate| candidate.is_file())
        .expect("Node executable")
}

fn fixture(mode: &str) -> Child {
    Command::new(node_path())
        .arg(PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("test_support/acp-fixture.mjs"))
        .arg(mode)
        .arg(format!(
            "v{}",
            include_str!("../../../.node-version").trim()
        ))
        .env_clear()
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .kill_on_drop(true)
        .spawn()
        .expect("synthetic worker")
}

async fn reap(child: &mut Child, failed: bool) -> std::process::ExitStatus {
    let kill = if failed { child.start_kill() } else { Ok(()) };
    let status = tokio::time::timeout(Duration::from_secs(10), child.wait()).await;
    if status.is_err() {
        tokio::time::timeout(Duration::from_secs(10), child.kill())
            .await
            .expect("fixture kill deadline")
            .expect("kill and reap overdue fixture");
    }
    kill.expect("kill failed fixture");
    status
        .expect("bounded fixture exit")
        .expect("reap direct child")
}

async fn exchange(mode: &'static str) {
    let mut child = fixture(mode);
    let (mut reader, mut writer) = stdio(
        child.stdout.take().expect("stdout"),
        child.stdin.take().expect("stdin"),
        Duration::from_secs(10),
    )
    .expect("pipes");
    let mut traffic = tokio::spawn(async move {
        let request = RpcMessage::new(json!({"jsonrpc":"2.0","id":7,"method":"initialize",
        "params":{"protocolVersion":1,"clientCapabilities":{}}}))
        .expect("request");
        writer.send(&request).await.expect("write real child pipe");
        assert_ne!(mode, "panic", "synthetic exchange failure");
        if mode == "partial" {
            assert_eq!(
                reader.receive().await.expect_err("abnormal EOF"),
                TransportError::Truncated
            );
        } else {
            let reply = reader.receive().await.expect("initialize-shaped response");
            assert_eq!(reply.value()["id"], 7);
            assert_eq!(reply.value()["result"]["protocolVersion"], 1);
            assert_eq!(
                reader.receive().await.expect("notification").value()["method"],
                "_fixture/notice"
            );
            assert_eq!(
                reader.receive().await.expect_err("clean EOF"),
                TransportError::Closed
            );
        }
        assert_eq!(writer.send(&request).await, Err(TransportError::Closed));
    });
    let outcome = tokio::time::timeout(Duration::from_secs(30), &mut traffic).await;
    let failed = !matches!(outcome, Ok(Ok(())));
    if failed {
        traffic.abort();
    }
    let status = reap(&mut child, failed).await;
    if mode == "panic" {
        assert!(outcome
            .expect("bounded exchange")
            .expect_err("synthetic panic")
            .is_panic());
    } else {
        outcome.expect("bounded exchange").expect("fixture traffic");
        assert_eq!(status.code(), Some(if mode == "partial" { 42 } else { 0 }));
    }
}

#[test]
fn real_synthetic_process_exchanges_frames_and_reports_abnormal_eof() {
    Builder::new_current_thread()
        .enable_all()
        .build()
        .expect("test runtime")
        .block_on(async {
            exchange("exchange").await;
            exchange("partial").await;
            exchange("panic").await;
        });
}

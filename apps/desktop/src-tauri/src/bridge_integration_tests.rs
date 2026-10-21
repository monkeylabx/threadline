//! Integration boundary: actual Tauri State/async commands against loopback HTTP.
//! Uses MockRuntime, never a native window or production service.
use crate::{commands, config::HostConfig, dto::HostError, session::Session, HostState};
use std::{
    io::{Read, Write},
    net::TcpListener,
    sync::{atomic::AtomicBool, Arc, Mutex},
    thread,
};
use tauri::Manager;

fn server(replies: Vec<Vec<u8>>) -> (String, thread::JoinHandle<()>) {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let url = format!("http://{}", listener.local_addr().unwrap());
    let worker = thread::spawn(move || {
        for reply in replies {
            let (mut socket, _) = listener.accept().unwrap();
            socket
                .set_read_timeout(Some(std::time::Duration::from_secs(3)))
                .unwrap();
            let mut buffer = [0; 8192];
            let received = socket.read(&mut buffer).unwrap();
            assert!(received > 0);
            socket.write_all(&reply).unwrap();
        }
    });
    (url, worker)
}

fn response(content_type: &str, body: &[u8]) -> Vec<u8> {
    let mut output = format!("HTTP/1.1 200 OK\r\nContent-Type: {content_type}\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",body.len()).into_bytes();
    output.extend_from_slice(body);
    output
}

#[test]
fn actual_command_state_handles_wait_sync_and_display_without_secret_ipc() {
    let core_reply = response("application/proto", &[]);
    let (core, core_thread) = server(vec![core_reply.clone(), core_reply.clone(), core_reply]);
    let (realtime, realtime_thread) = server(vec![response(
        "application/connect+proto",
        &[2, 0, 0, 0, 2, b'{', b'}'],
    )]);
    let directory = std::env::temp_dir().join(format!(
        "threadline-bridge-integration-{}",
        std::process::id()
    ));
    std::fs::create_dir_all(&directory).unwrap();
    let app = configured_host(core, realtime, &directory);
    assert!(
        commands::session_info(app.state())
            .unwrap()
            .unwrap()
            .realtime_enabled
    );
    assert!(commands::list_channels(app.state()).unwrap().is_empty());
    assert!(
        commands::list_messages(app.state(), "channel-synthetic".into())
            .unwrap()
            .is_empty()
    );
    let report = tauri::async_runtime::block_on(commands::wait_channel(
        app.state(),
        "channel-synthetic".into(),
    ))
    .unwrap();
    assert!(report.online && !report.realtime_woke);
    assert!(
        commands::sync_channel(app.state(), "channel-synthetic".into())
            .unwrap()
            .online
    );
    let pending = commands::send_message(
        app.state(),
        "channel-synthetic".into(),
        "synthetic message".into(),
    )
    .unwrap();
    assert_eq!(pending.channel_seq, None);
    core_thread.join().unwrap();
    realtime_thread.join().unwrap();
    drop(app);
    std::fs::remove_dir_all(directory).unwrap();
}

#[test]
fn unconfigured_command_state_fails_closed() {
    let app = tauri::test::mock_builder()
        .manage(HostState {
            session: Arc::new(Mutex::new(None)),
            waiting: Arc::new(AtomicBool::new(false)),
        })
        .build(tauri::test::mock_context(tauri::test::noop_assets()))
        .unwrap();
    assert!(commands::session_info(app.state()).unwrap().is_none());
    assert_eq!(
        commands::sync_channel(app.state(), "channel-synthetic".into()),
        Err(HostError::NoSession)
    );
    assert_eq!(
        tauri::async_runtime::block_on(commands::wait_channel(
            app.state(),
            "channel-synthetic".into()
        )),
        Err(HostError::NoSession)
    );
}

fn configured_host(
    core: String,
    realtime: String,
    directory: &std::path::Path,
) -> tauri::App<tauri::test::MockRuntime> {
    let session = Session::connect(HostConfig {
        core_url: core,
        realtime_url: Some(realtime),
        dev_token: "synthetic-token".into(),
        tenant_id: "tenant-synthetic".into(),
        actor_id: "actor-synthetic".into(),
        device_id: "device-synthetic".into(),
        data_dir: directory.to_path_buf(),
        db_key: [9; 32],
    })
    .unwrap();
    let app = tauri::test::mock_builder()
        .manage(HostState {
            session: Arc::new(Mutex::new(Some(session))),
            waiting: Arc::new(AtomicBool::new(false)),
        })
        .build(tauri::test::mock_context(tauri::test::noop_assets()))
        .unwrap();
    app
}

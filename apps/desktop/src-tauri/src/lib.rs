//! Desktop host: the Device's local message service behind typed, window-scoped
//! Tauri commands. It opens one `locald` store, holds one Core client and
//! never lets the session credential or the database key cross the IPC
//! boundary. Without configuration the host still starts and the renderer
//! stays on fixtures, so the IM is usable while Core is offline.

#[cfg(test)]
mod bridge_integration_tests;
mod commands;
pub mod config;
pub mod dto;
mod notification;
pub mod session;
#[cfg(test)]
mod session_tests;

use std::sync::{atomic::AtomicBool, Arc, Mutex};

use session::Session;
use threadline_core_client::CoreClient;

/// Window-scoped state. `None` means the host runs without a configured
/// Device session.
pub struct HostState {
    session: Arc<Mutex<Option<Session<CoreClient>>>>,
    waiting: Arc<AtomicBool>,
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let session = match config::HostConfig::from_env(|name| std::env::var(name).ok()) {
        Ok(Some(config)) => match Session::connect(config) {
            Ok(session) => Some(session),
            Err(error) => {
                eprintln!("threadline-desktop-host: session unavailable ({error:?})");
                None
            }
        },
        Ok(None) => None,
        Err(error) => {
            eprintln!("threadline-desktop-host: configuration rejected ({error:?})");
            None
        }
    };
    tauri::Builder::default()
        .manage(HostState {
            session: Arc::new(Mutex::new(session)),
            waiting: Arc::new(AtomicBool::new(false)),
        })
        .invoke_handler(tauri::generate_handler![
            commands::session_info,
            commands::list_channels,
            commands::list_messages,
            commands::send_message,
            commands::sync_channel,
            commands::wait_channel,
        ])
        .run(tauri::generate_context!())
        .expect("failed to run the Threadline desktop host");
}

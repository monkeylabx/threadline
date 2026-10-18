//! Tauri commands: thin, window-scoped adapters over `Session`. Each locks the
//! session for one bounded operation; none of them starts background work.

use tauri::State;
use threadline_core_client::CoreClient;

use crate::dto::{ChannelDto, HostError, MessageDto, SessionInfo, SyncReportDto};
use crate::session::Session;
use crate::HostState;

fn with_session<T>(
    state: &State<'_, HostState>,
    operation: impl FnOnce(&mut Session<CoreClient>) -> Result<T, HostError>,
) -> Result<T, HostError> {
    let mut guard = state.session.lock().map_err(|_| HostError::Storage)?;
    operation(guard.as_mut().ok_or(HostError::NoSession)?)
}

/// `None` tells the renderer to stay on fixtures.
#[tauri::command]
pub fn session_info(state: State<'_, HostState>) -> Result<Option<SessionInfo>, HostError> {
    let guard = state.session.lock().map_err(|_| HostError::Storage)?;
    Ok(guard.as_ref().map(|session| session.info().clone()))
}

#[tauri::command]
pub fn list_channels(state: State<'_, HostState>) -> Result<Vec<ChannelDto>, HostError> {
    with_session(&state, |session| session.channels())
}

#[tauri::command]
pub fn list_messages(
    state: State<'_, HostState>,
    channel_id: String,
) -> Result<Vec<MessageDto>, HostError> {
    with_session(&state, |session| session.messages(&channel_id))
}

#[tauri::command]
pub fn send_message(
    state: State<'_, HostState>,
    channel_id: String,
    text: String,
) -> Result<MessageDto, HostError> {
    with_session(&state, |session| session.send(&channel_id, &text))
}

#[tauri::command]
pub fn sync_channel(
    state: State<'_, HostState>,
    channel_id: String,
) -> Result<SyncReportDto, HostError> {
    with_session(&state, |session| session.sync(&channel_id))
}

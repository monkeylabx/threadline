//! Session behaviour against an in-memory `Server`: send → pending, sync →
//! committed exactly once, retryable failures leave state untouched and
//! report offline, denials keep the item pending while staying online.

use std::{collections::HashMap, fs, path::PathBuf};

use threadline_client_core::storage::DatabaseKey;
use threadline_locald::{
    DeviceIdentity, Locald, SendAck, SendFailure, SendRequest, Server, SyncBatch, SyncFailure,
    SyncedEvent,
};

use crate::dto::{MessageState, SessionInfo};
use crate::session::Session;

const CHANNEL: &str = "channel-synthetic";

#[derive(Default)]
struct FakeServer {
    committed: Vec<(String, u64, Vec<u8>)>,
    by_key: HashMap<String, u64>,
    mode: Mode,
}

#[derive(Clone, Copy, Default, PartialEq, Eq)]
enum Mode {
    #[default]
    Online,
    Unreachable,
    Denying,
}

impl Server for FakeServer {
    fn send(&mut self, request: SendRequest) -> Result<SendAck, SendFailure> {
        match self.mode {
            Mode::Unreachable => return Err(SendFailure::Retryable),
            Mode::Denying => return Err(SendFailure::Denied),
            Mode::Online => {}
        }
        let (channel_seq, deduplicated) = match self.by_key.get(&request.idempotency_key) {
            Some(seq) => (*seq, true),
            None => {
                let seq = self.committed.len() as u64 + 1;
                self.committed.push((
                    request.event_id.clone(),
                    seq,
                    request.envelope_bytes.clone(),
                ));
                self.by_key.insert(request.idempotency_key.clone(), seq);
                (seq, false)
            }
        };
        Ok(SendAck {
            event_id: request.event_id,
            idempotency_key: request.idempotency_key,
            channel_seq,
            envelope_bytes: request.envelope_bytes,
            deduplicated,
        })
    }

    fn sync(&mut self, _: &str, _: &str, after: u64, _: u32) -> Result<SyncBatch, SyncFailure> {
        if self.mode == Mode::Unreachable {
            return Err(SyncFailure::Retryable);
        }
        Ok(SyncBatch {
            events: self
                .committed
                .iter()
                .filter(|(_, seq, _)| *seq > after)
                .map(|(event_id, seq, bytes)| SyncedEvent {
                    event_id: event_id.clone(),
                    channel_seq: *seq,
                    envelope_bytes: bytes.clone(),
                })
                .collect(),
            has_more: false,
        })
    }
}

struct TempStore(PathBuf);

impl TempStore {
    fn session(name: &str) -> (Self, Session<FakeServer>) {
        let dir =
            std::env::temp_dir().join(format!("threadline-host-{name}-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).expect("temp dir");
        let locald = Locald::open(
            dir.join("locald.sqlite"),
            DatabaseKey::new([7u8; 32]).expect("key"),
            DeviceIdentity {
                tenant_id: "tenant-synthetic".to_owned(),
                device_id: "device-synthetic".to_owned(),
            },
        )
        .expect("open");
        let info = SessionInfo {
            core_url: "http://127.0.0.1:1".to_owned(),
            tenant_id: "tenant-synthetic".to_owned(),
            actor_id: "actor-synthetic".to_owned(),
            device_id: "device-synthetic".to_owned(),
        };
        (
            Self(dir),
            Session::with_server(locald, FakeServer::default(), info),
        )
    }
}

impl Drop for TempStore {
    fn drop(&mut self) {
        let _ = fs::remove_dir_all(&self.0);
    }
}

fn states(session: &Session<FakeServer>) -> Vec<(String, MessageState, Option<u64>)> {
    session
        .messages(CHANNEL)
        .expect("messages")
        .into_iter()
        .map(|message| (message.text, message.state, message.channel_seq))
        .collect()
}

#[test]
fn send_is_pending_until_sync_commits_it_exactly_once() {
    let (_store, mut session) = TempStore::session("commit");
    let pending = session.send(CHANNEL, "你好").expect("send");
    assert_eq!(pending.state, MessageState::Pending);
    assert_eq!(pending.author_actor_id, "actor-synthetic");
    assert!(pending.sent_at_unix_ms.is_some());
    assert_eq!(
        states(&session),
        [("你好".to_owned(), MessageState::Pending, None)]
    );

    // The pull walks our own commit once to move the cursor past it; the
    // timeline still shows exactly one row.
    let report = session.sync(CHANNEL).expect("sync");
    assert_eq!(
        (report.committed, report.applied, report.online),
        (1, 1, true)
    );
    assert_eq!(
        states(&session),
        [("你好".to_owned(), MessageState::Committed, Some(1))]
    );

    let again = session.sync(CHANNEL).expect("second sync");
    assert_eq!((again.committed, again.applied), (0, 0));
    assert_eq!(states(&session).len(), 1);
}

#[test]
fn unreachable_server_keeps_the_item_pending_and_reports_offline() {
    let (_store, mut session) = TempStore::session("offline");
    session.server.mode = Mode::Unreachable;
    session.send(CHANNEL, "later").expect("send");
    let report = session.sync(CHANNEL).expect("sync");
    assert_eq!((report.retained, report.online), (1, false));
    assert_eq!(states(&session)[0].1, MessageState::Pending);

    session.server.mode = Mode::Online;
    let recovered = session.sync(CHANNEL).expect("sync");
    assert_eq!((recovered.committed, recovered.online), (1, true));
    assert_eq!(
        states(&session)[0],
        ("later".to_owned(), MessageState::Committed, Some(1))
    );
}

#[test]
fn denied_send_stays_pending_while_the_session_is_online() {
    let (_store, mut session) = TempStore::session("denied");
    session.server.mode = Mode::Denying;
    session.send(CHANNEL, "nope").expect("send");
    let report = session.sync(CHANNEL).expect("sync");
    assert_eq!(
        (report.retained, report.committed, report.online),
        (1, 0, true)
    );
    assert_eq!(states(&session)[0].1, MessageState::Pending);
}

#[test]
fn pull_applies_another_devices_messages_in_order() {
    let (_store, mut session) = TempStore::session("pull");
    let (_other_store, mut other) = TempStore::session("pull-other");
    other.send(CHANNEL, "from other 1").expect("send");
    other.send(CHANNEL, "from other 2").expect("send");
    other.sync(CHANNEL).expect("sync other");
    session.server = std::mem::take(&mut other.server);

    let report = session.sync(CHANNEL).expect("sync");
    assert_eq!((report.applied, report.online), (2, true));
    let texts: Vec<String> = states(&session)
        .into_iter()
        .map(|(text, _, _)| text)
        .collect();
    assert_eq!(texts, ["from other 1", "from other 2"]);
}

#[test]
fn empty_text_and_bad_channel_are_rejected_before_queueing() {
    let (_store, mut session) = TempStore::session("invalid");
    assert!(session.send(CHANNEL, "").is_err());
    assert!(session.send(" padded", "x").is_err());
    assert!(states(&session).is_empty());
}

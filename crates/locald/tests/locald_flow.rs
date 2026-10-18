use std::{collections::HashMap, fs, path::PathBuf};

use threadline_client_core::storage::DatabaseKey;
use threadline_locald::{
    DeviceIdentity, Locald, LocaldError, SendAck, SendFailure, SendRequest, Server, SyncBatch,
    SyncFailure, SyncedEvent,
};

const TENANT: &str = "tenant-locald-synthetic";
const DEVICE: &str = "device-locald-laptop-synthetic";
const CHANNEL: &str = "channel-locald-synthetic";

#[test]
fn enqueue_is_durable_before_any_send_and_survives_restart() {
    let fixture = StoreFixture::create();
    let mut locald = fixture.open();
    locald
        .enqueue(CHANNEL, "evt-1", "key-1", b"body-1")
        .expect("enqueue");
    locald
        .enqueue(CHANNEL, "evt-2", "key-2", b"body-2")
        .expect("enqueue");
    drop(locald);

    let locald = fixture.open();
    let pending = locald.pending(CHANNEL).expect("pending after restart");
    let ids = pending
        .iter()
        .map(|item| item.event_id.as_str())
        .collect::<Vec<_>>();
    assert_eq!(ids, ["evt-1", "evt-2"]);
    assert_eq!(locald.cursor(CHANNEL).expect("cursor"), 0);
}

#[test]
fn replay_sends_in_order_and_folds_each_ack_into_one_committed_item() {
    let fixture = StoreFixture::create();
    let mut locald = fixture.open();
    for id in ["evt-1", "evt-2", "evt-3"] {
        locald
            .enqueue(CHANNEL, id, &format!("key-{id}"), id.as_bytes())
            .expect("enqueue");
    }
    let mut server = FakeServer::default();

    let report = locald.replay(&mut server, CHANNEL).expect("replay");
    assert_eq!(
        (report.committed, report.deduplicated, report.retained),
        (3, 0, 0)
    );
    assert_eq!(server.sent_ids(), ["evt-1", "evt-2", "evt-3"]);
    assert!(locald.pending(CHANNEL).expect("pending").is_empty());
    let committed = locald.committed(CHANNEL, 0, 10).expect("committed");
    assert_eq!(
        committed
            .iter()
            .map(|record| (record.event_id.as_str(), record.committed_sequence))
            .collect::<Vec<_>>(),
        [("evt-1", 1), ("evt-2", 2), ("evt-3", 3)]
    );

    // A second pass with nothing pending sends nothing.
    let again = locald.replay(&mut server, CHANNEL).expect("replay again");
    assert_eq!(again, Default::default());
    assert_eq!(server.sent_ids().len(), 3);
}

#[test]
fn a_lost_ack_is_recovered_by_the_servers_deduplicated_receipt() {
    let fixture = StoreFixture::create();
    let mut locald = fixture.open();
    locald
        .enqueue(CHANNEL, "evt-1", "key-1", b"body-1")
        .expect("enqueue");
    // The server commits but the ACK is lost on the way back.
    let mut server = FakeServer {
        lose_next_ack: true,
        ..Default::default()
    };
    let first = locald
        .replay(&mut server, CHANNEL)
        .expect("replay with lost ack");
    assert_eq!(first.retained, 1);
    assert_eq!(locald.pending(CHANNEL).expect("pending").len(), 1);

    let second = locald.replay(&mut server, CHANNEL).expect("retry");
    assert_eq!((second.committed, second.deduplicated), (0, 1));
    assert!(locald.pending(CHANNEL).expect("pending").is_empty());
    assert_eq!(
        locald.committed(CHANNEL, 0, 10).expect("committed").len(),
        1
    );
    assert_eq!(
        server.commits.len(),
        1,
        "the server holds exactly one logical event"
    );
}

#[test]
fn retryable_failure_stops_the_pass_in_order_and_denial_retains_the_item() {
    let fixture = StoreFixture::create();
    let mut locald = fixture.open();
    for id in ["evt-1", "evt-2", "evt-3"] {
        locald
            .enqueue(CHANNEL, id, &format!("key-{id}"), id.as_bytes())
            .expect("enqueue");
    }
    let mut server = FakeServer::default();
    server
        .fail_event
        .insert("evt-2".to_owned(), SendFailure::Retryable);
    let report = locald.replay(&mut server, CHANNEL).expect("replay");
    assert_eq!((report.committed, report.retained), (1, 2));
    assert_eq!(
        server.sent_ids(),
        ["evt-1", "evt-2"],
        "evt-3 must wait behind the retryable failure"
    );
    let pending = locald.pending(CHANNEL).expect("pending");
    assert_eq!(
        pending
            .iter()
            .map(|item| item.event_id.as_str())
            .collect::<Vec<_>>(),
        ["evt-2", "evt-3"]
    );

    server
        .fail_event
        .insert("evt-2".to_owned(), SendFailure::Denied);
    let report = locald
        .replay(&mut server, CHANNEL)
        .expect("replay after denial");
    assert_eq!(
        (report.committed, report.retained),
        (1, 1),
        "a denied item is retained while the rest proceed"
    );
    assert_eq!(
        locald.pending(CHANNEL).expect("pending")[0].event_id,
        "evt-2"
    );
}

#[test]
fn pull_applies_contiguous_history_and_never_advances_past_a_hole() {
    let fixture = StoreFixture::create();
    let mut locald = fixture.open();
    let mut server = FakeServer::default();
    server.seed(["evt-a", "evt-b", "evt-c"]);

    let first = locald.pull(&mut server, CHANNEL, 2).expect("first pull");
    assert_eq!(
        (first.applied, first.next_sequence, first.has_more),
        (2, 2, true)
    );
    assert_eq!(locald.cursor(CHANNEL).expect("cursor"), 2);
    let second = locald.pull(&mut server, CHANNEL, 10).expect("second pull");
    assert_eq!(
        (second.applied, second.next_sequence, second.has_more),
        (1, 3, false)
    );
    assert_eq!(
        locald.committed(CHANNEL, 0, 10).expect("committed").len(),
        3
    );

    // A batch with a hole stops before it and leaves the cursor untouched.
    server
        .commits
        .push(("evt-e".to_owned(), 5, b"evt-e".to_vec()));
    assert_eq!(
        locald.pull(&mut server, CHANNEL, 10),
        Err(LocaldError::HistoryMismatch)
    );
    assert_eq!(locald.cursor(CHANNEL).expect("cursor"), 3);
    assert_eq!(
        locald.committed(CHANNEL, 0, 10).expect("committed").len(),
        3
    );
    assert_eq!(
        locald.pull(&mut FailingSync, CHANNEL, 10),
        Err(LocaldError::Retryable)
    );
}

#[test]
fn pull_folds_our_own_pending_event_when_the_event_arrives_before_the_ack() {
    let fixture = StoreFixture::create();
    let mut locald = fixture.open();
    locald
        .enqueue(CHANNEL, "evt-mine", "key-mine", b"mine")
        .expect("enqueue");
    let mut server = FakeServer {
        lose_next_ack: true,
        ..Default::default()
    };
    locald
        .replay(&mut server, CHANNEL)
        .expect("send with lost ack");

    let report = locald
        .pull(&mut server, CHANNEL, 10)
        .expect("pull sees our own event first");
    assert_eq!(report.applied, 1);
    assert!(
        locald.pending(CHANNEL).expect("pending").is_empty(),
        "the pending copy folded into the committed event"
    );
    assert_eq!(
        locald.committed(CHANNEL, 0, 10).expect("committed").len(),
        1
    );

    // The late ACK is now a harmless duplicate.
    let late = locald
        .replay(&mut server, CHANNEL)
        .expect("nothing pending");
    assert_eq!(late, Default::default());
    assert_eq!(
        locald
            .pull(&mut server, CHANNEL, 10)
            .expect("re-pull")
            .applied,
        0
    );
}

#[derive(Default)]
struct FakeServer {
    commits: Vec<(String, u64, Vec<u8>)>,
    keys: HashMap<String, usize>,
    sent: Vec<String>,
    fail_event: HashMap<String, SendFailure>,
    lose_next_ack: bool,
}

impl FakeServer {
    fn seed<const N: usize>(&mut self, ids: [&str; N]) {
        for id in ids {
            let seq = self.commits.len() as u64 + 1;
            self.commits
                .push((id.to_owned(), seq, id.as_bytes().to_vec()));
        }
    }

    fn sent_ids(&self) -> Vec<&str> {
        self.sent.iter().map(String::as_str).collect()
    }
}

impl Server for FakeServer {
    fn send(&mut self, request: SendRequest) -> Result<SendAck, SendFailure> {
        self.sent.push(request.event_id.clone());
        if let Some(failure) = self.fail_event.get(&request.event_id) {
            return Err(*failure);
        }
        let (index, deduplicated) = match self.keys.get(&request.idempotency_key) {
            Some(index) => (*index, true),
            None => {
                let seq = self.commits.len() as u64 + 1;
                self.commits.push((
                    request.event_id.clone(),
                    seq,
                    request.envelope_bytes.clone(),
                ));
                self.keys
                    .insert(request.idempotency_key.clone(), self.commits.len() - 1);
                (self.commits.len() - 1, false)
            }
        };
        if std::mem::take(&mut self.lose_next_ack) {
            return Err(SendFailure::Retryable);
        }
        let (event_id, seq, bytes) = &self.commits[index];
        Ok(SendAck {
            event_id: event_id.clone(),
            idempotency_key: request.idempotency_key,
            channel_seq: *seq,
            envelope_bytes: bytes.clone(),
            deduplicated,
        })
    }

    fn sync(
        &mut self,
        _tenant: &str,
        _channel: &str,
        after: u64,
        max: u32,
    ) -> Result<SyncBatch, SyncFailure> {
        let remaining = self
            .commits
            .iter()
            .filter(|(_, seq, _)| *seq > after)
            .collect::<Vec<_>>();
        let events = remaining
            .iter()
            .take(max as usize)
            .map(|(id, seq, bytes)| SyncedEvent {
                event_id: id.clone(),
                channel_seq: *seq,
                envelope_bytes: bytes.clone(),
            })
            .collect::<Vec<_>>();
        Ok(SyncBatch {
            has_more: remaining.len() > events.len(),
            events,
        })
    }
}

struct FailingSync;

impl Server for FailingSync {
    fn send(&mut self, _: SendRequest) -> Result<SendAck, SendFailure> {
        Err(SendFailure::Retryable)
    }

    fn sync(&mut self, _: &str, _: &str, _: u64, _: u32) -> Result<SyncBatch, SyncFailure> {
        Err(SyncFailure::Retryable)
    }
}

struct StoreFixture {
    directory: PathBuf,
    key: [u8; 32],
}

impl StoreFixture {
    fn create() -> Self {
        let mut nonce = [0_u8; 16];
        let mut key = [0_u8; 32];
        getrandom::fill(&mut nonce).expect("nonce");
        getrandom::fill(&mut key).expect("key");
        if key.iter().all(|byte| *byte == 0) {
            key[0] = 1;
        }
        let suffix = nonce
            .iter()
            .map(|byte| format!("{byte:02x}"))
            .collect::<String>();
        let directory = std::env::temp_dir().join(format!("threadline-locald-{suffix}"));
        fs::create_dir(&directory).expect("create test directory");
        Self { directory, key }
    }

    fn open(&self) -> Locald {
        Locald::open(
            self.directory.join("locald.db"),
            DatabaseKey::new(self.key).expect("key"),
            DeviceIdentity {
                tenant_id: TENANT.to_owned(),
                device_id: DEVICE.to_owned(),
            },
        )
        .expect("open locald store")
    }
}

impl Drop for StoreFixture {
    fn drop(&mut self) {
        let _ = fs::remove_dir_all(&self.directory);
    }
}

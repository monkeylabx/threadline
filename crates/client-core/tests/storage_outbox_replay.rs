use std::{fs, path::PathBuf};

use threadline_client_core::storage::{
    CommitPending, ConversationRef, DatabaseKey, EncryptedDatabase, PendingOutboxEvent,
    StorageError,
};

const TENANT: &str = "tenant-replay-synthetic";

#[test]
fn pending_events_replay_in_enqueue_order_across_reopen() {
    let fixture = DatabaseFixture::create();
    let channel = ConversationRef::channel("channel-replay").expect("valid channel");
    let mut database = fixture.open();
    for id in ["evt-first", "evt-second", "evt-third"] {
        database
            .enqueue_outbox(pending(channel, id))
            .expect("enqueue pending event");
    }
    drop(database);

    let database = fixture.open();
    let replay = database
        .list_pending_outbox(TENANT, channel)
        .expect("list pending after reopen");
    let order = replay
        .iter()
        .map(|record| record.event_id.as_str())
        .collect::<Vec<_>>();
    assert_eq!(order, ["evt-first", "evt-second", "evt-third"]);
    assert_eq!(
        replay
            .iter()
            .map(|record| record.enqueue_sequence)
            .collect::<Vec<_>>(),
        [1, 2, 3]
    );
    assert_eq!(replay[1].idempotency_key, "idem-evt-second");
    assert_eq!(replay[1].envelope_bytes, b"body-evt-second");
    let other = ConversationRef::channel("channel-other").expect("valid channel");
    assert!(database
        .list_pending_outbox(TENANT, other)
        .expect("list other conversation")
        .is_empty());
}

#[test]
fn commit_pending_moves_exactly_one_logical_item_and_is_idempotent() {
    let fixture = DatabaseFixture::create();
    let channel = ConversationRef::channel("channel-commit").expect("valid channel");
    let mut database = fixture.open();
    database
        .enqueue_outbox(pending(channel, "evt-1"))
        .expect("enqueue");
    database
        .enqueue_outbox(pending(channel, "evt-2"))
        .expect("enqueue");

    database
        .commit_pending(commit(channel, "evt-1", 7))
        .expect("first ACK commits");
    let remaining = database
        .list_pending_outbox(TENANT, channel)
        .expect("list pending");
    assert_eq!(remaining.len(), 1);
    assert_eq!(remaining[0].event_id, "evt-2");
    let committed = database
        .list_committed_events(TENANT, channel, 0, 10)
        .expect("list committed");
    assert_eq!(committed.len(), 1);
    assert_eq!(committed[0].event_id, "evt-1");
    assert_eq!(committed[0].committed_sequence, 7);
    assert_eq!(committed[0].envelope_bytes, b"server-evt-1");

    database
        .commit_pending(commit(channel, "evt-1", 7))
        .expect("duplicate ACK is idempotent");
    assert_eq!(
        database.commit_pending(commit(channel, "evt-1", 8)),
        Err(StorageError::Conflict),
        "a different sequence for a committed event is a conflict"
    );
    assert_eq!(
        database.commit_pending(commit(channel, "evt-never-seen", 9)),
        Err(StorageError::Conflict),
        "an ACK for an event never pending or committed locally is refused"
    );
    assert_eq!(
        database
            .list_committed_events(TENANT, channel, 0, 10)
            .expect("list committed")
            .len(),
        1
    );
    assert_eq!(
        database
            .list_pending_outbox(TENANT, channel)
            .expect("list pending")
            .len(),
        1,
        "refused commits leave the pending row untouched"
    );
}

#[test]
fn committed_events_page_in_ascending_sequence_order() {
    let fixture = DatabaseFixture::create();
    let channel = ConversationRef::channel("channel-page").expect("valid channel");
    let mut database = fixture.open();
    for (id, sequence) in [
        ("evt-c", 3_u64),
        ("evt-a", 1),
        ("evt-b", 2),
        ("evt-max", u64::MAX),
    ] {
        database
            .enqueue_outbox(pending(channel, id))
            .expect("enqueue");
        database
            .commit_pending(commit(channel, id, sequence))
            .expect("commit");
    }
    let first = database
        .list_committed_events(TENANT, channel, 0, 2)
        .expect("first page");
    assert_eq!(
        first
            .iter()
            .map(|record| record.committed_sequence)
            .collect::<Vec<_>>(),
        [1, 2]
    );
    let rest = database
        .list_committed_events(TENANT, channel, 2, 10)
        .expect("second page");
    assert_eq!(
        rest.iter()
            .map(|record| record.committed_sequence)
            .collect::<Vec<_>>(),
        [3, u64::MAX]
    );
    assert!(database
        .list_committed_events(TENANT, channel, u64::MAX, 10)
        .expect("past the end")
        .is_empty());
    assert_eq!(
        database.list_committed_events(TENANT, channel, 0, 0),
        Err(StorageError::InvalidInput)
    );
}

fn pending(channel: ConversationRef<'static>, id: &'static str) -> PendingOutboxEvent<'static> {
    PendingOutboxEvent {
        tenant_id: TENANT,
        conversation: channel,
        event_id: id,
        idempotency_key: Box::leak(format!("idem-{id}").into_boxed_str()),
        envelope_bytes: Box::leak(format!("body-{id}").into_bytes().into_boxed_slice()),
    }
}

fn commit(
    channel: ConversationRef<'static>,
    id: &'static str,
    sequence: u64,
) -> CommitPending<'static> {
    CommitPending {
        tenant_id: TENANT,
        conversation: channel,
        event_id: id,
        committed_sequence: sequence,
        envelope_bytes: Box::leak(format!("server-{id}").into_bytes().into_boxed_slice()),
    }
}

struct DatabaseFixture {
    directory: PathBuf,
    path: PathBuf,
    key: [u8; 32],
}

impl DatabaseFixture {
    fn create() -> Self {
        let mut nonce = [0_u8; 16];
        let mut key = [0_u8; 32];
        getrandom::fill(&mut nonce).expect("obtain runtime directory nonce");
        getrandom::fill(&mut key).expect("obtain runtime database key");
        if key.iter().all(|byte| *byte == 0) {
            key[0] = 1;
        }
        let suffix = nonce
            .iter()
            .map(|byte| format!("{byte:02x}"))
            .collect::<String>();
        let directory = std::env::temp_dir().join(format!("threadline-outbox-replay-{suffix}"));
        fs::create_dir(&directory).expect("create isolated storage test directory");
        let path = directory.join("client-core.db");
        Self {
            directory,
            path,
            key,
        }
    }

    fn open(&self) -> EncryptedDatabase {
        EncryptedDatabase::open(
            &self.path,
            DatabaseKey::new(self.key).expect("accept generated fixed-size key"),
        )
        .expect("open encrypted database through production interface")
    }
}

impl Drop for DatabaseFixture {
    fn drop(&mut self) {
        let _ = fs::remove_dir_all(&self.directory);
    }
}

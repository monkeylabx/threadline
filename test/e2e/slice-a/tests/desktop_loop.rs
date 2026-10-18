//! Two Devices exchange plaintext messages through locald + core-client against
//! a real Core and PostgreSQL, Core is restarted mid-run, and a lost ACK is
//! retried. Runs only where CI provides the Core binary and the DSN.

use threadline_core_client::envelope;
use threadline_e2e_slice_a::{CoreProcess, Environment, Peer, LIN_JIA, MENG_QING};
use threadline_locald::{LocaldError, SendRequest, Server};

#[test]
fn two_devices_exchange_messages_survive_a_core_restart_and_never_duplicate() {
    let Some(environment) = Environment::from_env() else {
        eprintln!(
            "slice-a e2e NOT RUN: THREADLINE_E2E_CORE_BIN / THREADLINE_E2E_POSTGRES_DSN unset"
        );
        return;
    };
    let baseline_rows = environment.channel_event_rows();
    let port = CoreProcess::free_port();
    let core = CoreProcess::start(&environment, port);
    let mut lin = Peer::open(core.base_url(), &environment.scratch("lin"), LIN_JIA, 0x11);
    let mut meng = Peer::open(
        core.base_url(),
        &environment.scratch("meng"),
        MENG_QING,
        0x22,
    );

    let channels = lin.client.list_channels().expect("list channels");
    assert!(
        channels
            .iter()
            .any(|channel| channel.channel_id == threadline_e2e_slice_a::CHANNEL),
        "seeded channel missing from ListChannels ({} channels)",
        channels.len()
    );

    both_ways(&mut lin, &mut meng);
    assert_eq!(environment.channel_event_rows() - baseline_rows, 2);
    let core = restart_replays_once(&environment, core, port, &mut lin, &mut meng);
    assert_eq!(environment.channel_event_rows() - baseline_rows, 3);
    lost_ack_retry_is_deduplicated(&mut lin);
    assert_eq!(
        environment.channel_event_rows() - baseline_rows,
        4,
        "exactly one row per logical event"
    );
    core.stop();
}

/// Lin sends, Meng pulls it; Meng replies, Lin pulls both in order.
fn both_ways(lin: &mut Peer, meng: &mut Peer) {
    lin.send("你好，孟青");
    let report = lin.replay();
    assert_eq!((report.committed, report.retained), (1, 0));

    let pulled = meng.pull().expect("meng pulls");
    assert!(pulled.applied >= 1, "meng applied nothing");
    let meng_view = meng.committed();
    let last = meng_view.last().expect("one event");
    assert_eq!(
        (last.1.as_str(), last.2.as_str()),
        ("actor-lin-jia", "你好，孟青")
    );

    meng.send("收到，林嘉");
    assert_eq!(meng.replay().committed, 1);
    lin.pull().expect("lin pulls");
    let lin_view = lin.committed();
    let tail: Vec<(&str, &str)> = lin_view
        .iter()
        .rev()
        .take(2)
        .map(|(_, actor, text)| (actor.as_str(), text.as_str()))
        .collect();
    assert_eq!(
        tail,
        [
            ("actor-meng-qing", "收到，林嘉"),
            ("actor-lin-jia", "你好，孟青")
        ]
    );
    assert!(
        lin_view.windows(2).all(|pair| pair[0].0 + 1 == pair[1].0),
        "sequence has a hole"
    );
}

/// With Core down the send stays queued and sync is retryable; after restart one
/// replay commits it exactly once and the other Device sees exactly one copy.
fn restart_replays_once(
    environment: &Environment,
    core: CoreProcess,
    port: u16,
    lin: &mut Peer,
    meng: &mut Peer,
) -> CoreProcess {
    core.stop();
    lin.send("离线时排队");
    let offline = lin.replay();
    assert_eq!((offline.committed, offline.retained), (0, 1));
    assert_eq!(lin.pull(), Err(LocaldError::Retryable));
    assert_eq!(lin.pending_count(), 1);

    let core = CoreProcess::start(environment, port);
    lin.reconnect(core.base_url());
    meng.reconnect(core.base_url());
    let recovered = lin.replay();
    assert_eq!(
        (
            recovered.committed,
            recovered.deduplicated,
            recovered.retained
        ),
        (1, 0, 0)
    );
    assert_eq!(lin.pending_count(), 0);
    assert_eq!(lin.replay().committed, 0, "second replay must find nothing");

    meng.pull().expect("meng pulls after restart");
    let copies = meng
        .committed()
        .iter()
        .filter(|(_, _, text)| text == "离线时排队")
        .count();
    assert_eq!(
        copies, 1,
        "queued message must appear exactly once on the peer"
    );
    core
}

/// The same pending bytes sent twice (a lost ACK) yield one committed event:
/// the second ACK is a deduplication at the same sequence, and the replay that
/// finally folds the pending row reports it as deduplicated, not committed.
fn lost_ack_retry_is_deduplicated(lin: &mut Peer) {
    let event_id = lin.send("ACK 丢失重试");
    let pending = lin
        .locald
        .pending(threadline_e2e_slice_a::CHANNEL)
        .expect("pending")
        .pop()
        .expect("queued item");
    let request = SendRequest {
        tenant_id: threadline_e2e_slice_a::TENANT.to_owned(),
        channel_id: threadline_e2e_slice_a::CHANNEL.to_owned(),
        event_id: event_id.clone(),
        idempotency_key: pending.idempotency_key.clone(),
        envelope_bytes: pending.envelope_bytes.clone(),
    };
    let first = lin.client.send(request.clone()).expect("first send");
    let second = lin.client.send(request).expect("retry send");
    assert!(!first.deduplicated && second.deduplicated);
    assert_eq!(first.channel_seq, second.channel_seq);
    assert_eq!(
        envelope::decode(&second.envelope_bytes)
            .expect("decode")
            .channel_seq,
        Some(first.channel_seq)
    );

    let folded = lin.replay();
    assert_eq!(
        (folded.committed, folded.deduplicated, folded.retained),
        (0, 1, 0)
    );
    assert_eq!(lin.pending_count(), 0);
    let committed = lin.committed();
    assert_eq!(
        committed
            .iter()
            .filter(|(_, _, text)| text == "ACK 丢失重试")
            .count(),
        1
    );
}

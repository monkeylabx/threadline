use super::*;
use threadline_client_proto::threadline::local_agent::v1::LocalToolPhase;

#[test]
fn normalization_filters_before_sequences_and_splits_utf8() {
    let mut activity = activity();
    activity
        .append("input-a", Update::Suppressed)
        .expect("filtered");
    append(&mut activity, "");
    activity
        .append(
            "input-a",
            Update::SyntheticTool(LocalToolPhase::Unspecified),
        )
        .expect("filtered");
    let text = format!("{}🦀{}", "x".repeat(8190), "é".repeat(4096));
    append(&mut activity, &text);
    for phase in [
        LocalToolPhase::Pending,
        LocalToolPhase::Running,
        LocalToolPhase::Finished,
        LocalToolPhase::Failed,
    ] {
        activity
            .append("input-a", Update::SyntheticTool(phase))
            .expect("fixed label");
    }
    let source = Source::base();
    let mut watch = activity.watch(&source, &first()).expect("watch");
    assert_eq!(ready(&mut watch, &source, &activity).1, 1);
    let frames = drain(&mut watch, &source, &activity);
    let mut reconstructed = String::new();
    for (index, frame) in frames.iter().enumerate() {
        assert_eq!(frame.sequence, (index + 1) as u64);
        assert_eq!(frame.input_id, "input-a");
        match frame.view.as_ref().expect("view") {
            View::AgentText(chunk) => {
                assert!(chunk.len() <= 8192);
                reconstructed.push_str(chunk);
            }
            View::Tool(tool) => {
                assert_eq!(tool.label, "Demonstration tool");
                assert!(tool.label.len() <= 128);
            }
            View::Ready(_) => panic!("ready not retained"),
        }
    }
    assert_eq!(reconstructed, text);
    assert_eq!(frames.len(), 7);
}

#[test]
fn count_limit_retains_128_then_reports_exact_gap() {
    let source = Source::base();
    let mut activity = activity();
    let mut old = activity.watch(&source, &first()).expect("watch");
    let (stream, _, _) = ready(&mut old, &source, &activity);
    for _ in 0..128 {
        append(&mut activity, "x");
    }
    let mut full = activity.watch(&source, &first()).expect("watch");
    assert_eq!(ready(&mut full, &source, &activity).1, 1);
    let retained = drain(&mut full, &source, &activity);
    assert_eq!(retained.len(), 128);
    assert!(retained.iter().map(Message::encoded_len).sum::<usize>() < 256 * 1024);
    let request = Request {
        stream_id: stream,
        ..first()
    };
    assert!(old.reconnect(&source, &activity, &request).is_ok());
    append(&mut activity, "x");
    assert_eq!(
        old.reconnect(&source, &activity, &request).err(),
        Some(Error::SequenceGap)
    );
    assert_eq!(old.next(&source, &activity), Err(Error::SequenceGap));
    let mut fresh = activity.watch(&source, &first()).expect("fresh");
    assert_eq!(ready(&mut fresh, &source, &activity).1, 2);
    let retained = drain(&mut fresh, &source, &activity);
    assert_eq!(
        (retained.len(), retained[0].sequence, retained[127].sequence),
        (128, 2, 129)
    );
}

#[test]
fn encoded_byte_limit_includes_metadata_and_exact_boundary() {
    let source = Source::base();
    let mut activity = activity();
    let mut old = activity.watch(&source, &first()).expect("watch");
    ready(&mut old, &source, &activity);
    for _ in 0..31 {
        append(&mut activity, &"x".repeat(8192));
    }
    let mut measure = activity.watch(&source, &first()).expect("measure");
    ready(&mut measure, &source, &activity);
    let retained = drain(&mut measure, &source, &activity);
    let remaining = 256 * 1024 - retained.iter().map(Message::encoded_len).sum::<usize>();
    let mut last = retained[0].clone();
    last.sequence = 32;
    last.view = Some(View::AgentText("x".repeat(128)));
    let overhead = last.encoded_len() - 128;
    assert!((128..=8192).contains(&(remaining - overhead)));
    append(&mut activity, &"x".repeat(remaining - overhead));
    let frames = drain(&mut old, &source, &activity);
    assert_eq!(frames.len(), 32);
    assert_eq!(
        frames.iter().map(Message::encoded_len).sum::<usize>(),
        256 * 1024
    );
    let mut lagging = activity.watch(&source, &first()).expect("lagging");
    ready(&mut lagging, &source, &activity);
    append(&mut activity, "é");
    assert_eq!(lagging.next(&source, &activity), Err(Error::SequenceGap));
    let mut fresh = activity.watch(&source, &first()).expect("fresh");
    assert_eq!(ready(&mut fresh, &source, &activity).1, 2);
    let frames = drain(&mut fresh, &source, &activity);
    assert_eq!(frames.len(), 32);
    assert!(frames.iter().map(Message::encoded_len).sum::<usize>() <= 256 * 1024);
}

#[test]
fn clear_preserves_loss_and_head_without_claiming_completion() {
    let source = Source::base();
    let mut activity = activity();
    let mut watch = activity.watch(&source, &first()).expect("watch");
    let (stream, floor, head) = ready(&mut watch, &source, &activity);
    assert_eq!((floor, head), (1, 0));
    assert!(watch.next(&source, &activity).expect("empty").is_none());
    append(&mut activity, "cleared");
    activity.clear();
    let mut request = Request {
        stream_id: stream,
        ..first()
    };
    assert_eq!(
        watch.reconnect(&source, &activity, &request).err(),
        Some(Error::SequenceGap)
    );
    request.after_sequence = 2;
    assert_eq!(
        watch.reconnect(&source, &activity, &request).err(),
        Some(Error::CursorInvalid)
    );
    request.after_sequence = 1;
    let mut resumed = watch
        .reconnect(&source, &activity, &request)
        .expect("at head");
    assert_eq!(ready(&mut resumed, &source, &activity).1, 2);
    assert!(resumed.next(&source, &activity).expect("empty").is_none());
    append(&mut activity, "later local activity");
    assert_eq!(frame(&mut resumed, &source, &activity).sequence, 2);
}

#[test]
fn input_bounds_and_debug_are_redacted() {
    let mut activity = activity();
    let sentinel = "SECRET-SENTINEL";
    for id in [
        "".to_owned(),
        "a b".to_owned(),
        "é".to_owned(),
        "x".repeat(129),
    ] {
        assert_eq!(
            activity.append(&id, Update::Text(sentinel)),
            Err(Error::InvalidArgument)
        );
        assert_eq!(
            Activity::new(id.clone(), "incarnation".into()).err(),
            Some(Error::InvalidArgument)
        );
        assert_eq!(
            Activity::new("run-a".into(), id).err(),
            Some(Error::InvalidArgument)
        );
    }
    assert_eq!(
        activity.append("input-a", Update::Text(&"x".repeat(65_537))),
        Err(Error::PayloadTooLarge)
    );
    append(&mut activity, &sentinel.repeat(4096));
    append(&mut activity, &"x".repeat(65_536));
    activity
        .append(&"i".repeat(128), Update::Text("x"))
        .expect("maximum input ID");
    Activity::new("r".repeat(128), "i".repeat(128)).expect("maximum trusted IDs");
    let source = Source::base();
    let mut watch = activity.watch(&source, &first()).expect("watch");
    let debug = format!("{activity:?} {watch:?} {}", Error::PermissionDenied);
    for value in [sentinel, "run-a", "incarnation-a", "actor-a", "input-a"] {
        assert!(!debug.contains(value));
    }
    assert_eq!(ready(&mut watch, &source, &activity).2, 17);
    assert_eq!(
        drain(&mut watch, &source, &activity)
            .last()
            .expect("last frame")
            .input_id
            .len(),
        128
    );
}

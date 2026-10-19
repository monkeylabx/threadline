// Codec compatibility only; these tests grant no authority or runtime admission.
use prost::Message;
use serde_json::Value;
use threadline_client_proto::threadline::local_agent::v1::{
    watch_run_activity_response::View, LocalToolPhase, WatchRunActivityResponse as Activity,
};

fn decode_hex(hex: &str) -> Vec<u8> {
    assert_eq!(hex.len() % 2, 0);
    (0..hex.len())
        .step_by(2)
        .map(|index| u8::from_str_radix(&hex[index..index + 2], 16).expect("fixture hex"))
        .collect()
}

fn assert_view(view: View, json: &Value) {
    match view {
        View::Ready(ready) => {
            assert_eq!(
                ready.first_available_sequence.to_string(),
                json["ready"]["firstAvailableSequence"]
            );
            assert_eq!(
                ready.latest_sequence.to_string(),
                json["ready"]["latestSequence"]
            );
        }
        View::AgentText(text) => assert_eq!(text, json["agentText"]),
        View::Tool(tool) => {
            assert_eq!(tool.label, json["tool"]["label"]);
            let phase = LocalToolPhase::try_from(tool.phase).expect("known golden phase");
            assert_eq!(phase.as_str_name(), json["tool"]["phase"]);
        }
    }
}

fn assert_golden(frame: &Value) {
    let bytes = decode_hex(frame["hex"].as_str().expect("fixture bytes"));
    let activity = Activity::decode(bytes.as_slice()).expect("canonical activity");
    let json = &frame["json"];
    assert_eq!(activity.run_id, json["runId"]);
    assert_eq!(activity.stream_id, json["streamId"]);
    assert_eq!(
        activity.sequence.to_string(),
        json["sequence"].as_str().unwrap_or("0")
    );
    assert_eq!(activity.input_id, json["inputId"].as_str().unwrap_or(""));
    assert_eq!(activity.encoded_len(), bytes.len());
    assert_eq!(activity.encode_to_vec(), bytes);
    assert_view(activity.view.expect("golden view"), json);
}

#[test]
fn canonical_ready_text_and_tool_frames_match_generated_rust() {
    let fixture: Value = serde_json::from_str(include_str!(
        "../../../test/fixtures/proto/agentd-local/wire.json"
    ))
    .expect("canonical C1 fixture");
    let mut count = 0;
    for frame in fixture["frames"].as_array().expect("frames") {
        if frame["type"] == "WatchRunActivityResponse" {
            assert_golden(frame);
            count += 1;
        }
    }
    assert_eq!(count, 3, "ready, text and tool golden variants");
}

#[test]
fn text_size_uses_utf8_and_protobuf_length_boundaries() {
    assert_eq!(Activity::default().encoded_len(), 0);
    // Independent expected bytes: one view tag, length varint, then UTF-8 data.
    for (text, bytes) in [
        (String::new(), 2),
        ("é".to_owned(), 4),
        ("x".repeat(127), 129),
        ("é".repeat(64), 131),
        ("x".repeat(16383), 16386),
        ("x".repeat(16384), 16388),
    ] {
        let activity = Activity {
            view: Some(View::AgentText(text)),
            ..Activity::default()
        };
        assert_eq!(activity.encoded_len(), bytes);
        assert_eq!(activity.encode_to_vec().len(), bytes);
    }
}

#[test]
fn size_includes_metadata_and_sequence_varints() {
    for (sequence, bytes) in [(0, 30), (127, 32), (128, 33), (u64::MAX, 41)] {
        let activity = Activity {
            run_id: "run-a".to_owned(),
            stream_id: "stream-a".to_owned(),
            input_id: "input-a".to_owned(),
            sequence,
            view: Some(View::AgentText("é".to_owned())),
        };
        assert_eq!(activity.encoded_len(), bytes);
        assert_eq!(activity.encode_to_vec().len(), bytes);
    }
    for (length, bytes) in [(127, 129), (128, 131)] {
        let activity = Activity {
            run_id: "r".repeat(length),
            ..Activity::default()
        };
        assert_eq!(activity.encoded_len(), bytes);
    }
}

#[test]
fn malformed_or_truncated_wire_is_not_a_valid_activity() {
    for bytes in [&[0x32, 0x02, 0xc3][..], &[0x32, 0x01, 0xff], &[0x18, 0x80]] {
        assert!(Activity::decode(bytes).is_err());
    }
}

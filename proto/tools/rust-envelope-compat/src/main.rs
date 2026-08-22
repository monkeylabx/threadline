use prost::Message;
use prost_reflect::{DescriptorPool, DynamicMessage, Value};
use std::{env, fs, path::Path};

const CANARY_FIELD: u32 = 50_000;

fn decode_hex(path: &Path) -> Vec<u8> {
    let source = fs::read_to_string(path).expect("read canonical hex fixture");
    let source = source.trim();
    assert!(
        source.len().is_multiple_of(2),
        "hex fixture must contain complete bytes"
    );
    assert!(
        source
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte)),
        "hex fixture must contain canonical lowercase hex",
    );
    (0..source.len())
        .step_by(2)
        .map(|offset| {
            u8::from_str_radix(&source[offset..offset + 2], 16).expect("canonical lowercase hex")
        })
        .collect()
}

fn canary_bytes(message: &DynamicMessage) -> Vec<Vec<u8>> {
    message
        .unknown_fields()
        .filter(|field| field.number() == CANARY_FIELD)
        .map(|field| {
            let mut encoded = Vec::new();
            field.encode(&mut encoded);
            encoded
        })
        .collect()
}

fn relay(
    pool: &DescriptorPool,
    descriptor_name: &str,
    input: &[u8],
    expected_canary: &[u8],
    mutation_field: &str,
    expected_value: &str,
    replacement_value: &str,
) -> Vec<u8> {
    let descriptor = pool
        .get_message_by_name(descriptor_name)
        .unwrap_or_else(|| panic!("missing descriptor {descriptor_name}"));
    let mut dynamic =
        DynamicMessage::decode(descriptor.clone(), input).expect("decode Golden Frame");

    assert_eq!(
        dynamic
            .get_field_by_name(mutation_field)
            .expect("known mutation field")
            .as_ref(),
        &Value::String(expected_value.to_owned()),
        "known field must have its documented semantic value",
    );
    assert_eq!(canary_bytes(&dynamic), vec![expected_canary.to_vec()]);

    let no_op = DynamicMessage::decode(descriptor.clone(), dynamic.encode_to_vec().as_slice())
        .expect("decode no-op round trip");
    assert_eq!(canary_bytes(&no_op), vec![expected_canary.to_vec()]);

    assert!(
        dynamic
            .try_set_field_by_name(mutation_field, Value::Bool(true))
            .is_err(),
        "a type-confused mutation must fail closed",
    );
    dynamic
        .try_set_field_by_name(mutation_field, Value::String(replacement_value.to_owned()))
        .expect("apply descriptor-checked known-field mutation");
    let encoded = dynamic.encode_to_vec();
    let roundtrip =
        DynamicMessage::decode(descriptor, encoded.as_slice()).expect("decode mutated round trip");

    assert_eq!(
        roundtrip
            .get_field_by_name(mutation_field)
            .expect("known mutation field")
            .as_ref(),
        &Value::String(replacement_value.to_owned()),
    );
    assert_eq!(canary_bytes(&roundtrip), vec![expected_canary.to_vec()]);
    encoded
}

fn verify_matrix(
    current: &DescriptorPool,
    previous: &DescriptorPool,
    descriptor_name: &str,
    frame_path: &Path,
    canary_path: &Path,
    mutation_field: &str,
    original_value: &str,
    prefix: &str,
) {
    let original = decode_hex(frame_path);
    let canary = decode_hex(canary_path);

    let current_first = relay(
        current,
        descriptor_name,
        &original,
        &canary,
        mutation_field,
        original_value,
        &format!("{prefix}-current-1"),
    );
    let previous_relay = relay(
        previous,
        descriptor_name,
        &current_first,
        &canary,
        mutation_field,
        &format!("{prefix}-current-1"),
        &format!("{prefix}-n-minus-one-1"),
    );
    let current_final = relay(
        current,
        descriptor_name,
        &previous_relay,
        &canary,
        mutation_field,
        &format!("{prefix}-n-minus-one-1"),
        &format!("{prefix}-current-2"),
    );
    relay(
        previous,
        descriptor_name,
        &current_final,
        &canary,
        mutation_field,
        &format!("{prefix}-current-2"),
        &format!("{prefix}-n-minus-one-final-check"),
    );

    let previous_first = relay(
        previous,
        descriptor_name,
        &original,
        &canary,
        mutation_field,
        original_value,
        &format!("{prefix}-n-minus-one-2"),
    );
    let current_relay = relay(
        current,
        descriptor_name,
        &previous_first,
        &canary,
        mutation_field,
        &format!("{prefix}-n-minus-one-2"),
        &format!("{prefix}-current-3"),
    );
    let previous_final = relay(
        previous,
        descriptor_name,
        &current_relay,
        &canary,
        mutation_field,
        &format!("{prefix}-current-3"),
        &format!("{prefix}-n-minus-one-3"),
    );
    relay(
        current,
        descriptor_name,
        &previous_final,
        &canary,
        mutation_field,
        &format!("{prefix}-n-minus-one-3"),
        &format!("{prefix}-current-final-check"),
    );
}

fn main() {
    let arguments: Vec<String> = env::args().collect();
    assert_eq!(
        arguments.len(),
        4,
        "usage: threadline-rust-envelope-compat <current-descriptor-set> <n-minus-one-descriptor-set> <golden-root>"
    );
    let current = DescriptorPool::decode(
        fs::read(&arguments[1])
            .expect("read current descriptor set")
            .as_slice(),
    )
    .expect("decode current descriptor set");
    let previous = DescriptorPool::decode(
        fs::read(&arguments[2])
            .expect("read N-1 descriptor set")
            .as_slice(),
    )
    .expect("decode N-1 descriptor set");
    let root = Path::new(&arguments[3]);

    verify_matrix(
        &current,
        &previous,
        "threadline.message.v1.ChannelEventEnvelope",
        &root.join("channel-event-envelope.golden.hex"),
        &root.join("ciphertext-envelope.canary.hex"),
        "event_id",
        "evt-golden-0001",
        "evt-rust",
    );
    verify_matrix(
        &current,
        &previous,
        "threadline.crypto.v1.RecoveryEnvelope",
        &root.join("recovery-envelope.golden.hex"),
        &root.join("crypto-envelope.canary.hex"),
        "recovery_key_id",
        "recovery-key-golden-v7",
        "recovery-key-rust",
    );

    println!(
        "Rust DynamicMessage preserved both field-50000 canaries through bidirectional current/N-1 read-mutate-write round trips."
    );
}

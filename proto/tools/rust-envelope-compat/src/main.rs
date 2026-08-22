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

fn verify(
    pool: &DescriptorPool,
    descriptor_name: &str,
    frame_path: &Path,
    canary_path: &Path,
    mutation_field: &str,
    original_value: &str,
    mutation_value: &str,
) {
    let descriptor = pool
        .get_message_by_name(descriptor_name)
        .unwrap_or_else(|| panic!("missing descriptor {descriptor_name}"));
    let original = decode_hex(frame_path);
    let expected_canary = decode_hex(canary_path);
    let mut dynamic = DynamicMessage::decode(descriptor.clone(), original.as_slice())
        .expect("decode Golden Frame");

    assert_eq!(
        dynamic
            .get_field_by_name(mutation_field)
            .expect("known mutation field")
            .as_ref(),
        &Value::String(original_value.to_owned()),
        "known field must have its documented semantic value",
    );
    assert_eq!(canary_bytes(&dynamic), vec![expected_canary.clone()]);

    let no_op = DynamicMessage::decode(descriptor.clone(), dynamic.encode_to_vec().as_slice())
        .expect("decode no-op round trip");
    assert_eq!(canary_bytes(&no_op), vec![expected_canary.clone()]);

    assert!(
        dynamic
            .try_set_field_by_name(mutation_field, Value::Bool(true))
            .is_err(),
        "a type-confused mutation must fail closed",
    );
    dynamic
        .try_set_field_by_name(mutation_field, Value::String(mutation_value.to_owned()))
        .expect("apply descriptor-checked known-field mutation");
    let roundtrip = DynamicMessage::decode(descriptor, dynamic.encode_to_vec().as_slice())
        .expect("decode mutated round trip");

    assert_eq!(
        roundtrip
            .get_field_by_name(mutation_field)
            .expect("known mutation field")
            .as_ref(),
        &Value::String(mutation_value.to_owned()),
    );
    assert_eq!(canary_bytes(&roundtrip), vec![expected_canary]);
}

fn main() {
    let arguments: Vec<String> = env::args().collect();
    assert_eq!(
        arguments.len(),
        3,
        "usage: threadline-rust-envelope-compat <descriptor-set> <golden-root>"
    );
    let pool = DescriptorPool::decode(
        fs::read(&arguments[1])
            .expect("read descriptor set")
            .as_slice(),
    )
    .expect("decode descriptor set");
    let root = Path::new(&arguments[2]);

    verify(
        &pool,
        "threadline.message.v1.ChannelEventEnvelope",
        &root.join("channel-event-envelope.golden.hex"),
        &root.join("ciphertext-envelope.canary.hex"),
        "event_id",
        "evt-golden-0001",
        "evt-rust-mutated",
    );
    verify(
        &pool,
        "threadline.crypto.v1.RecoveryEnvelope",
        &root.join("recovery-envelope.golden.hex"),
        &root.join("crypto-envelope.canary.hex"),
        "recovery_key_id",
        "recovery-key-golden-v7",
        "recovery-key-rust-mutated",
    );

    println!(
        "Rust DynamicMessage preserved both field-50000 canaries through no-op and known-field mutation round trips."
    );
}

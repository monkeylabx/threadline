use prost::Message;
use prost_reflect::{DescriptorPool, DynamicMessage, ReflectMessage, Value};
use std::{env, fs, path::Path};

const CANARY_FIELD: u32 = 50_000;
const PROTECTED_SCOPE_FIELD: u32 = 18;

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

fn unknown_field_bytes(message: &DynamicMessage, number: u32) -> Vec<Vec<u8>> {
    message
        .unknown_fields()
        .filter(|field| field.number() == number)
        .map(|field| {
            let mut encoded = Vec::new();
            field.encode(&mut encoded);
            encoded
        })
        .collect()
}

fn canary_bytes(message: &DynamicMessage) -> Vec<Vec<u8>> {
    unknown_field_bytes(message, CANARY_FIELD)
}

fn current_recovery_binding_values(message: &DynamicMessage) -> Option<Vec<Value>> {
    message.descriptor().get_field_by_name("scope_hash")?;
    let names = [
        "scope_hash",
        "scope_binding_hash",
        "recovery_case_id",
        "recovery_recipient_device_id",
        "delivery_policy_version",
        "delivery_expires_at",
        "delivery_binding_hash",
        "protected_scope",
    ];
    let values: Vec<Value> = names
        .iter()
        .map(|field| {
            message
                .get_field_by_name(field)
                .unwrap_or_else(|| panic!("current RecoveryEnvelope field {field}"))
                .as_ref()
                .clone()
        })
        .collect();
    let all_non_empty = values.iter().all(|value| match value {
        Value::Bytes(value) => !value.is_empty(),
        Value::String(value) => !value.is_empty(),
        Value::Message(value) => !value.encode_to_vec().is_empty(),
        _ => false,
    });
    all_non_empty.then_some(values)
}

struct CurrentRecoveryBindings {
    current_values: Vec<Value>,
    previous_unknown_fields: Vec<(u32, Vec<Vec<u8>>)>,
}

struct RelayContract<'a> {
    descriptor_name: &'a str,
    expected_canary: &'a [u8],
    mutation_field: &'a str,
    expected_value: &'a str,
    replacement_value: &'a str,
    expected_current_recovery_bindings: Option<&'a CurrentRecoveryBindings>,
}

struct MatrixContract<'a> {
    descriptor_name: &'a str,
    frame_path: &'a Path,
    canary_path: &'a Path,
    mutation_field: &'a str,
    original_value: &'a str,
    prefix: &'a str,
    require_current_recovery_bindings: bool,
}

fn assert_current_recovery_bindings(
    message: &DynamicMessage,
    expected: Option<&CurrentRecoveryBindings>,
) {
    let Some(expected) = expected else { return };
    if message
        .descriptor()
        .get_field_by_name("scope_hash")
        .is_some()
    {
        let actual = current_recovery_binding_values(message)
            .expect("current RecoveryEnvelope fields 11-18 must remain non-empty");
        assert_eq!(
            actual, expected.current_values,
            "current RecoveryEnvelope binding/delivery fields changed"
        );
    } else {
        for (number, expected_bytes) in &expected.previous_unknown_fields {
            assert_eq!(
                unknown_field_bytes(message, *number),
                *expected_bytes,
                "N-1 relay changed unknown current-only field {number}"
            );
        }
    }
}

fn relay(pool: &DescriptorPool, input: &[u8], contract: RelayContract<'_>) -> Vec<u8> {
    let RelayContract {
        descriptor_name,
        expected_canary,
        mutation_field,
        expected_value,
        replacement_value,
        expected_current_recovery_bindings,
    } = contract;
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
    assert_current_recovery_bindings(&dynamic, expected_current_recovery_bindings);

    let no_op = DynamicMessage::decode(descriptor.clone(), dynamic.encode_to_vec().as_slice())
        .expect("decode no-op round trip");
    assert_eq!(canary_bytes(&no_op), vec![expected_canary.to_vec()]);
    assert_current_recovery_bindings(&no_op, expected_current_recovery_bindings);

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
    assert_current_recovery_bindings(&roundtrip, expected_current_recovery_bindings);
    encoded
}

fn verify_matrix(
    current: &DescriptorPool,
    previous: &DescriptorPool,
    contract: MatrixContract<'_>,
) {
    let MatrixContract {
        descriptor_name,
        frame_path,
        canary_path,
        mutation_field,
        original_value,
        prefix,
        require_current_recovery_bindings,
    } = contract;
    let original = decode_hex(frame_path);
    let canary = decode_hex(canary_path);
    let expected_current_recovery_bindings = if require_current_recovery_bindings {
        let descriptor = current
            .get_message_by_name(descriptor_name)
            .unwrap_or_else(|| panic!("missing descriptor {descriptor_name}"));
        let message = DynamicMessage::decode(descriptor.clone(), original.as_slice())
            .expect("decode current RecoveryEnvelope fixture");
        let previous_descriptor = previous
            .get_message_by_name(descriptor_name)
            .unwrap_or_else(|| panic!("missing N-1 descriptor {descriptor_name}"));
        let previous_message = DynamicMessage::decode(previous_descriptor, original.as_slice())
            .expect("decode current RecoveryEnvelope with N-1 descriptor");
        let previous_unknown_fields = (11..=PROTECTED_SCOPE_FIELD)
            .map(|number| {
                let bytes = unknown_field_bytes(&previous_message, number);
                assert!(
                    !bytes.is_empty(),
                    "N-1 descriptor must treat current-only field {number} as unknown"
                );
                (number, bytes)
            })
            .collect();
        Some(CurrentRecoveryBindings {
            current_values: current_recovery_binding_values(&message)
                .expect("current RecoveryEnvelope binding fields 11-18"),
            previous_unknown_fields,
        })
    } else {
        None
    };
    let expected_current_recovery_bindings = expected_current_recovery_bindings.as_ref();

    let current_first = relay(
        current,
        &original,
        RelayContract {
            descriptor_name,
            expected_canary: &canary,
            mutation_field,
            expected_value: original_value,
            replacement_value: &format!("{prefix}-current-1"),
            expected_current_recovery_bindings,
        },
    );
    let previous_relay = relay(
        previous,
        &current_first,
        RelayContract {
            descriptor_name,
            expected_canary: &canary,
            mutation_field,
            expected_value: &format!("{prefix}-current-1"),
            replacement_value: &format!("{prefix}-n-minus-one-1"),
            expected_current_recovery_bindings,
        },
    );
    let current_final = relay(
        current,
        &previous_relay,
        RelayContract {
            descriptor_name,
            expected_canary: &canary,
            mutation_field,
            expected_value: &format!("{prefix}-n-minus-one-1"),
            replacement_value: &format!("{prefix}-current-2"),
            expected_current_recovery_bindings,
        },
    );
    relay(
        previous,
        &current_final,
        RelayContract {
            descriptor_name,
            expected_canary: &canary,
            mutation_field,
            expected_value: &format!("{prefix}-current-2"),
            replacement_value: &format!("{prefix}-n-minus-one-final-check"),
            expected_current_recovery_bindings,
        },
    );

    let previous_first = relay(
        previous,
        &original,
        RelayContract {
            descriptor_name,
            expected_canary: &canary,
            mutation_field,
            expected_value: original_value,
            replacement_value: &format!("{prefix}-n-minus-one-2"),
            expected_current_recovery_bindings,
        },
    );
    let current_relay = relay(
        current,
        &previous_first,
        RelayContract {
            descriptor_name,
            expected_canary: &canary,
            mutation_field,
            expected_value: &format!("{prefix}-n-minus-one-2"),
            replacement_value: &format!("{prefix}-current-3"),
            expected_current_recovery_bindings,
        },
    );
    let previous_final = relay(
        previous,
        &current_relay,
        RelayContract {
            descriptor_name,
            expected_canary: &canary,
            mutation_field,
            expected_value: &format!("{prefix}-current-3"),
            replacement_value: &format!("{prefix}-n-minus-one-3"),
            expected_current_recovery_bindings,
        },
    );
    relay(
        current,
        &previous_final,
        RelayContract {
            descriptor_name,
            expected_canary: &canary,
            mutation_field,
            expected_value: &format!("{prefix}-n-minus-one-3"),
            replacement_value: &format!("{prefix}-current-final-check"),
            expected_current_recovery_bindings,
        },
    );
}

fn main() {
    let arguments: Vec<String> = env::args().collect();
    assert!(
        arguments.len() == 4
            || (arguments.len() == 6 && arguments[4] == "--require-current-recovery-bindings"),
        "usage: threadline-rust-envelope-compat <current-descriptor-set> <n-minus-one-descriptor-set> <frame-root> [--require-current-recovery-bindings <legacy-recovery-frame>]"
    );
    let require_current_recovery_bindings = arguments
        .get(4)
        .is_some_and(|value| value == "--require-current-recovery-bindings");
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
        MatrixContract {
            descriptor_name: "threadline.message.v1.ChannelEventEnvelope",
            frame_path: &root.join("channel-event-envelope.golden.hex"),
            canary_path: &root.join("ciphertext-envelope.canary.hex"),
            mutation_field: "event_id",
            original_value: "evt-golden-0001",
            prefix: "evt-rust",
            require_current_recovery_bindings: false,
        },
    );
    verify_matrix(
        &current,
        &previous,
        MatrixContract {
            descriptor_name: "threadline.crypto.v1.RecoveryEnvelope",
            frame_path: &root.join("recovery-envelope.golden.hex"),
            canary_path: &root.join("crypto-envelope.canary.hex"),
            mutation_field: "recovery_key_id",
            original_value: "recovery-key-golden-v7",
            prefix: "recovery-key-rust",
            require_current_recovery_bindings,
        },
    );

    if require_current_recovery_bindings {
        let legacy_frame = Path::new(&arguments[5]);
        verify_matrix(
            &current,
            &previous,
            MatrixContract {
                descriptor_name: "threadline.crypto.v1.RecoveryEnvelope",
                frame_path: legacy_frame,
                canary_path: &root.join("crypto-envelope.canary.hex"),
                mutation_field: "recovery_key_id",
                original_value: "recovery-key-golden-v7",
                prefix: "recovery-key-rust-legacy",
                require_current_recovery_bindings: false,
            },
        );
        let descriptor = current
            .get_message_by_name("threadline.crypto.v1.RecoveryEnvelope")
            .expect("current RecoveryEnvelope descriptor");
        let legacy = DynamicMessage::decode(descriptor, decode_hex(legacy_frame).as_slice())
            .expect("decode binding-less N-1 RecoveryEnvelope");
        assert!(
            current_recovery_binding_values(&legacy).is_none()
                || legacy
                    .get_field_by_name("protected_scope")
                    .is_none_or(|value| match value.as_ref() {
                        Value::Message(value) => value.encode_to_vec().is_empty(),
                        _ => true,
                    }),
            "binding-less N-1 RecoveryEnvelope must fail the current scoped Execute gate"
        );
    }

    println!(
        "Rust DynamicMessage preserved field-50000, current field 18, and binding-less legacy bytes through bidirectional current/N-1 round trips; legacy scoped Execute failed closed."
    );
}

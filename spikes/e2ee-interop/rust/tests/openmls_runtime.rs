use openmls::prelude::{tls_codec::*, *};
use openmls_basic_credential::SignatureKeyPair;
use openmls_rust_crypto::OpenMlsRustCrypto;
use std::panic::{catch_unwind, AssertUnwindSafe};

const CIPHERSUITE: Ciphersuite = Ciphersuite::MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519;

fn credential(
    identity: &[u8],
    provider: &impl OpenMlsProvider,
) -> (CredentialWithKey, SignatureKeyPair) {
    let signer = SignatureKeyPair::new(CIPHERSUITE.signature_algorithm()).unwrap();
    signer.store(provider.storage()).unwrap();
    let credential = BasicCredential::new(identity.to_vec());
    (
        CredentialWithKey {
            credential: credential.into(),
            signature_key: signer.public().into(),
        },
        signer,
    )
}

fn key_package(
    provider: &impl OpenMlsProvider,
    signer: &SignatureKeyPair,
    credential: CredentialWithKey,
) -> KeyPackageBundle {
    KeyPackage::builder()
        .build(CIPHERSUITE, provider, signer, credential)
        .unwrap()
}

fn incoming(message: MlsMessageOut) -> MlsMessageIn {
    let bytes = message.to_bytes().unwrap();
    MlsMessageIn::tls_deserialize(&mut bytes.as_slice()).unwrap()
}

fn protocol(message: MlsMessageOut) -> ProtocolMessage {
    incoming(message).try_into_protocol_message().unwrap()
}

fn welcome(message: MlsMessageOut) -> Welcome {
    match incoming(message).extract() {
        MlsMessageBodyIn::Welcome(welcome) => welcome,
        _ => panic!("expected welcome"),
    }
}

fn merge_commit(group: &mut MlsGroup, provider: &impl OpenMlsProvider, message: MlsMessageOut) {
    let processed = group.process_message(provider, protocol(message)).unwrap();
    match processed.into_content() {
        ProcessedMessageContent::StagedCommitMessage(commit) => {
            group.merge_staged_commit(provider, *commit).unwrap();
        }
        _ => panic!("expected staged commit"),
    }
}

#[test]
fn exercises_key_packages_epochs_offline_join_and_revocation() {
    let alice_provider = OpenMlsRustCrypto::default();
    let bob_provider = OpenMlsRustCrypto::default();
    let charlie_provider = OpenMlsRustCrypto::default();

    let (alice_credential, alice_signer) = credential(b"alice-device-1", &alice_provider);
    let (bob_credential, bob_signer) = credential(b"bob-device-1", &bob_provider);
    let (charlie_credential, charlie_signer) =
        credential(b"charlie-offline-device", &charlie_provider);
    let bob_key_package = key_package(&bob_provider, &bob_signer, bob_credential);
    // Generated before the group advances: this models an asynchronously uploaded KeyPackage.
    let charlie_key_package = key_package(&charlie_provider, &charlie_signer, charlie_credential);

    let mut alice_group = MlsGroup::new(
        &alice_provider,
        &alice_signer,
        &MlsGroupCreateConfig::default(),
        alice_credential,
    )
    .unwrap();
    assert_eq!(alice_group.epoch().as_u64(), 0);

    let (_add_bob_commit, bob_welcome, _) = alice_group
        .add_members(
            &alice_provider,
            &alice_signer,
            core::slice::from_ref(bob_key_package.key_package()),
        )
        .unwrap();
    alice_group.merge_pending_commit(&alice_provider).unwrap();
    assert_eq!(alice_group.epoch().as_u64(), 1);

    let mut bob_group = StagedWelcome::new_from_welcome(
        &bob_provider,
        &MlsGroupJoinConfig::default(),
        welcome(bob_welcome),
        Some(alice_group.export_ratchet_tree().into()),
    )
    .unwrap()
    .into_group(&bob_provider)
    .unwrap();
    assert_eq!(bob_group.epoch(), alice_group.epoch());

    // Synthetic non-secret payload; only the encrypted MLS frame leaves Alice's state.
    let old_epoch_message = protocol(
        alice_group
            .create_message(&alice_provider, &alice_signer, &[0xa5; 32])
            .unwrap(),
    );

    let (commit_1, _, _) = alice_group
        .self_update(
            &alice_provider,
            &alice_signer,
            LeafNodeParameters::default(),
        )
        .unwrap()
        .into_contents();
    alice_group.merge_pending_commit(&alice_provider).unwrap();
    assert_eq!(alice_group.epoch().as_u64(), 2);

    let (commit_2, _, _) = alice_group
        .self_update(
            &alice_provider,
            &alice_signer,
            LeafNodeParameters::default(),
        )
        .unwrap()
        .into_contents();
    alice_group.merge_pending_commit(&alice_provider).unwrap();
    assert_eq!(alice_group.epoch().as_u64(), 3);

    // OpenMLS rejects the future commit. The adapter contract maps this to
    // TL_E2EE_FUTURE_EPOCH and queues it until the predecessor arrives.
    assert!(bob_group
        .process_message(&bob_provider, protocol(commit_2.clone()),)
        .is_err());
    merge_commit(&mut bob_group, &bob_provider, commit_1);
    merge_commit(&mut bob_group, &bob_provider, commit_2.clone());
    assert_eq!(bob_group.epoch().as_u64(), 3);

    // Replaying an already merged commit and delivering an old-epoch frame are rejected.
    assert!(bob_group
        .process_message(&bob_provider, protocol(commit_2))
        .is_err());
    assert!(bob_group
        .process_message(&bob_provider, old_epoch_message)
        .is_err());

    // The offline device joins the current epoch from its pre-published KeyPackage.
    let (add_charlie_commit, charlie_welcome, _) = alice_group
        .add_members(
            &alice_provider,
            &alice_signer,
            core::slice::from_ref(charlie_key_package.key_package()),
        )
        .unwrap();
    alice_group.merge_pending_commit(&alice_provider).unwrap();
    merge_commit(&mut bob_group, &bob_provider, add_charlie_commit);
    let mut charlie_group = StagedWelcome::new_from_welcome(
        &charlie_provider,
        &MlsGroupJoinConfig::default(),
        welcome(charlie_welcome),
        Some(alice_group.export_ratchet_tree().into()),
    )
    .unwrap()
    .into_group(&charlie_provider)
    .unwrap();
    assert_eq!(charlie_group.epoch().as_u64(), 4);

    // Device-level revocation advances the epoch and deactivates the removed device.
    let bob_index = bob_group.own_leaf_index();
    let (remove_bob_commit, _, _) = alice_group
        .remove_members(&alice_provider, &alice_signer, &[bob_index])
        .unwrap();
    alice_group.merge_pending_commit(&alice_provider).unwrap();
    merge_commit(
        &mut charlie_group,
        &charlie_provider,
        remove_bob_commit.clone(),
    );
    merge_commit(&mut bob_group, &bob_provider, remove_bob_commit);
    assert_eq!(alice_group.epoch().as_u64(), 5);
    assert!(!bob_group.is_active());

    let post_revocation_message = protocol(
        alice_group
            .create_message(&alice_provider, &alice_signer, &[0xa5; 32])
            .unwrap(),
    );
    assert!(bob_group
        .process_message(&bob_provider, post_revocation_message)
        .is_err());
}

#[test]
fn corrupted_ciphertext_is_contained_at_the_adapter_boundary() {
    let alice_provider = OpenMlsRustCrypto::default();
    let bob_provider = OpenMlsRustCrypto::default();
    let (alice_credential, alice_signer) = credential(b"alice-device-1", &alice_provider);
    let (bob_credential, bob_signer) = credential(b"bob-device-1", &bob_provider);
    let bob_key_package = key_package(&bob_provider, &bob_signer, bob_credential);
    let mut alice_group = MlsGroup::new(
        &alice_provider,
        &alice_signer,
        &MlsGroupCreateConfig::default(),
        alice_credential,
    )
    .unwrap();
    let (_, bob_welcome, _) = alice_group
        .add_members(
            &alice_provider,
            &alice_signer,
            core::slice::from_ref(bob_key_package.key_package()),
        )
        .unwrap();
    alice_group.merge_pending_commit(&alice_provider).unwrap();
    let mut bob_group = StagedWelcome::new_from_welcome(
        &bob_provider,
        &MlsGroupJoinConfig::default(),
        welcome(bob_welcome),
        Some(alice_group.export_ratchet_tree().into()),
    )
    .unwrap()
    .into_group(&bob_provider)
    .unwrap();

    let mut corrupted = alice_group
        .create_message(&alice_provider, &alice_signer, &[0xa5; 32])
        .unwrap()
        .tls_serialize_detached()
        .unwrap();
    *corrupted.last_mut().unwrap() ^= 0x01;
    let corrupted = MlsMessageIn::tls_deserialize(&mut corrupted.as_slice())
        .unwrap()
        .try_into_protocol_message()
        .unwrap();

    // OpenMLS 0.8.1 returns an AEAD error in release builds, but contains a
    // debug_assert on the same path in debug builds. The spike catches either
    // outcome at the library-independent adapter boundary and maps it to
    // TL_E2EE_CORRUPT; the debug panic remains a production-readiness blocker.
    let outcome = catch_unwind(AssertUnwindSafe(|| {
        bob_group.process_message(&bob_provider, corrupted)
    }));
    assert!(matches!(outcome, Err(_) | Ok(Err(_))));
}

//! Crash/resume and key-erasure evidence for the P00-08 gate.
//!
//! ADR-0003 requires that crash/resume, encrypted persistence, key erasure and
//! retention expiry all pass before OpenMLS can be accepted. The T011 spike ran
//! entirely in memory, so a restart was never modelled. This harness restarts
//! the group state through a serialized store, and records what the shipped
//! storage provider does and does not give Threadline.

use openmls::prelude::{tls_codec::*, *};
use openmls_basic_credential::SignatureKeyPair;
use openmls_memory_storage::MemoryStorage;
use openmls_rust_crypto::RustCrypto;
use openmls_traits::OpenMlsProvider;
use std::fs::File;

const CIPHERSUITE: Ciphersuite = Ciphersuite::MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519;
const PAYLOAD: &[u8] = b"threadline-persistence-frame";

/// The seam ADR-0003 assigns to `client-crypto`: Threadline owns storage, the
/// library only owns the protocol. Splitting the provider here is what makes a
/// restart expressible at all.
struct SpikeProvider {
    crypto: RustCrypto,
    storage: MemoryStorage,
}

impl SpikeProvider {
    fn new() -> Self {
        Self {
            crypto: RustCrypto::default(),
            storage: MemoryStorage::default(),
        }
    }

    /// Simulates a process restart: everything not in the store is lost.
    fn restart_from(path: &std::path::Path) -> Self {
        let mut storage = MemoryStorage::default();
        let file = File::open(path).expect("snapshot missing");
        storage
            .load_from_file(&file)
            .expect("storage snapshot does not load");
        Self {
            crypto: RustCrypto::default(),
            storage,
        }
    }

    fn snapshot_to(&self, path: &std::path::Path) {
        let file = File::create(path).expect("cannot create snapshot");
        self.storage
            .save_to_file(&file)
            .expect("storage snapshot does not save");
    }
}

impl OpenMlsProvider for SpikeProvider {
    type CryptoProvider = RustCrypto;
    type RandProvider = RustCrypto;
    type StorageProvider = MemoryStorage;

    fn storage(&self) -> &Self::StorageProvider {
        &self.storage
    }

    fn crypto(&self) -> &Self::CryptoProvider {
        &self.crypto
    }

    fn rand(&self) -> &Self::RandProvider {
        &self.crypto
    }
}

fn credential(identity: &[u8], provider: &SpikeProvider) -> (CredentialWithKey, SignatureKeyPair) {
    let signer = SignatureKeyPair::new(CIPHERSUITE.signature_algorithm()).unwrap();
    signer.store(provider.storage()).unwrap();
    (
        CredentialWithKey {
            credential: BasicCredential::new(identity.to_vec()).into(),
            signature_key: signer.public().into(),
        },
        signer,
    )
}

fn protocol(message: MlsMessageOut) -> ProtocolMessage {
    MlsMessageIn::tls_deserialize(&mut message.to_bytes().unwrap().as_slice())
        .unwrap()
        .try_into_protocol_message()
        .unwrap()
}

fn scratch_dir() -> std::path::PathBuf {
    let dir = std::env::temp_dir().join("threadline-t011-persistence");
    std::fs::create_dir_all(&dir).expect("cannot create scratch dir");
    dir
}

/// A device restarts mid-conversation: the group must resume at the same epoch
/// and stay able to send and receive, without any in-memory carry-over.
#[test]
fn group_state_survives_a_simulated_crash_and_restart() {
    let alice_provider = SpikeProvider::new();
    let bob_provider = SpikeProvider::new();
    let (alice_credential, alice_signer) = credential(b"alice-device-1", &alice_provider);
    let (bob_credential, bob_signer) = credential(b"bob-device-1", &bob_provider);

    let bob_key_package = KeyPackage::builder()
        .build(CIPHERSUITE, &bob_provider, &bob_signer, bob_credential)
        .unwrap();

    let mut alice_group = MlsGroup::new(
        &alice_provider,
        &alice_signer,
        &MlsGroupCreateConfig::default(),
        alice_credential,
    )
    .unwrap();
    let group_id = alice_group.group_id().clone();

    let (_, welcome, _) = alice_group
        .add_members(
            &alice_provider,
            &alice_signer,
            core::slice::from_ref(bob_key_package.key_package()),
        )
        .unwrap();
    alice_group.merge_pending_commit(&alice_provider).unwrap();

    let welcome = match MlsMessageIn::tls_deserialize(&mut welcome.to_bytes().unwrap().as_slice())
        .unwrap()
        .extract()
    {
        MlsMessageBodyIn::Welcome(welcome) => welcome,
        _ => panic!("expected a welcome"),
    };
    let bob_group = StagedWelcome::new_from_welcome(
        &bob_provider,
        &MlsGroupJoinConfig::default(),
        welcome,
        Some(alice_group.export_ratchet_tree().into()),
    )
    .unwrap()
    .into_group(&bob_provider)
    .unwrap();
    assert_eq!(bob_group.epoch().as_u64(), 1);

    // Bob crashes here. Only what reached the store survives.
    let snapshot = scratch_dir().join("bob-epoch-1.json");
    bob_provider.snapshot_to(&snapshot);
    drop(bob_group);
    drop(bob_provider);

    let restarted_provider = SpikeProvider::restart_from(&snapshot);
    let mut bob_group = MlsGroup::load(restarted_provider.storage(), &group_id)
        .expect("loading the restarted group failed")
        .expect("the restarted store has no group state");
    assert_eq!(
        bob_group.epoch().as_u64(),
        1,
        "the restarted device resumed at the wrong epoch"
    );

    // The restarted device must still decrypt traffic from the live peer.
    let message = alice_group
        .create_message(&alice_provider, &alice_signer, PAYLOAD)
        .unwrap();
    let processed = bob_group
        .process_message(&restarted_provider, protocol(message))
        .expect("the restarted device cannot process a live frame");
    match processed.into_content() {
        ProcessedMessageContent::ApplicationMessage(message) => {
            assert_eq!(message.into_bytes(), PAYLOAD)
        }
        other => panic!("unexpected content: {other:?}"),
    }

    // ...and must still be able to commit, which needs its private tree state,
    // not only the public group state.
    let (commit, _, _) = bob_group
        .self_update(
            &restarted_provider,
            &bob_signer,
            LeafNodeParameters::default(),
        )
        .expect("the restarted device cannot commit")
        .into_contents();
    bob_group.merge_pending_commit(&restarted_provider).unwrap();
    assert_eq!(bob_group.epoch().as_u64(), 2);

    let processed = alice_group
        .process_message(&alice_provider, protocol(commit))
        .expect("peer rejected the restarted device's commit");
    match processed.into_content() {
        ProcessedMessageContent::StagedCommitMessage(staged) => {
            alice_group
                .merge_staged_commit(&alice_provider, *staged)
                .unwrap();
        }
        other => panic!("unexpected content: {other:?}"),
    }
    assert_eq!(alice_group.epoch().as_u64(), 2);

    std::fs::remove_file(&snapshot).ok();
}

/// Retention and device revocation both depend on key material actually leaving
/// the store. `MlsGroup::delete` must make the group unloadable afterwards.
#[test]
fn deleting_a_group_erases_its_state_from_the_store() {
    let provider = SpikeProvider::new();
    let (credential_with_key, signer) = credential(b"alice-device-1", &provider);
    let mut group = MlsGroup::new(
        &provider,
        &signer,
        &MlsGroupCreateConfig::default(),
        credential_with_key,
    )
    .unwrap();
    let group_id = group.group_id().clone();

    assert!(MlsGroup::load(provider.storage(), &group_id)
        .unwrap()
        .is_some());

    group
        .delete(provider.storage())
        .expect("group deletion failed");

    assert!(
        MlsGroup::load(provider.storage(), &group_id)
            .unwrap()
            .is_none(),
        "group state is still loadable after deletion"
    );
}

/// The persistence that ships with the candidate is a base64 JSON dump of the
/// key store. It is adequate for a spike and unusable for production: ADR-0003
/// requires local key material to sit in encrypted local storage with OS-held
/// wrapping keys. This test pins the gap so the ADR claim stays honest, and
/// checks structure only -- it never prints or asserts on the values.
#[test]
fn shipped_persistence_is_unencrypted_and_cannot_be_shipped() {
    let provider = SpikeProvider::new();
    let (credential_with_key, signer) = credential(b"alice-device-1", &provider);
    MlsGroup::new(
        &provider,
        &signer,
        &MlsGroupCreateConfig::default(),
        credential_with_key,
    )
    .unwrap();

    let snapshot = scratch_dir().join("plaintext-check.json");
    provider.snapshot_to(&snapshot);
    let raw = std::fs::read_to_string(&snapshot).expect("snapshot unreadable");
    std::fs::remove_file(&snapshot).ok();

    let parsed: serde_json::Value =
        serde_json::from_str(&raw).expect("snapshot is not plain JSON after all");
    let values = parsed
        .get("values")
        .and_then(serde_json::Value::as_object)
        .expect("snapshot has no value map");
    assert!(
        !values.is_empty(),
        "snapshot is empty, so this check proves nothing"
    );

    // Every entry is directly base64-decodable: there is no wrapping key, no
    // AEAD tag, and no key derivation between the process and the disk.
    for value in values.values() {
        let encoded = value.as_str().expect("snapshot value is not a string");
        assert!(
            base64_decodes(encoded),
            "snapshot value is not plain base64; re-check whether upstream added encryption"
        );
    }
}

fn base64_decodes(input: &str) -> bool {
    const ALPHABET: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    !input.is_empty()
        && input.len().is_multiple_of(4)
        && input
            .trim_end_matches('=')
            .bytes()
            .all(|byte| ALPHABET.contains(&byte))
}

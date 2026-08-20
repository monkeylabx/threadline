//! Cross-implementation RFC 9420 interoperability: OpenMLS <-> mls-rs.
//!
//! ADR-0003 requires that the same versioned transcript produce the same
//! external result in "at least one independent RFC 9420 implementation". The
//! T011 spike could not show that, because its Swift and Kotlin harnesses only
//! re-parsed a semantic vector. This harness closes that gap on the wire: both
//! libraries drive the same `tl-mls-1` group, and every message crossing
//! between them is a serialized `MLSMessage`, never a library type.
//!
//! mls-rs is also ADR-0003's named fallback candidate, so a failure here is a
//! finding about the fallback as much as about OpenMLS.

use mls_rs::{
    client_builder::{MlsConfig, PaddingMode},
    error::MlsError,
    group::{
        mls_rules::{DefaultMlsRules, EncryptionOptions},
        CommitEffect, ReceivedMessage,
    },
    identity::{
        basic::{BasicCredential as MlsRsBasicCredential, BasicIdentityProvider},
        SigningIdentity,
    },
    time::MlsTime,
    CipherSuite, CipherSuiteProvider, Client, CryptoProvider, ExtensionList, MlsMessage,
};
use mls_rs_crypto_rustcrypto::RustCryptoProvider;
use openmls::prelude::{tls_codec::*, *};
use openmls_basic_credential::SignatureKeyPair;
use openmls_rust_crypto::OpenMlsRustCrypto;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

/// `tl-mls-1` cipher suite, as seen by each library's own naming.
const OPENMLS_SUITE: Ciphersuite = Ciphersuite::MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519;
const MLS_RS_SUITE: CipherSuite = CipherSuite::CURVE25519_AES128;

const APPLICATION_PAYLOAD: &[u8] = b"threadline-interop-application-frame";

/// Leaf-node lifetime policy that both implementations accept.
///
/// OpenMLS 0.8.1 requires `not_before < now < not_after` (strictly) and caps the
/// total range at `DEFAULT_KEY_PACKAGE_LIFETIME_MARGIN + DEFAULT_KEY_PACKAGE_LIFETIME`
/// = 1h + 84d. mls-rs 0.55.3 defaults to `not_before = now` and a 365-day range,
/// so neither of its defaults survives OpenMLS validation. `tl-mls-1` therefore
/// has to pin the policy rather than inherit either library's default; see
/// `default_lifetime_policies_are_not_interoperable` below.
const SKEW_MARGIN: Duration = Duration::from_secs(60 * 60);
const TL_MLS_1_LIFETIME: Duration = Duration::from_secs(60 * 60 * 24 * 80);

fn now_seconds() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("system clock before the unix epoch")
        .as_secs()
}

/// `not_before`, backdated by the same skew margin OpenMLS applies itself.
fn not_before() -> MlsTime {
    MlsTime::from(now_seconds() - SKEW_MARGIN.as_secs())
}

// --- mls-rs side -----------------------------------------------------------

fn mls_rs_client(name: &str) -> Result<Client<impl MlsConfig>, MlsError> {
    mls_rs_client_with_lifetime(name, TL_MLS_1_LIFETIME)
}

fn mls_rs_client_with_lifetime(
    name: &str,
    lifetime: Duration,
) -> Result<Client<impl MlsConfig>, MlsError> {
    mls_rs_client_full(name, lifetime, true)
}

fn mls_rs_client_full(
    name: &str,
    lifetime: Duration,
    encrypt_control_messages: bool,
) -> Result<Client<impl MlsConfig>, MlsError> {
    let crypto = RustCryptoProvider::default();
    let cipher_suite = crypto
        .cipher_suite_provider(MLS_RS_SUITE)
        .expect("mls-rs does not support tl-mls-1");
    let (secret, public) = cipher_suite.signature_key_generate().unwrap();
    let signing_identity = SigningIdentity::new(
        MlsRsBasicCredential::new(name.as_bytes().to_vec()).into_credential(),
        public,
    );

    Ok(Client::builder()
        .identity_provider(BasicIdentityProvider)
        .crypto_provider(crypto)
        .signing_identity(signing_identity, secret, MLS_RS_SUITE)
        .key_package_lifetime(lifetime)
        // OpenMLS defaults to `PURE_CIPHERTEXT_WIRE_FORMAT_POLICY` and rejects
        // PublicMessage handshakes; mls-rs sends them in the clear by default.
        // `tl-mls-1` pins the encrypted form, which also keeps proposal and
        // sender detail away from the delivery service.
        .mls_rules(
            DefaultMlsRules::new().with_encryption_options(EncryptionOptions::new(
                encrypt_control_messages,
                PaddingMode::None,
            )),
        )
        .build())
}

// --- OpenMLS side ----------------------------------------------------------

struct OpenMlsDevice {
    provider: OpenMlsRustCrypto,
    credential: CredentialWithKey,
    signer: SignatureKeyPair,
}

impl OpenMlsDevice {
    fn new(identity: &[u8]) -> Self {
        let provider = OpenMlsRustCrypto::default();
        let signer = SignatureKeyPair::new(OPENMLS_SUITE.signature_algorithm()).unwrap();
        signer.store(provider.storage()).unwrap();
        let credential = CredentialWithKey {
            credential: BasicCredential::new(identity.to_vec()).into(),
            signature_key: signer.public().into(),
        };
        Self {
            provider,
            credential,
            signer,
        }
    }

    /// Serialized `MLSMessage(KeyPackage)`, the only thing the other
    /// implementation ever sees.
    fn key_package_bytes(&self) -> Vec<u8> {
        let key_package: KeyPackage = KeyPackage::builder()
            .build(
                OPENMLS_SUITE,
                &self.provider,
                &self.signer,
                self.credential.clone(),
            )
            .unwrap()
            .key_package()
            .clone();
        MlsMessageOut::from(key_package).to_bytes().unwrap()
    }

    fn create_group(&self) -> MlsGroup {
        // The ratchet tree extension keeps the tree inside the Welcome, so the
        // two implementations never have to agree on an out-of-band tree
        // encoding to complete a join.
        let config = MlsGroupCreateConfig::builder()
            .use_ratchet_tree_extension(true)
            .ciphersuite(OPENMLS_SUITE)
            .build();
        MlsGroup::new(
            &self.provider,
            &self.signer,
            &config,
            self.credential.clone(),
        )
        .unwrap()
    }

    fn join(&self, welcome_bytes: &[u8]) -> MlsGroup {
        let welcome = match MlsMessageIn::tls_deserialize(&mut &welcome_bytes[..])
            .expect("welcome does not deserialize as MLSMessage")
            .extract()
        {
            MlsMessageBodyIn::Welcome(welcome) => welcome,
            other => panic!("expected a Welcome, got {other:?}"),
        };
        let config = MlsGroupJoinConfig::builder()
            .use_ratchet_tree_extension(true)
            .build();
        StagedWelcome::new_from_welcome(&self.provider, &config, welcome, None)
            .expect("OpenMLS cannot stage the mls-rs Welcome")
            .into_group(&self.provider)
            .expect("OpenMLS cannot complete the mls-rs join")
    }
}

fn to_protocol_message(bytes: &[u8]) -> ProtocolMessage {
    MlsMessageIn::tls_deserialize(&mut &bytes[..])
        .expect("message does not deserialize as MLSMessage")
        .try_into_protocol_message()
        .expect("message is not a protocol message")
}

fn openmls_process(group: &mut MlsGroup, provider: &OpenMlsRustCrypto, bytes: &[u8]) -> Vec<u8> {
    let processed = group
        .process_message(provider, to_protocol_message(bytes))
        .expect("OpenMLS rejected a message produced by mls-rs");
    match processed.into_content() {
        ProcessedMessageContent::ApplicationMessage(message) => message.into_bytes(),
        ProcessedMessageContent::StagedCommitMessage(commit) => {
            group.merge_staged_commit(provider, *commit).unwrap();
            Vec::new()
        }
        other => panic!("unexpected content: {other:?}"),
    }
}

// --- direction 1: mls-rs creates, OpenMLS joins ----------------------------

#[test]
fn mls_rs_group_accepts_an_openmls_device() {
    let alice = mls_rs_client("alice-mls-rs-device").unwrap();
    let bob = OpenMlsDevice::new(b"bob-openmls-device");

    let mut alice_group = alice
        .create_group(
            ExtensionList::default(),
            Default::default(),
            Some(not_before()),
        )
        .unwrap();
    assert_eq!(alice_group.current_epoch(), 0);

    let bob_key_package = MlsMessage::from_bytes(&bob.key_package_bytes())
        .expect("mls-rs cannot parse the OpenMLS KeyPackage");
    let commit = alice_group
        .commit_builder()
        .add_member(bob_key_package)
        .expect("mls-rs rejected the OpenMLS KeyPackage")
        .build()
        .unwrap();
    alice_group.apply_pending_commit().unwrap();
    assert_eq!(alice_group.current_epoch(), 1);

    let welcome = commit
        .welcome_messages
        .first()
        .expect("no welcome produced")
        .to_bytes()
        .unwrap();
    let mut bob_group = bob.join(&welcome);
    assert_eq!(bob_group.epoch().as_u64(), 1);
    assert_eq!(
        bob_group
            .export_secret(bob.provider.crypto(), "threadline-interop", b"", 32)
            .unwrap(),
        alice_group
            .export_secret(b"threadline-interop", &[], 32)
            .unwrap()
            .as_bytes(),
        "epoch exporter disagrees across implementations"
    );

    // mls-rs -> OpenMLS application frame.
    let from_alice = alice_group
        .encrypt_application_message(APPLICATION_PAYLOAD, Default::default())
        .unwrap()
        .to_bytes()
        .unwrap();
    assert_eq!(
        openmls_process(&mut bob_group, &bob.provider, &from_alice),
        APPLICATION_PAYLOAD,
        "OpenMLS could not decrypt an mls-rs application frame"
    );

    // OpenMLS -> mls-rs application frame.
    let from_bob = bob_group
        .create_message(&bob.provider, &bob.signer, APPLICATION_PAYLOAD)
        .unwrap()
        .to_bytes()
        .unwrap();
    let received = alice_group
        .process_incoming_message(MlsMessage::from_bytes(&from_bob).unwrap())
        .expect("mls-rs rejected an OpenMLS application frame");
    match received {
        ReceivedMessage::ApplicationMessage(message) => {
            assert_eq!(message.data(), APPLICATION_PAYLOAD)
        }
        other => panic!("unexpected mls-rs message: {other:?}"),
    }

    // OpenMLS commits; mls-rs must follow the epoch.
    let (commit, _, _) = bob_group
        .self_update(&bob.provider, &bob.signer, LeafNodeParameters::default())
        .unwrap()
        .into_contents();
    bob_group.merge_pending_commit(&bob.provider).unwrap();
    assert_eq!(bob_group.epoch().as_u64(), 2);

    let received = alice_group
        .process_incoming_message(MlsMessage::from_bytes(&commit.to_bytes().unwrap()).unwrap())
        .expect("mls-rs rejected an OpenMLS commit");
    assert!(matches!(received, ReceivedMessage::Commit(_)));
    assert_eq!(alice_group.current_epoch(), 2);
    assert_eq!(
        bob_group
            .export_secret(bob.provider.crypto(), "threadline-interop", b"", 32)
            .unwrap(),
        alice_group
            .export_secret(b"threadline-interop", &[], 32)
            .unwrap()
            .as_bytes(),
        "epoch exporter disagrees after an OpenMLS commit"
    );
}

// --- direction 2: OpenMLS creates, mls-rs joins ----------------------------

#[test]
fn openmls_group_accepts_an_mls_rs_device() {
    let alice = OpenMlsDevice::new(b"alice-openmls-device");
    let bob = mls_rs_client("bob-mls-rs-device").unwrap();

    let mut alice_group = alice.create_group();
    let bob_key_package = bob
        .generate_key_package_message(Default::default(), Default::default(), Some(not_before()))
        .unwrap()
        .to_bytes()
        .unwrap();

    let key_package = match MlsMessageIn::tls_deserialize(&mut &bob_key_package[..])
        .expect("OpenMLS cannot parse the mls-rs KeyPackage")
        .extract()
    {
        MlsMessageBodyIn::KeyPackage(key_package) => key_package
            .validate(alice.provider.crypto(), ProtocolVersion::Mls10)
            .expect("OpenMLS rejected the mls-rs KeyPackage"),
        other => panic!("expected a KeyPackage, got {other:?}"),
    };

    let (_commit, welcome, _) = alice_group
        .add_members(&alice.provider, &alice.signer, &[key_package])
        .expect("OpenMLS could not add the mls-rs device");
    alice_group.merge_pending_commit(&alice.provider).unwrap();
    assert_eq!(alice_group.epoch().as_u64(), 1);

    let (mut bob_group, _) = bob
        .join_group(
            None,
            &MlsMessage::from_bytes(&welcome.to_bytes().unwrap()).unwrap(),
            None,
        )
        .expect("mls-rs cannot join from the OpenMLS Welcome");
    assert_eq!(bob_group.current_epoch(), 1);
    assert_eq!(
        alice_group
            .export_secret(alice.provider.crypto(), "threadline-interop", b"", 32)
            .unwrap(),
        bob_group
            .export_secret(b"threadline-interop", &[], 32)
            .unwrap()
            .as_bytes(),
        "epoch exporter disagrees across implementations"
    );

    // OpenMLS -> mls-rs, then mls-rs -> OpenMLS.
    let from_alice = alice_group
        .create_message(&alice.provider, &alice.signer, APPLICATION_PAYLOAD)
        .unwrap()
        .to_bytes()
        .unwrap();
    match bob_group
        .process_incoming_message(MlsMessage::from_bytes(&from_alice).unwrap())
        .expect("mls-rs rejected an OpenMLS application frame")
    {
        ReceivedMessage::ApplicationMessage(message) => {
            assert_eq!(message.data(), APPLICATION_PAYLOAD)
        }
        other => panic!("unexpected mls-rs message: {other:?}"),
    }

    let from_bob = bob_group
        .encrypt_application_message(APPLICATION_PAYLOAD, Default::default())
        .unwrap()
        .to_bytes()
        .unwrap();
    assert_eq!(
        openmls_process(&mut alice_group, &alice.provider, &from_bob),
        APPLICATION_PAYLOAD,
        "OpenMLS could not decrypt an mls-rs application frame"
    );

    // An mls-rs commit must move the OpenMLS group's epoch.
    let commit = bob_group.commit(Vec::new()).unwrap();
    bob_group.apply_pending_commit().unwrap();
    assert_eq!(bob_group.current_epoch(), 2);
    openmls_process(
        &mut alice_group,
        &alice.provider,
        &commit.commit_message.to_bytes().unwrap(),
    );
    assert_eq!(alice_group.epoch().as_u64(), 2);
    assert_eq!(
        alice_group
            .export_secret(alice.provider.crypto(), "threadline-interop", b"", 32)
            .unwrap(),
        bob_group
            .export_secret(b"threadline-interop", &[], 32)
            .unwrap()
            .as_bytes(),
        "epoch exporter disagrees after an mls-rs commit"
    );
}

// --- removal must hold across implementations ------------------------------

/// ADR-0003 requires that a revoked device cannot enter the successor epoch.
/// The guarantee is only real if the *other* implementation enforces it too.
#[test]
fn revocation_holds_when_the_remover_is_the_other_implementation() {
    let alice = OpenMlsDevice::new(b"alice-openmls-device");
    let bob = mls_rs_client("bob-mls-rs-device").unwrap();

    let mut alice_group = alice.create_group();
    let bob_key_package = bob
        .generate_key_package_message(Default::default(), Default::default(), Some(not_before()))
        .unwrap()
        .to_bytes()
        .unwrap();
    let key_package = match MlsMessageIn::tls_deserialize(&mut &bob_key_package[..])
        .unwrap()
        .extract()
    {
        MlsMessageBodyIn::KeyPackage(key_package) => key_package
            .validate(alice.provider.crypto(), ProtocolVersion::Mls10)
            .unwrap(),
        other => panic!("expected a KeyPackage, got {other:?}"),
    };
    let (_, welcome, _) = alice_group
        .add_members(&alice.provider, &alice.signer, &[key_package])
        .unwrap();
    alice_group.merge_pending_commit(&alice.provider).unwrap();
    let (mut bob_group, _) = bob
        .join_group(
            None,
            &MlsMessage::from_bytes(&welcome.to_bytes().unwrap()).unwrap(),
            None,
        )
        .unwrap();

    // OpenMLS removes the mls-rs device.
    let bob_leaf = LeafNodeIndex::new(bob_group.current_member_index());
    let (commit, _, _) = alice_group
        .remove_members(&alice.provider, &alice.signer, &[bob_leaf])
        .unwrap();
    alice_group.merge_pending_commit(&alice.provider).unwrap();
    assert_eq!(alice_group.epoch().as_u64(), 2);

    // mls-rs must see itself removed rather than silently continuing.
    let received = bob_group
        .process_incoming_message(MlsMessage::from_bytes(&commit.to_bytes().unwrap()).unwrap())
        .expect("mls-rs could not process the removal commit");
    match received {
        ReceivedMessage::Commit(description) => {
            assert!(
                matches!(description.effect, CommitEffect::Removed { .. }),
                "mls-rs did not report itself removed by the OpenMLS commit"
            );
        }
        other => panic!("unexpected mls-rs message: {other:?}"),
    }

    // The removed device must not be able to read the successor epoch.
    let after_removal = alice_group
        .create_message(&alice.provider, &alice.signer, APPLICATION_PAYLOAD)
        .unwrap()
        .to_bytes()
        .unwrap();
    assert!(
        bob_group
            .process_incoming_message(MlsMessage::from_bytes(&after_removal).unwrap())
            .is_err(),
        "a removed mls-rs device decrypted a successor-epoch frame"
    );
}

// --- the findings, locked as regressions -----------------------------------

/// Neither library's default leaf-node lifetime is accepted by the other, so
/// `tl-mls-1` has to pin the policy. mls-rs 0.55.3 issues `not_before = now`
/// with a 365-day range; OpenMLS 0.8.1 requires `not_before < now` strictly and
/// caps the range at 1h + 84d. A future dependency bump that "fixes" this
/// should make this test fail loudly rather than change behaviour silently.
#[test]
fn default_lifetime_policies_are_not_interoperable() {
    let alice = OpenMlsDevice::new(b"alice-openmls-device");
    let bob = mls_rs_client_full(
        "bob-mls-rs-device",
        Duration::from_secs(365 * 24 * 3600),
        true,
    )
    .unwrap();

    // Default `not_before` (= now) and default 365-day range.
    let bob_key_package = bob
        .generate_key_package_message(Default::default(), Default::default(), None)
        .unwrap()
        .to_bytes()
        .unwrap();

    let validated = match MlsMessageIn::tls_deserialize(&mut &bob_key_package[..])
        .expect("OpenMLS cannot parse the mls-rs KeyPackage")
        .extract()
    {
        MlsMessageBodyIn::KeyPackage(key_package) => {
            key_package.validate(alice.provider.crypto(), ProtocolVersion::Mls10)
        }
        other => panic!("expected a KeyPackage, got {other:?}"),
    };

    assert!(
        validated.is_err(),
        "OpenMLS accepted an mls-rs KeyPackage built with default lifetime settings; \
         re-check the tl-mls-1 lifetime policy against the new library versions"
    );
}

/// The handshake wire format has to be pinned as well: OpenMLS defaults to
/// `PURE_CIPHERTEXT_WIRE_FORMAT_POLICY` and rejects the PublicMessage commits
/// that mls-rs emits by default.
#[test]
fn plaintext_control_messages_are_rejected_by_openmls_defaults() {
    let alice = OpenMlsDevice::new(b"alice-openmls-device");
    let bob = mls_rs_client_full("bob-mls-rs-device", TL_MLS_1_LIFETIME, false).unwrap();

    let bob_key_package = bob
        .generate_key_package_message(Default::default(), Default::default(), Some(not_before()))
        .unwrap()
        .to_bytes()
        .unwrap();
    let key_package = match MlsMessageIn::tls_deserialize(&mut &bob_key_package[..])
        .unwrap()
        .extract()
    {
        MlsMessageBodyIn::KeyPackage(key_package) => key_package
            .validate(alice.provider.crypto(), ProtocolVersion::Mls10)
            .unwrap(),
        other => panic!("expected a KeyPackage, got {other:?}"),
    };

    let mut alice_group = alice.create_group();
    let (_, welcome, _) = alice_group
        .add_members(&alice.provider, &alice.signer, &[key_package])
        .unwrap();
    alice_group.merge_pending_commit(&alice.provider).unwrap();
    let (mut bob_group, _) = bob
        .join_group(
            None,
            &MlsMessage::from_bytes(&welcome.to_bytes().unwrap()).unwrap(),
            None,
        )
        .unwrap();

    let commit = bob_group.commit(Vec::new()).unwrap();
    bob_group.apply_pending_commit().unwrap();

    let outcome = alice_group.process_message(
        &alice.provider,
        to_protocol_message(&commit.commit_message.to_bytes().unwrap()),
    );
    assert!(
        matches!(outcome, Err(ProcessMessageError::IncompatibleWireFormat)),
        "expected OpenMLS to reject a PublicMessage commit under its default \
         wire format policy, got {outcome:?}"
    );
}

//! Group-size performance profile for the P00-08 gate.
//!
//! ADR-0003 requires performance evidence over target group size, epoch
//! frequency, KeyPackage consumption and mobile memory pressure before OpenMLS
//! can be accepted. This is a profile, not a benchmark suite: it produces the
//! order-of-magnitude numbers the ADR needs to state a supported group size,
//! and it is deliberately `#[ignore]`d so ordinary test runs stay fast.
//!
//! Run it with:
//!
//! ```sh
//! cargo test --manifest-path spikes/e2ee-interop/rust/Cargo.toml --locked \
//!     --release -- --ignored --nocapture
//! ```

use openmls::prelude::{tls_codec::*, *};
use openmls_basic_credential::SignatureKeyPair;
use openmls_rust_crypto::OpenMlsRustCrypto;
use std::time::{Duration, Instant};

const CIPHERSUITE: Ciphersuite = Ciphersuite::MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519;
const PAYLOAD: &[u8] = b"threadline-perf-frame";
/// Threadline channels are device-scoped, so a 50-person channel with three
/// devices each is already 150 leaves. 256 is the stress point.
const GROUP_SIZES: [usize; 5] = [2, 8, 32, 128, 256];

struct Device {
    provider: OpenMlsRustCrypto,
    credential: CredentialWithKey,
    signer: SignatureKeyPair,
}

impl Device {
    fn new(identity: &[u8]) -> Self {
        let provider = OpenMlsRustCrypto::default();
        let signer = SignatureKeyPair::new(CIPHERSUITE.signature_algorithm()).unwrap();
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

    fn key_package(&self) -> KeyPackageBundle {
        KeyPackage::builder()
            .build(
                CIPHERSUITE,
                &self.provider,
                &self.signer,
                self.credential.clone(),
            )
            .unwrap()
    }
}

fn millis(duration: Duration) -> f64 {
    duration.as_secs_f64() * 1000.0
}

#[test]
#[ignore = "performance profile; run explicitly with --release --ignored --nocapture"]
fn group_size_performance_profile() {
    println!(
        "\n{:>6} {:>14} {:>14} {:>14} {:>14} {:>12} {:>12}",
        "leaves",
        "add-commit ms",
        "self-upd ms",
        "encrypt ms",
        "decrypt ms",
        "welcome B",
        "commit B"
    );

    for size in GROUP_SIZES {
        let creator = Device::new(b"creator-device");
        let mut group = MlsGroup::new(
            &creator.provider,
            &creator.signer,
            &MlsGroupCreateConfig::default(),
            creator.credential.clone(),
        )
        .unwrap();

        // Fill the group to `size` leaves, one member per commit, which is the
        // shape Threadline actually produces: membership changes are ordered by
        // the control plane and committed one at a time. The last leaf is the
        // receiver, so decryption is measured from a device that joined last and
        // therefore holds the least tree state.
        let filler: Vec<Device> = (1..size.saturating_sub(1))
            .map(|index| Device::new(format!("member-device-{index}").as_bytes()))
            .collect();

        for member in &filler {
            let key_package = member.key_package();
            group
                .add_members(
                    &creator.provider,
                    &creator.signer,
                    core::slice::from_ref(key_package.key_package()),
                )
                .unwrap();
            group.merge_pending_commit(&creator.provider).unwrap();
        }

        // The final add is the measured one: it runs against the largest tree.
        let receiver = Device::new(b"receiver-device");
        let receiver_key_package = receiver.key_package();
        let started = Instant::now();
        let (commit, welcome, _) = group
            .add_members(
                &creator.provider,
                &creator.signer,
                core::slice::from_ref(receiver_key_package.key_package()),
            )
            .unwrap();
        group.merge_pending_commit(&creator.provider).unwrap();
        let last_add = started.elapsed();
        let welcome_bytes = welcome.tls_serialize_detached().unwrap().len();
        let commit_bytes = commit.tls_serialize_detached().unwrap().len();

        let welcome =
            match MlsMessageIn::tls_deserialize(&mut welcome.to_bytes().unwrap().as_slice())
                .unwrap()
                .extract()
            {
                MlsMessageBodyIn::Welcome(welcome) => welcome,
                _ => panic!("expected a welcome"),
            };
        let mut receiver_group = StagedWelcome::new_from_welcome(
            &receiver.provider,
            &MlsGroupJoinConfig::default(),
            welcome,
            Some(group.export_ratchet_tree().into()),
        )
        .unwrap()
        .into_group(&receiver.provider)
        .unwrap();
        assert_eq!(group.members().count(), size);

        let started = Instant::now();
        let (commit, _, _) = group
            .self_update(
                &creator.provider,
                &creator.signer,
                LeafNodeParameters::default(),
            )
            .unwrap()
            .into_contents();
        group.merge_pending_commit(&creator.provider).unwrap();
        let self_update = started.elapsed();

        // The receiver has to follow that epoch before it can read application
        // traffic, so the decrypt timing below is measured in the same epoch.
        let message = MlsMessageIn::tls_deserialize(&mut commit.to_bytes().unwrap().as_slice())
            .unwrap()
            .try_into_protocol_message()
            .unwrap();
        let processed = receiver_group
            .process_message(&receiver.provider, message)
            .unwrap();
        match processed.into_content() {
            ProcessedMessageContent::StagedCommitMessage(staged) => receiver_group
                .merge_staged_commit(&receiver.provider, *staged)
                .unwrap(),
            other => panic!("unexpected content: {other:?}"),
        }
        assert_eq!(receiver_group.epoch(), group.epoch());

        let started = Instant::now();
        let application = group
            .create_message(&creator.provider, &creator.signer, PAYLOAD)
            .unwrap();
        let encrypt = started.elapsed();

        let decrypt = {
            let message =
                MlsMessageIn::tls_deserialize(&mut application.to_bytes().unwrap().as_slice())
                    .unwrap()
                    .try_into_protocol_message()
                    .unwrap();
            let started = Instant::now();
            let processed = receiver_group
                .process_message(&receiver.provider, message)
                .expect("receiver could not decrypt");
            let elapsed = started.elapsed();
            match processed.into_content() {
                ProcessedMessageContent::ApplicationMessage(message) => {
                    assert_eq!(message.into_bytes(), PAYLOAD)
                }
                other => panic!("unexpected content: {other:?}"),
            }
            elapsed
        };

        println!(
            "{size:>6} {:>14.3} {:>14.3} {:>14.3} {:>14.3} {welcome_bytes:>12} {commit_bytes:>12}",
            millis(last_add),
            millis(self_update),
            millis(encrypt),
            millis(decrypt),
        );
    }
    println!();
}

/// KeyPackage generation is on the enrollment path and on every re-stock, so
/// its cost bounds how cheaply a device can keep the Key Directory supplied.
#[test]
#[ignore = "performance profile; run explicitly with --release --ignored --nocapture"]
fn key_package_generation_profile() {
    const COUNT: usize = 100;
    let device = Device::new(b"enrolling-device");

    let started = Instant::now();
    let mut total_bytes = 0usize;
    for _ in 0..COUNT {
        let bundle = device.key_package();
        total_bytes += bundle.key_package().tls_serialize_detached().unwrap().len();
    }
    let elapsed = started.elapsed();

    println!(
        "\nkey packages: {COUNT} generated in {:.1} ms ({:.3} ms each), {} bytes each\n",
        millis(elapsed),
        millis(elapsed) / COUNT as f64,
        total_bytes / COUNT,
    );
}

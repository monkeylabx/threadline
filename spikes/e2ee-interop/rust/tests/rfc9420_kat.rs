//! RFC 9420 Known Answer Tests for the `tl-mls-1` cipher suite.
//!
//! The ADR-0003 P00-08 gate requires RFC 9420 Known Answer Tests, not only
//! library-internal round trips. Every construction below is re-implemented
//! from RFC 9420 on top of the `OpenMlsCrypto` primitives, and the TLS
//! presentation-language encoding is written here rather than taken from the
//! library. A vector therefore fails if the candidate crypto provider disagrees
//! with the RFC, instead of agreeing with itself.
//!
//! Vectors are the upstream `mlswg/mls-implementations` vectors filtered to
//! cipher suite 1; see `../../vectors/rfc9420/README.md` for provenance.

use openmls_rust_crypto::OpenMlsRustCrypto;
use openmls_traits::{
    crypto::OpenMlsCrypto,
    types::{
        AeadType, HashType, HpkeAeadType, HpkeCiphertext, HpkeConfig, HpkeKdfType, HpkeKemType,
        SignatureScheme,
    },
    OpenMlsProvider,
};
use serde_json::Value;
use std::path::{Path, PathBuf};

/// `tl-mls-1` fixes MLS cipher suite 1.
const CIPHER_SUITE: u16 = 1;
const HASH: HashType = HashType::Sha2_256;
/// KDF.Nh for SHA-256.
const NH: usize = 32;
const SIGNATURE_SCHEME: SignatureScheme = SignatureScheme::ED25519;
const HPKE: HpkeConfig = HpkeConfig(
    HpkeKemType::DhKem25519,
    HpkeKdfType::HkdfSha256,
    HpkeAeadType::AesGcm128,
);
/// MLS protocol version 1 (`mls10`), the only version `tl-mls-1` accepts.
const PROTOCOL_VERSION: u16 = 1;

fn vectors_dir() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR")).join("../vectors/rfc9420")
}

fn load(name: &str) -> Vec<Value> {
    let path = vectors_dir().join(name);
    let raw = std::fs::read_to_string(&path)
        .unwrap_or_else(|error| panic!("missing vector {}: {error}", path.display()));
    let parsed: Vec<Value> = serde_json::from_str(&raw).expect("vector is not a JSON array");
    assert!(!parsed.is_empty(), "vector {name} is empty");
    for entry in &parsed {
        assert_eq!(
            entry["cipher_suite"].as_u64(),
            Some(u64::from(CIPHER_SUITE)),
            "vector {name} contains a cipher suite outside tl-mls-1"
        );
    }
    parsed
}

fn bytes(value: &Value, key: &str) -> Vec<u8> {
    let hex_value = value[key]
        .as_str()
        .unwrap_or_else(|| panic!("field {key} is not a hex string"));
    hex::decode(hex_value).unwrap_or_else(|error| panic!("field {key} is not hex: {error}"))
}

fn number(value: &Value, key: &str) -> u64 {
    value[key]
        .as_u64()
        .unwrap_or_else(|| panic!("field {key} is not a number"))
}

// --- RFC 9420 presentation language ---------------------------------------

/// RFC 9420 section 2.1.2 variable-length vector header.
fn push_varint(out: &mut Vec<u8>, length: usize) {
    match length {
        0..=0x3f => out.push(length as u8),
        0x40..=0x3fff => {
            let encoded = (length as u16) | 0x4000;
            out.extend_from_slice(&encoded.to_be_bytes());
        }
        0x4000..=0x3fff_ffff => {
            let encoded = (length as u32) | 0x8000_0000;
            out.extend_from_slice(&encoded.to_be_bytes());
        }
        _ => panic!("value too long for an RFC 9420 variable-length vector"),
    }
}

/// `opaque field<V>`
fn push_opaque(out: &mut Vec<u8>, data: &[u8]) {
    push_varint(out, data.len());
    out.extend_from_slice(data);
}

// --- RFC 9420 section 8 labeled constructions ------------------------------

struct Kat<'a> {
    crypto: &'a dyn OpenMlsCrypto,
}

impl Kat<'_> {
    fn hash(&self, data: &[u8]) -> Vec<u8> {
        self.crypto.hash(HASH, data).expect("hash")
    }

    fn extract(&self, salt: &[u8], ikm: &[u8]) -> Vec<u8> {
        self.crypto
            .hkdf_extract(HASH, salt, ikm)
            .expect("hkdf extract")
            .as_slice()
            .to_vec()
    }

    /// `RefHash(label, value) = Hash(RefHashInput)`
    fn ref_hash(&self, label: &[u8], value: &[u8]) -> Vec<u8> {
        let mut input = Vec::new();
        push_opaque(&mut input, label);
        push_opaque(&mut input, value);
        self.hash(&input)
    }

    /// `ExpandWithLabel(Secret, Label, Context, Length)`
    fn expand_with_label(
        &self,
        secret: &[u8],
        label: &[u8],
        context: &[u8],
        length: usize,
    ) -> Vec<u8> {
        let mut kdf_label = Vec::new();
        kdf_label.extend_from_slice(&(length as u16).to_be_bytes());
        let mut full_label = b"MLS 1.0 ".to_vec();
        full_label.extend_from_slice(label);
        push_opaque(&mut kdf_label, &full_label);
        push_opaque(&mut kdf_label, context);
        self.crypto
            .hkdf_expand(HASH, secret, &kdf_label, length)
            .expect("hkdf expand")
            .as_slice()
            .to_vec()
    }

    /// `DeriveSecret(Secret, Label)`
    fn derive_secret(&self, secret: &[u8], label: &[u8]) -> Vec<u8> {
        self.expand_with_label(secret, label, b"", NH)
    }

    /// `DeriveTreeSecret(Secret, Label, Generation, Length)`
    fn derive_tree_secret(
        &self,
        secret: &[u8],
        label: &[u8],
        generation: u32,
        length: usize,
    ) -> Vec<u8> {
        self.expand_with_label(secret, label, &generation.to_be_bytes(), length)
    }

    /// `SignContent` / `EncryptContext` share the labeled-struct shape.
    fn labeled_struct(&self, label: &[u8], payload: &[u8]) -> Vec<u8> {
        let mut out = Vec::new();
        let mut full_label = b"MLS 1.0 ".to_vec();
        full_label.extend_from_slice(label);
        push_opaque(&mut out, &full_label);
        push_opaque(&mut out, payload);
        out
    }
}

// --- crypto-basics ---------------------------------------------------------

#[test]
fn rfc9420_crypto_basics_kat() {
    let provider = OpenMlsRustCrypto::default();
    let kat = Kat {
        crypto: provider.crypto(),
    };

    for entry in load("crypto-basics.cs1.json") {
        let ref_hash = &entry["ref_hash"];
        let label = ref_hash["label"]
            .as_str()
            .expect("ref_hash label")
            .as_bytes();
        assert_eq!(
            kat.ref_hash(label, &bytes(ref_hash, "value")),
            bytes(ref_hash, "out"),
            "RefHash mismatch"
        );

        let expand = &entry["expand_with_label"];
        assert_eq!(
            kat.expand_with_label(
                &bytes(expand, "secret"),
                expand["label"].as_str().expect("label").as_bytes(),
                &bytes(expand, "context"),
                number(expand, "length") as usize,
            ),
            bytes(expand, "out"),
            "ExpandWithLabel mismatch"
        );

        let derive = &entry["derive_secret"];
        assert_eq!(
            kat.derive_secret(
                &bytes(derive, "secret"),
                derive["label"].as_str().expect("label").as_bytes(),
            ),
            bytes(derive, "out"),
            "DeriveSecret mismatch"
        );

        let tree = &entry["derive_tree_secret"];
        assert_eq!(
            kat.derive_tree_secret(
                &bytes(tree, "secret"),
                tree["label"].as_str().expect("label").as_bytes(),
                number(tree, "generation") as u32,
                number(tree, "length") as usize,
            ),
            bytes(tree, "out"),
            "DeriveTreeSecret mismatch"
        );

        // SignWithLabel: the provider must verify the RFC signature, and because
        // Ed25519 is deterministic it must also reproduce it byte for byte.
        let sign = &entry["sign_with_label"];
        let sign_content = kat.labeled_struct(
            sign["label"].as_str().expect("label").as_bytes(),
            &bytes(sign, "content"),
        );
        let public = bytes(sign, "pub");
        provider
            .crypto()
            .verify_signature(
                SIGNATURE_SCHEME,
                &sign_content,
                &public,
                &bytes(sign, "signature"),
            )
            .expect("SignWithLabel signature does not verify");

        let produced = provider
            .crypto()
            .sign(SIGNATURE_SCHEME, &sign_content, &bytes(sign, "priv"))
            .expect("SignWithLabel signing failed");
        assert_eq!(
            produced,
            bytes(sign, "signature"),
            "SignWithLabel is not deterministic against the RFC vector"
        );

        // EncryptWithLabel: decrypt the RFC ciphertext with the RFC private key.
        let encrypt = &entry["encrypt_with_label"];
        let info = kat.labeled_struct(
            encrypt["label"].as_str().expect("label").as_bytes(),
            &bytes(encrypt, "context"),
        );
        let ciphertext = HpkeCiphertext {
            kem_output: bytes(encrypt, "kem_output").into(),
            ciphertext: bytes(encrypt, "ciphertext").into(),
        };
        let plaintext = provider
            .crypto()
            .hpke_open(HPKE, &ciphertext, &bytes(encrypt, "priv"), &info, b"")
            .expect("EncryptWithLabel decryption failed");
        assert_eq!(
            plaintext,
            bytes(encrypt, "plaintext"),
            "EncryptWithLabel plaintext mismatch"
        );

        // Round-trip the same labeled context through the sealing path so the
        // provider's sender side is covered as well.
        let resealed = provider
            .crypto()
            .hpke_seal(HPKE, &bytes(encrypt, "pub"), &info, b"", &plaintext)
            .expect("hpke seal");
        let reopened = provider
            .crypto()
            .hpke_open(HPKE, &resealed, &bytes(encrypt, "priv"), &info, b"")
            .expect("hpke open of resealed ciphertext");
        assert_eq!(reopened, plaintext, "EncryptWithLabel round trip mismatch");

        // A different label must not decrypt: the domain separation the ADR
        // relies on has to be real, not incidental.
        let wrong_info = kat.labeled_struct(b"ThreadlineWrongLabel", &bytes(encrypt, "context"));
        assert!(
            provider
                .crypto()
                .hpke_open(HPKE, &ciphertext, &bytes(encrypt, "priv"), &wrong_info, b"")
                .is_err(),
            "EncryptWithLabel decrypted under a different label"
        );
    }
}

// --- key schedule ----------------------------------------------------------

/// RFC 9420 section 8.1 `GroupContext`, serialized here rather than by the
/// library under test.
fn group_context(
    group_id: &[u8],
    epoch: u64,
    tree_hash: &[u8],
    confirmed_transcript_hash: &[u8],
) -> Vec<u8> {
    let mut out = Vec::new();
    out.extend_from_slice(&PROTOCOL_VERSION.to_be_bytes());
    out.extend_from_slice(&CIPHER_SUITE.to_be_bytes());
    push_opaque(&mut out, group_id);
    out.extend_from_slice(&epoch.to_be_bytes());
    push_opaque(&mut out, tree_hash);
    push_opaque(&mut out, confirmed_transcript_hash);
    // Empty `Extension extensions<V>`.
    push_varint(&mut out, 0);
    out
}

#[test]
fn rfc9420_key_schedule_kat() {
    let provider = OpenMlsRustCrypto::default();
    let kat = Kat {
        crypto: provider.crypto(),
    };

    for entry in load("key-schedule.cs1.json") {
        let group_id = bytes(&entry, "group_id");
        let mut init_secret = bytes(&entry, "initial_init_secret");

        for (index, epoch) in entry["epochs"]
            .as_array()
            .expect("epochs array")
            .iter()
            .enumerate()
        {
            let epoch_number = index as u64;
            let context = group_context(
                &group_id,
                epoch_number,
                &bytes(epoch, "tree_hash"),
                &bytes(epoch, "confirmed_transcript_hash"),
            );
            assert_eq!(
                context,
                bytes(epoch, "group_context"),
                "GroupContext encoding mismatch at epoch {epoch_number}"
            );

            let joiner_secret = kat.expand_with_label(
                &kat.extract(&init_secret, &bytes(epoch, "commit_secret")),
                b"joiner",
                &context,
                NH,
            );
            assert_eq!(
                joiner_secret,
                bytes(epoch, "joiner_secret"),
                "joiner_secret mismatch at epoch {epoch_number}"
            );

            let member_secret = kat.extract(&joiner_secret, &bytes(epoch, "psk_secret"));
            assert_eq!(
                kat.expand_with_label(&member_secret, b"welcome", b"", NH),
                bytes(epoch, "welcome_secret"),
                "welcome_secret mismatch at epoch {epoch_number}"
            );

            let epoch_secret = kat.expand_with_label(&member_secret, b"epoch", &context, NH);
            for (label, field) in [
                (&b"sender data"[..], "sender_data_secret"),
                (b"encryption", "encryption_secret"),
                (b"exporter", "exporter_secret"),
                (b"authentication", "epoch_authenticator"),
                (b"external", "external_secret"),
                (b"confirm", "confirmation_key"),
                (b"membership", "membership_key"),
                (b"resumption", "resumption_psk"),
                (b"init", "init_secret"),
            ] {
                assert_eq!(
                    kat.derive_secret(&epoch_secret, label),
                    bytes(epoch, field),
                    "{field} mismatch at epoch {epoch_number}"
                );
            }

            // external_pub = KEM.DeriveKeyPair(external_secret).pub
            let external_key_pair = provider
                .crypto()
                .derive_hpke_keypair(HPKE, &kat.derive_secret(&epoch_secret, b"external"))
                .expect("derive external key pair");
            assert_eq!(
                external_key_pair.public,
                bytes(epoch, "external_pub"),
                "external_pub mismatch at epoch {epoch_number}"
            );

            // MLS-Exporter(Label, Context, Length)
            let exporter = &epoch["exporter"];
            let exporter_secret = kat.derive_secret(&epoch_secret, b"exporter");
            let exported = kat.expand_with_label(
                // `exporter.label` is a plain string in the vector format, unlike
                // `exporter.context`, which is hex-encoded binary.
                &kat.derive_secret(
                    &exporter_secret,
                    exporter["label"]
                        .as_str()
                        .expect("exporter label")
                        .as_bytes(),
                ),
                b"exported",
                &kat.hash(&bytes(exporter, "context")),
                number(exporter, "length") as usize,
            );
            assert_eq!(
                exported,
                bytes(exporter, "secret"),
                "MLS-Exporter mismatch at epoch {epoch_number}"
            );

            init_secret = kat.derive_secret(&epoch_secret, b"init");
        }
    }
}

// --- PSK secret ------------------------------------------------------------

#[test]
fn rfc9420_psk_secret_kat() {
    let provider = OpenMlsRustCrypto::default();
    let kat = Kat {
        crypto: provider.crypto(),
    };
    let zero = vec![0u8; NH];
    let mut covered_counts = Vec::new();

    for entry in load("psk_secret.cs1.json") {
        let psks = entry["psks"].as_array().expect("psks array");
        let count = psks.len();
        covered_counts.push(count);

        let mut psk_secret = zero.clone();
        for (index, psk) in psks.iter().enumerate() {
            // PreSharedKeyID with external PSKType (1).
            let mut psk_id_struct = vec![1u8];
            push_opaque(&mut psk_id_struct, &bytes(psk, "psk_id"));
            push_opaque(&mut psk_id_struct, &bytes(psk, "psk_nonce"));

            // PSKLabel { PreSharedKeyID id; uint16 index; uint16 count; }
            let mut psk_label = psk_id_struct;
            psk_label.extend_from_slice(&(index as u16).to_be_bytes());
            psk_label.extend_from_slice(&(count as u16).to_be_bytes());

            let extracted = kat.extract(&zero, &bytes(psk, "psk"));
            let psk_input = kat.expand_with_label(&extracted, b"derived psk", &psk_label, NH);
            psk_secret = kat.extract(&psk_input, &psk_secret);
        }

        assert_eq!(
            psk_secret,
            bytes(&entry, "psk_secret"),
            "psk_secret mismatch for {count} PSKs"
        );
    }

    // The zero-PSK case is the one Threadline hits on every non-PSK commit;
    // make sure the vector set actually exercised it.
    assert!(
        covered_counts.contains(&0),
        "psk vectors did not cover the no-PSK case"
    );
    assert!(
        covered_counts.iter().any(|count| *count > 1),
        "psk vectors did not cover the chained multi-PSK case"
    );
}

// --- negative control ------------------------------------------------------

/// The KAT harness is only evidence if it can fail. A one-bit change in the
/// label domain separation must break the derivation.
#[test]
fn kat_harness_detects_a_wrong_label() {
    let provider = OpenMlsRustCrypto::default();
    let kat = Kat {
        crypto: provider.crypto(),
    };
    let entry = load("crypto-basics.cs1.json").remove(0);
    let derive = &entry["derive_secret"];
    let secret = bytes(derive, "secret");
    let expected = bytes(derive, "out");

    assert_eq!(
        kat.derive_secret(&secret, derive["label"].as_str().unwrap().as_bytes()),
        expected
    );
    assert_ne!(
        kat.derive_secret(&secret, b"not-the-rfc-label"),
        expected,
        "harness cannot distinguish labels"
    );

    // AEAD tampering must be detected too: this is the property the corrupted
    // frame test in `openmls_runtime.rs` depends on.
    let key = vec![0x11u8; AeadType::Aes128Gcm.key_size()];
    let nonce = vec![0x22u8; AeadType::Aes128Gcm.nonce_size()];
    let mut sealed = provider
        .crypto()
        .aead_encrypt(AeadType::Aes128Gcm, &key, b"threadline", &nonce, b"aad")
        .expect("aead encrypt");
    *sealed.last_mut().expect("non-empty ciphertext") ^= 0x01;
    assert!(
        provider
            .crypto()
            .aead_decrypt(AeadType::Aes128Gcm, &key, &sealed, &nonce, b"aad")
            .is_err(),
        "tampered AEAD ciphertext was accepted"
    );
}

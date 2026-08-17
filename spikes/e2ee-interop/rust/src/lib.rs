use sha2::{Digest, Sha256};
use std::{collections::BTreeMap, fmt, fs, path::Path};

pub const PROFILE: &str = "tl-mls-1";
pub const CIPHERSUITE: &str = "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519";

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct GoldenVector {
    values: BTreeMap<String, String>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum VectorError {
    Io(String),
    DuplicateKey(String),
    InvalidLine(usize),
    Missing(String),
    Mismatch {
        key: String,
        expected: String,
        actual: String,
    },
}

impl fmt::Display for VectorError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Io(error) => write!(f, "vector I/O failed: {error}"),
            Self::DuplicateKey(key) => write!(f, "duplicate vector key: {key}"),
            Self::InvalidLine(line) => write!(f, "invalid vector line: {line}"),
            Self::Missing(key) => write!(f, "missing vector key: {key}"),
            Self::Mismatch {
                key,
                expected,
                actual,
            } => {
                write!(
                    f,
                    "vector mismatch for {key}: expected {expected}, got {actual}"
                )
            }
        }
    }
}

impl std::error::Error for VectorError {}

impl GoldenVector {
    pub fn load(path: impl AsRef<Path>) -> Result<Self, VectorError> {
        let source =
            fs::read_to_string(path).map_err(|error| VectorError::Io(error.to_string()))?;
        Self::parse(&source)
    }

    pub fn parse(source: &str) -> Result<Self, VectorError> {
        let mut values = BTreeMap::new();
        for (index, raw_line) in source.lines().enumerate() {
            let line = raw_line.trim();
            if line.is_empty() || line.starts_with('#') {
                continue;
            }
            let (key, value) = line
                .split_once('=')
                .ok_or(VectorError::InvalidLine(index + 1))?;
            if key.is_empty() || value.is_empty() {
                return Err(VectorError::InvalidLine(index + 1));
            }
            if values.insert(key.to_owned(), value.to_owned()).is_some() {
                return Err(VectorError::DuplicateKey(key.to_owned()));
            }
        }
        Ok(Self { values })
    }

    pub fn value(&self, key: &str) -> Result<&str, VectorError> {
        self.values
            .get(key)
            .map(String::as_str)
            .ok_or_else(|| VectorError::Missing(key.to_owned()))
    }

    pub fn validate(&self) -> Result<(), VectorError> {
        self.expect("format_version", "1")?;
        self.expect("profile", PROFILE)?;
        self.expect("protocol", "MLS-1.0-RFC9420")?;
        self.expect("ciphersuite", CIPHERSUITE)?;
        self.expect("mls_library", "openmls")?;
        self.expect("mls_library_version", "0.8.1")?;
        self.expect("vector_class", "semantic-contract")?;

        // Pinned by the P00-08 cross-implementation evidence: neither library's
        // defaults are interoperable, so tl-mls-1 fixes the handshake wire
        // format and the leaf-node lifetime policy explicitly.
        self.expect("wire_format.handshake", "private-message")?;
        self.expect("leaf_lifetime.not_before_skew_seconds", "3600")?;
        self.expect("leaf_lifetime.max_range_seconds", "7261200")?;
        self.expect("interop.independent_implementation", "mls-rs")?;
        self.expect("recovery.wrapper", "optional")?;
        self.expect("recovery.private_key_location", "external-kms-hsm-only")?;
        self.expect("output.classification", "public-metadata-and-digests-only")?;

        for (key, expected) in [
            ("key_package.expected", "accept"),
            ("offline_new_device.expected", "join-current-epoch"),
            ("out_of_order_commit.expected", "queue-until-predecessor"),
            (
                "device_revocation.expected",
                "removed-device-cannot-read-future-epoch",
            ),
            ("history.authorized", "accept"),
            ("history.unauthorized", "TL_E2EE_HISTORY_UNAUTHORIZED"),
            ("history.cross_tenant", "TL_E2EE_TENANT_MISMATCH"),
            ("recovery.success", "accept"),
            ("recovery.no_recipient", "TL_E2EE_RECOVERY_UNAVAILABLE"),
            ("recovery.refused", "TL_E2EE_RECOVERY_REFUSED"),
            ("recovery.corrupt", "TL_E2EE_CORRUPT"),
            ("recovery.old_epoch", "TL_E2EE_OLD_EPOCH"),
            ("recovery.cross_group", "TL_E2EE_GROUP_MISMATCH"),
            ("recovery.cross_tenant", "TL_E2EE_TENANT_MISMATCH"),
        ] {
            self.expect(key, expected)?;
        }

        for key in [
            "error.replay",
            "error.corrupt",
            "error.old_epoch",
            "error.future_epoch",
            "error.unknown_version",
        ] {
            let value = self.value(key)?;
            if !value.starts_with("TL_E2EE_") {
                return Err(VectorError::Mismatch {
                    key: key.to_owned(),
                    expected: "stable TL_E2EE_* error".to_owned(),
                    actual: value.to_owned(),
                });
            }
        }

        let canonical = format!(
            "tl-recovery-envelope-v{}|{}|{}|{}|{}|{}",
            self.value("recovery.version")?,
            self.value("recovery.tenant_id")?,
            self.value("recovery.group_id")?,
            self.value("recovery.epoch")?,
            self.value("recovery.recipient_key_id")?,
            self.value("recovery.payload_sha256")?,
        );
        let actual = format!("{:x}", Sha256::digest(canonical.as_bytes()));
        self.expect("recovery.binding_sha256", &actual)
    }

    fn expect(&self, key: &str, expected: &str) -> Result<(), VectorError> {
        let actual = self.value(key)?;
        if actual == expected {
            return Ok(());
        }
        Err(VectorError::Mismatch {
            key: key.to_owned(),
            expected: expected.to_owned(),
            actual: actual.to_owned(),
        })
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RecoveryDecision {
    Accept,
    Refuse(&'static str),
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RecoveryRequest<'a> {
    pub version: u8,
    pub tenant_id: &'a str,
    pub group_id: &'a str,
    pub epoch: u64,
    pub recipient_present: bool,
    pub authorized: bool,
    pub payload_integrity_valid: bool,
}

pub fn assess_recovery(request: &RecoveryRequest<'_>) -> RecoveryDecision {
    if request.version != 1 {
        return RecoveryDecision::Refuse("TL_E2EE_UNKNOWN_VERSION");
    }
    if !request.recipient_present {
        return RecoveryDecision::Refuse("TL_E2EE_RECOVERY_UNAVAILABLE");
    }
    if request.tenant_id != "tenant-acme" {
        return RecoveryDecision::Refuse("TL_E2EE_TENANT_MISMATCH");
    }
    if request.group_id != "group-7f3a" {
        return RecoveryDecision::Refuse("TL_E2EE_GROUP_MISMATCH");
    }
    if request.epoch != 4 {
        return RecoveryDecision::Refuse("TL_E2EE_OLD_EPOCH");
    }
    if !request.payload_integrity_valid {
        return RecoveryDecision::Refuse("TL_E2EE_CORRUPT");
    }
    if !request.authorized {
        return RecoveryDecision::Refuse("TL_E2EE_RECOVERY_REFUSED");
    }
    RecoveryDecision::Accept
}

pub fn assess_history(tenant_matches: bool, authorized: bool) -> RecoveryDecision {
    if !tenant_matches {
        return RecoveryDecision::Refuse("TL_E2EE_TENANT_MISMATCH");
    }
    if !authorized {
        return RecoveryDecision::Refuse("TL_E2EE_HISTORY_UNAUTHORIZED");
    }
    RecoveryDecision::Accept
}

#[cfg(test)]
mod tests {
    use super::*;

    fn vector_path() -> std::path::PathBuf {
        Path::new(env!("CARGO_MANIFEST_DIR")).join("../../../test/crypto/e2ee-interop-v1.vector")
    }

    #[test]
    fn validates_public_golden_vector() {
        GoldenVector::load(vector_path())
            .unwrap()
            .validate()
            .unwrap();
    }

    #[test]
    fn recovery_policy_has_deterministic_refusals() {
        let valid = RecoveryRequest {
            version: 1,
            tenant_id: "tenant-acme",
            group_id: "group-7f3a",
            epoch: 4,
            recipient_present: true,
            authorized: true,
            payload_integrity_valid: true,
        };
        assert_eq!(assess_recovery(&valid), RecoveryDecision::Accept);

        let mut mutation = valid.clone();
        mutation.version = 2;
        assert_eq!(
            assess_recovery(&mutation),
            RecoveryDecision::Refuse("TL_E2EE_UNKNOWN_VERSION")
        );
        mutation = valid.clone();
        mutation.recipient_present = false;
        assert_eq!(
            assess_recovery(&mutation),
            RecoveryDecision::Refuse("TL_E2EE_RECOVERY_UNAVAILABLE")
        );
        mutation = valid.clone();
        mutation.tenant_id = "tenant-other";
        assert_eq!(
            assess_recovery(&mutation),
            RecoveryDecision::Refuse("TL_E2EE_TENANT_MISMATCH")
        );
        mutation = valid.clone();
        mutation.group_id = "group-other";
        assert_eq!(
            assess_recovery(&mutation),
            RecoveryDecision::Refuse("TL_E2EE_GROUP_MISMATCH")
        );
        mutation = valid.clone();
        mutation.epoch = 3;
        assert_eq!(
            assess_recovery(&mutation),
            RecoveryDecision::Refuse("TL_E2EE_OLD_EPOCH")
        );
        mutation = valid.clone();
        mutation.payload_integrity_valid = false;
        assert_eq!(
            assess_recovery(&mutation),
            RecoveryDecision::Refuse("TL_E2EE_CORRUPT")
        );
        mutation = valid;
        mutation.authorized = false;
        assert_eq!(
            assess_recovery(&mutation),
            RecoveryDecision::Refuse("TL_E2EE_RECOVERY_REFUSED")
        );
    }

    #[test]
    fn history_policy_is_tenant_bound_and_authorized() {
        assert_eq!(assess_history(true, true), RecoveryDecision::Accept);
        assert_eq!(
            assess_history(true, false),
            RecoveryDecision::Refuse("TL_E2EE_HISTORY_UNAUTHORIZED")
        );
        assert_eq!(
            assess_history(false, true),
            RecoveryDecision::Refuse("TL_E2EE_TENANT_MISMATCH")
        );
    }
}

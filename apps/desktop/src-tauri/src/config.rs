//! Process configuration from the environment. All-or-nothing: either every
//! `THREADLINE_DESKTOP_*` variable is present and valid, or none is and the
//! host runs without a session. Secrets never appear in `Debug` output.

use std::{fmt, path::PathBuf};

/// Variable names in the order `from_env` consumes them.
const ALL: [&str; 7] = [
    "THREADLINE_DESKTOP_CORE_URL",
    "THREADLINE_DESKTOP_DEV_TOKEN",
    "THREADLINE_DESKTOP_TENANT_ID",
    "THREADLINE_DESKTOP_ACTOR_ID",
    "THREADLINE_DESKTOP_DEVICE_ID",
    "THREADLINE_DESKTOP_DATA_DIR",
    "THREADLINE_DESKTOP_DB_KEY",
];

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum ConfigError {
    /// Some variables are set but this one is missing or empty.
    Missing(&'static str),
    /// The database key is not 64 hexadecimal characters or is all zero.
    InvalidKey,
}

pub struct HostConfig {
    pub core_url: String,
    pub realtime_url: Option<String>,
    pub dev_token: String,
    pub tenant_id: String,
    pub actor_id: String,
    pub device_id: String,
    pub data_dir: PathBuf,
    pub db_key: [u8; 32],
}

impl fmt::Debug for HostConfig {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("HostConfig")
            .field("core_url", &self.core_url)
            .field("tenant_id", &self.tenant_id)
            .field("actor_id", &self.actor_id)
            .field("device_id", &self.device_id)
            .field("data_dir", &self.data_dir)
            .finish_non_exhaustive()
    }
}

impl HostConfig {
    /// `Ok(None)` when no variable is set; `Err` when the set is incomplete or
    /// a value is malformed, so a typo never silently falls back to fixtures.
    pub fn from_env(lookup: impl Fn(&str) -> Option<String>) -> Result<Option<Self>, ConfigError> {
        let values: Vec<Option<String>> = ALL
            .iter()
            .map(|name| lookup(name).filter(|value| !value.trim().is_empty()))
            .collect();
        let realtime_url =
            lookup("THREADLINE_DESKTOP_REALTIME_URL").filter(|value| !value.trim().is_empty());
        if values.iter().all(Option::is_none) && realtime_url.is_none() {
            return Ok(None);
        }
        let mut required = ALL.iter().zip(values).map(|(name, value)| {
            value
                .map(|value| value.trim().to_owned())
                .ok_or(ConfigError::Missing(name))
        });
        let mut next = || required.next().expect("seven variables");
        let core_url = next()?;
        let dev_token = next()?;
        let tenant_id = next()?;
        let actor_id = next()?;
        let device_id = next()?;
        let data_dir = PathBuf::from(next()?);
        let db_key = parse_key(&next()?)?;
        Ok(Some(Self {
            core_url,
            realtime_url,
            dev_token,
            tenant_id,
            actor_id,
            device_id,
            data_dir,
            db_key,
        }))
    }
}

fn parse_key(hex: &str) -> Result<[u8; 32], ConfigError> {
    if hex.len() != 64 || !hex.is_ascii() {
        return Err(ConfigError::InvalidKey);
    }
    let bytes = (0..32)
        .map(|index| u8::from_str_radix(&hex[index * 2..index * 2 + 2], 16))
        .collect::<Result<Vec<u8>, _>>()
        .map_err(|_| ConfigError::InvalidKey)?;
    let key: [u8; 32] = bytes.try_into().map_err(|_| ConfigError::InvalidKey)?;
    if key.iter().all(|byte| *byte == 0) {
        return Err(ConfigError::InvalidKey);
    }
    Ok(key)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn complete() -> Vec<(&'static str, String)> {
        let values = [
            "http://127.0.0.1:8080",
            "dev-token-synthetic-0123456789",
            "tenant-synthetic",
            "actor-synthetic",
            "device-synthetic",
            "/tmp/threadline-synthetic",
            &"ab".repeat(32),
        ];
        ALL.iter().copied().zip(values.map(str::to_owned)).collect()
    }

    fn lookup<'values>(
        values: &'values [(&'values str, String)],
    ) -> impl Fn(&str) -> Option<String> + 'values {
        move |name| {
            values
                .iter()
                .find(|(key, _)| *key == name)
                .map(|(_, value)| value.clone())
        }
    }

    #[test]
    fn complete_environment_parses_and_debug_redacts_secrets() {
        let values = complete();
        let config = HostConfig::from_env(lookup(&values))
            .expect("parse")
            .expect("configured");
        assert_eq!(config.core_url, "http://127.0.0.1:8080");
        assert_eq!(config.db_key, [0xab; 32]);
        assert_eq!(config.data_dir, PathBuf::from("/tmp/threadline-synthetic"));
        let debug = format!("{config:?}");
        assert!(!debug.contains("dev-token-synthetic"));
        assert!(!debug.contains("abab"));
        assert!(debug.contains("device-synthetic"));
    }

    #[test]
    fn empty_environment_means_no_session() {
        assert!(matches!(HostConfig::from_env(|_| None), Ok(None)));
        assert!(matches!(
            HostConfig::from_env(|_| Some("   ".to_owned())),
            Ok(None)
        ));
    }

    #[test]
    fn partial_environment_names_the_missing_variable() {
        let mut values = complete();
        values.retain(|(name, _)| *name != "THREADLINE_DESKTOP_DEVICE_ID");
        assert_eq!(
            HostConfig::from_env(lookup(&values)).err(),
            Some(ConfigError::Missing("THREADLINE_DESKTOP_DEVICE_ID"))
        );
    }

    #[test]
    fn optional_realtime_is_host_only_and_requires_a_complete_session() {
        let mut values = complete();
        values.push((
            "THREADLINE_DESKTOP_REALTIME_URL",
            "http://127.0.0.1:8082".into(),
        ));
        let config = HostConfig::from_env(lookup(&values)).unwrap().unwrap();
        assert_eq!(
            config.realtime_url.as_deref(),
            Some("http://127.0.0.1:8082")
        );
        assert_eq!(
            HostConfig::from_env(|name| (name == "THREADLINE_DESKTOP_REALTIME_URL")
                .then(|| "http://127.0.0.1:8082".into()))
            .err(),
            Some(ConfigError::Missing("THREADLINE_DESKTOP_CORE_URL"))
        );
    }

    #[test]
    fn database_key_must_be_64_hex_digits_and_not_all_zero() {
        for bad in ["ab".repeat(31), "zz".repeat(32), "00".repeat(32)] {
            let mut values = complete();
            values.retain(|(name, _)| *name != "THREADLINE_DESKTOP_DB_KEY");
            values.push(("THREADLINE_DESKTOP_DB_KEY", bad));
            assert_eq!(
                HostConfig::from_env(lookup(&values)).err(),
                Some(ConfigError::InvalidKey)
            );
        }
    }
}

//! One Device session: the `locald` store plus the network seam. Generic over
//! `Server` so the whole send → sync → display path is unit-tested against an
//! in-memory server; the host instantiates it with `CoreClient`.

use std::{
    sync::atomic::{AtomicU64, Ordering},
    time::{SystemTime, UNIX_EPOCH},
};

use prost_types::Timestamp;
use threadline_client_core::storage::{DatabaseKey, StorageError};
use threadline_core_client::envelope::{self, PlaintextMessage};
use threadline_core_client::{CoreClient, Endpoint};
use threadline_locald::{DeviceIdentity, Locald, LocaldError, Server};

use crate::config::HostConfig;
use crate::dto::{ChannelDto, HostError, MessageDto, SessionInfo, SyncReportDto};

const PULL_BATCH: u32 = 500;
const HISTORY_LIMIT: u32 = 1_000;
const STORE_FILE: &str = "locald.sqlite";

pub struct Session<S: Server> {
    locald: Locald,
    pub(crate) server: S,
    info: SessionInfo,
    sequence: AtomicU64,
}

impl Session<CoreClient> {
    /// Opens the encrypted store under `data_dir` and binds the Core client.
    /// No network call happens here; the renderer drives sync explicitly.
    pub fn connect(config: HostConfig) -> Result<Self, HostError> {
        let client = CoreClient::new(Endpoint {
            base_url: config.core_url.clone(),
            bearer_token: config.dev_token.clone(),
        })
        .map_err(|_| HostError::InvalidInput)?;
        let identity = DeviceIdentity {
            tenant_id: config.tenant_id.clone(),
            device_id: config.device_id.clone(),
        };
        std::fs::create_dir_all(&config.data_dir).map_err(|_| HostError::Storage)?;
        let key = DatabaseKey::new(config.db_key).map_err(|_| HostError::InvalidInput)?;
        let locald =
            Locald::open(config.data_dir.join(STORE_FILE), key, identity).map_err(map_locald)?;
        let info = SessionInfo {
            core_url: config.core_url,
            tenant_id: config.tenant_id,
            actor_id: config.actor_id,
            device_id: config.device_id,
        };
        Ok(Self::with_server(locald, client, info))
    }

    pub fn channels(&self) -> Result<Vec<ChannelDto>, HostError> {
        let channels = self
            .server
            .list_channels()
            .map_err(|_| HostError::Unavailable)?;
        Ok(channels
            .into_iter()
            .map(|channel| ChannelDto {
                channel_id: channel.channel_id,
                name: channel.name,
            })
            .collect())
    }
}

impl<S: Server> Session<S> {
    pub fn with_server(locald: Locald, server: S, info: SessionInfo) -> Self {
        Self {
            locald,
            server,
            info,
            sequence: AtomicU64::new(0),
        }
    }

    pub fn info(&self) -> &SessionInfo {
        &self.info
    }

    /// Committed history in sequence order followed by pending items in
    /// enqueue order, exactly what the timeline renders.
    pub fn messages(&self, channel_id: &str) -> Result<Vec<MessageDto>, HostError> {
        let committed = self
            .locald
            .committed(channel_id, 0, HISTORY_LIMIT)
            .map_err(map_locald)?;
        let pending = self.locald.pending(channel_id).map_err(map_locald)?;
        let mut messages: Vec<MessageDto> = committed
            .iter()
            .map(|record| {
                MessageDto::from_decoded(
                    channel_id,
                    envelope::decode(&record.envelope_bytes),
                    &record.event_id,
                    Some(record.committed_sequence),
                )
            })
            .collect();
        messages.extend(pending.iter().map(|record| {
            MessageDto::from_decoded(
                channel_id,
                envelope::decode(&record.envelope_bytes),
                &record.event_id,
                None,
            )
        }));
        Ok(messages)
    }

    /// Queues one message durably and returns its pending row. The event id
    /// and idempotency key are minted once here and never change on retry.
    pub fn send(&mut self, channel_id: &str, text: &str) -> Result<MessageDto, HostError> {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map_err(|_| HostError::Storage)?;
        let ordinal = self.sequence.fetch_add(1, Ordering::Relaxed);
        let event_id = format!("evt-{}-{}-{ordinal}", self.info.device_id, now.as_nanos());
        let idempotency_key = format!("key-{event_id}");
        let sent_at = Timestamp {
            seconds: i64::try_from(now.as_secs()).map_err(|_| HostError::Storage)?,
            nanos: i32::try_from(now.subsec_nanos()).map_err(|_| HostError::Storage)?,
        };
        let bytes = envelope::encode(PlaintextMessage {
            tenant_id: &self.info.tenant_id,
            channel_id,
            event_id: &event_id,
            idempotency_key: &idempotency_key,
            sender_actor_id: &self.info.actor_id,
            sender_device_id: &self.info.device_id,
            text,
            client_sent_at: Some(sent_at),
        })
        .map_err(|_| HostError::InvalidInput)?;
        self.locald
            .enqueue(channel_id, &event_id, &idempotency_key, &bytes)
            .map_err(map_locald)?;
        Ok(MessageDto::from_decoded(
            channel_id,
            envelope::decode(&bytes),
            &event_id,
            None,
        ))
    }

    /// Replays pending items, then pulls committed history after the cursor.
    /// The pull outcome decides `online`: a reachable server that still leaves
    /// items pending means those items were denied, not that we are offline.
    pub fn sync(&mut self, channel_id: &str) -> Result<SyncReportDto, HostError> {
        let replay = self
            .locald
            .replay(&mut self.server, channel_id)
            .map_err(map_locald)?;
        let mut report = SyncReportDto {
            committed: replay.committed,
            deduplicated: replay.deduplicated,
            retained: replay.retained,
            online: true,
            ..SyncReportDto::default()
        };
        match self.locald.pull(&mut self.server, channel_id, PULL_BATCH) {
            Ok(pull) => {
                report.applied = pull.applied;
                report.has_more = pull.has_more;
            }
            Err(LocaldError::Retryable) => report.online = false,
            Err(error) => return Err(map_locald(error)),
        }
        Ok(report)
    }
}

fn map_locald(error: LocaldError) -> HostError {
    match error {
        LocaldError::InvalidInput | LocaldError::Storage(StorageError::InvalidInput) => {
            HostError::InvalidInput
        }
        LocaldError::Storage(_) => HostError::Storage,
        LocaldError::Denied => HostError::Denied,
        LocaldError::Retryable => HostError::Unavailable,
        LocaldError::HistoryMismatch => HostError::HistoryMismatch,
    }
}

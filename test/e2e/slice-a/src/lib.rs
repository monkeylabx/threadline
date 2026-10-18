//! Harness for the Slice A end-to-end test: owns one Core process and one
//! `locald` peer per Device, so the test body reads as the user scenario.
//! Nothing here prints tokens, DSNs or envelope bytes.

use std::{
    env, fs,
    net::TcpListener,
    path::{Path, PathBuf},
    process::{Child, Command, Stdio},
    thread,
    time::{Duration, Instant},
};

use threadline_client_core::storage::DatabaseKey;
use threadline_core_client::envelope::{self, PlaintextMessage};
use threadline_core_client::{CoreClient, Endpoint};
use threadline_locald::{DeviceIdentity, Locald, LocaldError, PullReport, ReplayReport};

pub const TENANT: &str = "tenant-dev";
pub const CHANNEL: &str = "channel-product";
const HEALTH_TIMEOUT: Duration = Duration::from_secs(30);

/// One seeded Device from `deploy/compose/dev-sessions.json`.
#[derive(Clone, Copy, Debug)]
pub struct Device {
    pub token: &'static str,
    pub actor_id: &'static str,
    pub device_id: &'static str,
}

pub const LIN_JIA: Device = Device {
    token: "dev-session-lin-jia-laptop-0000000001",
    actor_id: "actor-lin-jia",
    device_id: "device-lin-jia-laptop",
};

pub const MENG_QING: Device = Device {
    token: "dev-session-meng-qing-laptop-000000001",
    actor_id: "actor-meng-qing",
    device_id: "device-meng-qing-laptop",
};

/// What CI provides. `None` means the test is NOT RUN rather than failed.
pub struct Environment {
    core_bin: PathBuf,
    dsn: String,
    repo_root: PathBuf,
}

impl Environment {
    pub fn from_env() -> Option<Self> {
        let core_bin = env::var_os("THREADLINE_E2E_CORE_BIN")?;
        let dsn = env::var("THREADLINE_E2E_POSTGRES_DSN").ok()?;
        let repo_root = Path::new(env!("CARGO_MANIFEST_DIR"))
            .join("../../..")
            .canonicalize()
            .ok()?;
        Some(Self {
            core_bin: PathBuf::from(core_bin),
            dsn,
            repo_root,
        })
    }

    pub fn scratch(&self, name: &str) -> PathBuf {
        let dir = env::temp_dir().join(format!("threadline-e2e-{name}-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).expect("scratch dir");
        dir
    }

    /// Rows of the seeded Channel's committed history, read with `psql`.
    pub fn channel_event_rows(&self) -> u64 {
        let output = Command::new("psql")
            .arg(&self.dsn)
            .args(["--no-psqlrc", "--tuples-only", "--no-align", "--command"])
            .arg(format!(
                "SELECT count(*) FROM domain.channel_events WHERE tenant_id = '{TENANT}' AND channel_id = '{CHANNEL}'"
            ))
            .stderr(Stdio::inherit())
            .output()
            .expect("psql available");
        assert!(output.status.success(), "psql count query failed");
        String::from_utf8_lossy(&output.stdout)
            .trim()
            .parse()
            .expect("integer row count")
    }
}

/// A Core process the test can stop and restart on the same port.
pub struct CoreProcess {
    child: Child,
    base_url: String,
}

impl CoreProcess {
    pub fn free_port() -> u16 {
        let listener = TcpListener::bind("127.0.0.1:0").expect("bind");
        listener.local_addr().expect("addr").port()
    }

    pub fn start(environment: &Environment, port: u16) -> Self {
        let compose = environment.repo_root.join("deploy/compose");
        let child = Command::new(&environment.core_bin)
            .env(
                "THREADLINE_CORE_LISTEN_ADDRESS",
                format!("127.0.0.1:{port}"),
            )
            .env("THREADLINE_CORE_POSTGRES_DSN", &environment.dsn)
            .env(
                "THREADLINE_OUTBOX_POLICY_FILE",
                compose.join("outbox-policy.json"),
            )
            .env("THREADLINE_DEV_SESSIONS_ENABLED", "1")
            .env(
                "THREADLINE_DEV_SESSION_FIXTURES",
                compose.join("dev-sessions.json"),
            )
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::inherit())
            .spawn()
            .expect("spawn core");
        let process = Self {
            child,
            base_url: format!("http://127.0.0.1:{port}"),
        };
        process.wait_healthy();
        process
    }

    pub fn base_url(&self) -> &str {
        &self.base_url
    }

    fn wait_healthy(&self) {
        let http = reqwest::blocking::Client::builder()
            .timeout(Duration::from_secs(1))
            .build()
            .expect("http client");
        let deadline = Instant::now() + HEALTH_TIMEOUT;
        while Instant::now() < deadline {
            if let Ok(response) = http.get(format!("{}/healthz", self.base_url)).send() {
                if response.status().is_success() {
                    return;
                }
            }
            thread::sleep(Duration::from_millis(200));
        }
        panic!("core did not become healthy within {HEALTH_TIMEOUT:?}");
    }

    pub fn stop(mut self) {
        let _ = self.child.kill();
        let _ = self.child.wait();
    }
}

impl Drop for CoreProcess {
    fn drop(&mut self) {
        let _ = self.child.kill();
        let _ = self.child.wait();
    }
}

/// One Device: its encrypted store plus its Core client.
pub struct Peer {
    pub locald: Locald,
    pub client: CoreClient,
    device: Device,
    minted: u32,
}

impl Peer {
    pub fn open(base_url: &str, store_dir: &Path, device: Device, key_byte: u8) -> Self {
        let locald = Locald::open(
            store_dir.join("locald.sqlite"),
            DatabaseKey::new([key_byte; 32]).expect("key"),
            DeviceIdentity {
                tenant_id: TENANT.to_owned(),
                device_id: device.device_id.to_owned(),
            },
        )
        .expect("open store");
        let client = CoreClient::new(Endpoint {
            base_url: base_url.to_owned(),
            bearer_token: device.token.to_owned(),
        })
        .expect("client");
        Self {
            locald,
            client,
            device,
            minted: 0,
        }
    }

    /// Points the client at a restarted Core so no pooled connection to the
    /// old process is reused.
    pub fn reconnect(&mut self, base_url: &str) {
        self.client = CoreClient::new(Endpoint {
            base_url: base_url.to_owned(),
            bearer_token: self.device.token.to_owned(),
        })
        .expect("client");
    }

    /// Queues one message durably; returns its event id.
    pub fn send(&mut self, text: &str) -> String {
        self.minted += 1;
        let event_id = format!("evt-e2e-{}-{}", self.device.device_id, self.minted);
        let idempotency_key = format!("key-{event_id}");
        let bytes = envelope::encode(PlaintextMessage {
            tenant_id: TENANT,
            channel_id: CHANNEL,
            event_id: &event_id,
            idempotency_key: &idempotency_key,
            sender_actor_id: self.device.actor_id,
            sender_device_id: self.device.device_id,
            text,
            client_sent_at: None,
        })
        .expect("encode");
        self.locald
            .enqueue(CHANNEL, &event_id, &idempotency_key, &bytes)
            .expect("enqueue");
        event_id
    }

    pub fn replay(&mut self) -> ReplayReport {
        self.locald
            .replay(&mut self.client, CHANNEL)
            .expect("replay runs")
    }

    pub fn pull(&mut self) -> Result<PullReport, LocaldError> {
        self.locald.pull(&mut self.client, CHANNEL, 500)
    }

    /// `(channel_seq, sender_actor_id, text)` for every committed row.
    pub fn committed(&self) -> Vec<(u64, String, String)> {
        self.locald
            .committed(CHANNEL, 0, 1_000)
            .expect("committed")
            .iter()
            .map(|record| {
                let decoded = envelope::decode(&record.envelope_bytes).expect("decodable");
                (
                    record.committed_sequence,
                    decoded.sender_actor_id,
                    decoded.text,
                )
            })
            .collect()
    }

    pub fn pending_count(&self) -> usize {
        self.locald.pending(CHANNEL).expect("pending").len()
    }
}

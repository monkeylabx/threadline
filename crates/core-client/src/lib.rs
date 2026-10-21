//! Connect client for Core, used by the Desktop host as the production
//! implementation of `threadline_locald::Server`.
//!
//! One blocking HTTP/1.1 client speaks Connect unary (`application/proto`) to
//! `MessageService.SendEvent`, `SyncService.SyncEvents` and
//! `ChannelService.ListChannels`; `server` is the `locald::Server` facade. The bearer credential lives only in memory
//! and never appears in errors, logs or `Debug` output. Envelope payloads are
//! opaque to the transport; the Slice A plaintext codec is `envelope`.

pub mod envelope;
pub mod realtime;
mod rpc;
mod server;
mod transport;

pub use rpc::Channel;
pub use transport::{ClientError, ConnectCode};

use std::fmt;

use transport::Transport;

/// Connection facts for one Device session. The token is a dev session or a
/// production session credential; this crate treats both as opaque.
#[derive(Clone)]
pub struct Endpoint {
    pub base_url: String,
    pub bearer_token: String,
}

/// One authenticated client. Cheap to hold for the life of a window.
pub struct CoreClient {
    transport: Transport,
}

impl CoreClient {
    /// Builds a client. `base_url` is `http(s)://host:port` without a trailing
    /// slash; the token must be a non-empty HTTP token68 value.
    pub fn new(endpoint: Endpoint) -> Result<Self, ClientError> {
        Ok(Self {
            transport: Transport::new(endpoint)?,
        })
    }
}

impl fmt::Debug for CoreClient {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("CoreClient")
            .field("base_url", &self.transport.base_url())
            .finish_non_exhaustive()
    }
}

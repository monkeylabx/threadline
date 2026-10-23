//! Authorized channel discovery and its stable, secret-free IPC failure classes.
use threadline_core_client::{ClientError, ConnectCode, CoreClient};

use crate::dto::{ChannelDto, HostError};

pub(crate) fn load(client: &CoreClient) -> Result<Vec<ChannelDto>, HostError> {
    let channels = client.list_channels().map_err(classify)?;
    Ok(channels
        .into_iter()
        .map(|channel| ChannelDto {
            channel_id: channel.channel_id,
            name: channel.name,
        })
        .collect())
}

fn classify(error: ClientError) -> HostError {
    match error {
        ClientError::Status(ConnectCode::Unauthenticated | ConnectCode::PermissionDenied) => {
            HostError::Denied
        }
        ClientError::Transport
        | ClientError::Status(
            ConnectCode::Unavailable
            | ConnectCode::DeadlineExceeded
            | ConnectCode::ResourceExhausted
            | ConnectCode::Internal
            | ConnectCode::Canceled,
        ) => HostError::Unavailable,
        _ => HostError::InvalidInput,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn local_transport_failure_retries_but_invalid_configuration_does_not() {
        assert_eq!(classify(ClientError::Transport), HostError::Unavailable);
        assert_eq!(
            classify(ClientError::InvalidEndpoint),
            HostError::InvalidInput
        );
    }
}

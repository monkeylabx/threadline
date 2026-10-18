//! Channel listing for the sidebar. The `locald::Server` procedures live in
//! `server`.

use threadline_client_proto::threadline::channel::v1 as channelv1;
use threadline_client_proto::threadline::r#type::v1 as typev1;

use crate::transport::ClientError;
use crate::CoreClient;

const LIST_CHANNELS: &str = "/threadline.channel.v1.ChannelService/ListChannels";
const CHANNEL_PAGE_SIZE: i32 = 200;

/// A Channel the Member may read, as the sidebar needs it.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Channel {
    pub channel_id: String,
    pub name: String,
}

impl CoreClient {
    /// Every readable Channel of the authenticated Member, following page
    /// tokens until the server returns a short page.
    pub fn list_channels(&self) -> Result<Vec<Channel>, ClientError> {
        let mut channels = Vec::new();
        let mut page_token = String::new();
        loop {
            let request = channelv1::ListChannelsRequest {
                page: Some(typev1::PageRequest {
                    page_size: CHANNEL_PAGE_SIZE,
                    page_token: page_token.clone(),
                }),
                ..Default::default()
            };
            let response: channelv1::ListChannelsResponse =
                self.transport.unary(LIST_CHANNELS, &request)?;
            channels.extend(response.channels.into_iter().map(|channel| Channel {
                channel_id: channel.channel_id,
                name: channel.name,
            }));
            page_token = response
                .page
                .map(|page| page.next_page_token)
                .unwrap_or_default();
            if page_token.is_empty() {
                return Ok(channels);
            }
        }
    }
}

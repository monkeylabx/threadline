mod support;

use prost::Message;
use support::{FakeCore, Scripted};
use threadline_client_proto::threadline::channel::v1 as channelv1;
use threadline_client_proto::threadline::r#type::v1 as typev1;
use threadline_core_client::{ClientError, CoreClient, Endpoint};

const TOKEN: &str = "dev-token-synthetic-0123456789";

fn client(base_url: &str) -> CoreClient {
    CoreClient::new(Endpoint {
        base_url: base_url.to_owned(),
        bearer_token: TOKEN.to_owned(),
    })
    .expect("client")
}

#[test]
fn endpoint_validation_rejects_bad_scheme_and_bad_token() {
    for (base_url, token) in [
        ("ftp://127.0.0.1:1", TOKEN),
        ("http://127.0.0.1:1", ""),
        ("http://127.0.0.1:1", "has space"),
    ] {
        let result = CoreClient::new(Endpoint {
            base_url: base_url.to_owned(),
            bearer_token: token.to_owned(),
        });
        assert!(matches!(result, Err(ClientError::InvalidEndpoint)));
    }
}

#[test]
fn debug_output_never_contains_the_bearer_token() {
    let debug = format!("{:?}", client("http://127.0.0.1:1/"));
    assert!(debug.contains("http://127.0.0.1:1"));
    assert!(!debug.contains(TOKEN));
}

#[test]
fn list_channels_follows_page_tokens_until_a_short_page() {
    let page = |ids: &[&str], token: &str| channelv1::ListChannelsResponse {
        channels: ids
            .iter()
            .map(|id| channelv1::Channel {
                channel_id: (*id).to_owned(),
                name: format!("name-{id}"),
                ..Default::default()
            })
            .collect(),
        page: Some(typev1::PageResponse {
            next_page_token: token.to_owned(),
        }),
    };
    let fake = FakeCore::start(vec![
        Scripted::proto(page(&["a", "b"], "b").encode_to_vec()),
        Scripted::proto(page(&["c"], "").encode_to_vec()),
    ]);
    let channels = client(&fake.base_url).list_channels().expect("channels");
    let first = fake.next_request();
    let second = fake.next_request();
    assert_eq!(
        first.path,
        "/threadline.channel.v1.ChannelService/ListChannels"
    );
    let second_request =
        channelv1::ListChannelsRequest::decode(second.body.as_slice()).expect("request");
    assert_eq!(second_request.page.expect("page").page_token, "b");
    let ids: Vec<&str> = channels
        .iter()
        .map(|channel| channel.channel_id.as_str())
        .collect();
    assert_eq!(ids, ["a", "b", "c"]);
    assert_eq!(channels[0].name, "name-a");
}

#[test]
fn list_channels_surfaces_status_and_malformed_bodies() {
    let fake = FakeCore::start(vec![
        Scripted::error(401, "unauthenticated"),
        Scripted::proto(vec![0xff, 0xff, 0xff]),
    ]);
    let client = client(&fake.base_url);
    assert!(matches!(
        client.list_channels(),
        Err(ClientError::Status(_))
    ));
    assert_eq!(client.list_channels(), Err(ClientError::Malformed));
}

use super::super::FakeRun;
use crate::local_interface::tests::authority::Source;
use prost::Message;
use threadline_client_proto::threadline::local_agent::v1::{
    watch_run_activity_response::View, WatchRunActivityRequest as Request,
    WatchRunActivityResponse as Frame,
};

pub fn text(owner: &mut FakeRun, source: &Source) -> String {
    let request = Request {
        run_id: "run-a".to_owned(),
        ..Request::default()
    };
    let mut watch = owner.watch(source, &request).expect("current observer");
    let first = watch
        .next(source, owner.activity())
        .expect("ready")
        .expect("ready frame");
    let first = Frame::decode(first.as_slice()).expect("canonical ready");
    assert_eq!(first.sequence, 0);
    assert!(matches!(first.view, Some(View::Ready(_))));
    let mut text = String::new();
    while let Some(bytes) = watch
        .next(source, owner.activity())
        .expect("current observer")
    {
        let frame = Frame::decode(bytes.as_slice()).expect("canonical update");
        let Some(View::AgentText(chunk)) = frame.view else {
            panic!("text only");
        };
        assert_eq!(frame.input_id, "input-a");
        text.push_str(&chunk);
    }
    text
}

use super::{authority::Source, fixtures};
use crate::local_interface::{Activity, ActivityUpdate as Update, InputError as Error, Watch};
use prost::Message;
use threadline_client_proto::threadline::local_agent::v1::{
    watch_run_activity_response::View, WatchRunActivityRequest as Request,
    WatchRunActivityResponse as Frame,
};

mod policy;
mod retention;

fn activity() -> Activity {
    Activity::new("run-a".to_owned(), "incarnation-a".to_owned()).expect("synthetic identity")
}

fn first() -> Request {
    Request {
        run_id: "run-a".to_owned(),
        ..Request::default()
    }
}

fn frame(watch: &mut Watch, source: &Source, activity: &Activity) -> Frame {
    let bytes = watch
        .next(source, activity)
        .expect("authorized")
        .expect("frame");
    Frame::decode(bytes.as_slice()).expect("canonical frame")
}

fn ready(watch: &mut Watch, source: &Source, activity: &Activity) -> (String, u64, u64) {
    let frame = frame(watch, source, activity);
    assert_eq!(frame.sequence, 0);
    assert!(frame.input_id.is_empty());
    let Some(View::Ready(ready)) = frame.view else {
        panic!("ready first");
    };
    (
        frame.stream_id,
        ready.first_available_sequence,
        ready.latest_sequence,
    )
}

fn drain(watch: &mut Watch, source: &Source, activity: &Activity) -> Vec<Frame> {
    let mut frames = Vec::new();
    while let Some(bytes) = watch.next(source, activity).expect("authorized") {
        frames.push(Frame::decode(bytes.as_slice()).expect("canonical frame"));
    }
    frames
}

fn append(activity: &mut Activity, text: &str) {
    activity
        .append("input-a", Update::Text(text))
        .expect("bounded text");
}

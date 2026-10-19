use super::{ActivityUpdate, Error};
use prost::Message;
use std::collections::VecDeque;
use threadline_client_proto::threadline::local_agent::v1::{
    watch_run_activity_response::View, ActivityStreamReady, LocalToolActivity, LocalToolPhase,
    WatchRunActivityResponse as Frame,
};

const MAX_TEXT: usize = 8192;
const MAX_BYTES: usize = 256 * 1024;

pub(super) struct Buffer {
    pub run_id: String,
    stream_template: String,
    frames: VecDeque<Frame>,
    bytes: usize,
    head: u64,
}

impl Buffer {
    pub fn new(run_id: String, stream_template: String) -> Self {
        Self {
            run_id,
            stream_template,
            frames: VecDeque::new(),
            bytes: 0,
            head: 0,
        }
    }

    pub fn append(&mut self, input: &str, update: ActivityUpdate<'_>) -> Result<(), Error> {
        match update {
            ActivityUpdate::Text(text) if text.len() > 65_536 => Err(Error::PayloadTooLarge),
            ActivityUpdate::Text(mut text) => {
                while !text.is_empty() {
                    let mut end = text.len().min(MAX_TEXT);
                    while !text.is_char_boundary(end) {
                        end -= 1;
                    }
                    self.push(input, View::AgentText(text[..end].to_owned()))?;
                    text = &text[end..];
                }
                Ok(())
            }
            ActivityUpdate::SyntheticTool(phase) if phase != LocalToolPhase::Unspecified => self
                .push(
                    input,
                    View::Tool(LocalToolActivity {
                        label: "Demonstration tool".to_owned(),
                        phase: phase as i32,
                    }),
                ),
            _ => Ok(()),
        }
    }

    fn push(&mut self, input: &str, view: View) -> Result<(), Error> {
        if self.head >= u64::MAX - 1 {
            return Err(Error::ResourceExhausted);
        }
        let sequence = self.head + 1;
        let frame = Frame {
            run_id: self.run_id.clone(),
            stream_id: self.stream_template.clone(),
            sequence,
            input_id: input.to_owned(),
            view: Some(view),
        };
        self.bytes += frame.encoded_len();
        self.frames.push_back(frame);
        self.head = sequence;
        while self.frames.len() > 128 || self.bytes > MAX_BYTES {
            if let Some(oldest) = self.frames.pop_front() {
                self.bytes -= oldest.encoded_len();
            }
        }
        Ok(())
    }

    pub fn floor(&self) -> u64 {
        self.frames
            .front()
            .map_or(self.head + 1, |frame| frame.sequence)
    }

    pub fn check_cursor(&self, after: u64) -> Result<(), Error> {
        if after > self.head {
            return Err(Error::CursorInvalid);
        }
        if after < self.floor() - 1 {
            return Err(Error::SequenceGap);
        }
        Ok(())
    }

    pub fn ready(&self) -> Frame {
        Frame {
            run_id: self.run_id.clone(),
            view: Some(View::Ready(ActivityStreamReady {
                first_available_sequence: self.floor(),
                latest_sequence: self.head,
            })),
            ..Frame::default()
        }
    }

    pub fn next(&self, after: u64) -> Option<Frame> {
        self.frames
            .iter()
            .find(|frame| frame.sequence > after)
            .cloned()
    }

    pub fn clear(&mut self) {
        self.frames.clear();
        self.bytes = 0;
    }
}

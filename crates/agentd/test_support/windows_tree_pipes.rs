use super::{api, JobError};
use std::{
    fs::{File, OpenOptions},
    io::{pipe, PipeReader, PipeWriter},
    os::windows::io::{AsHandle, BorrowedHandle},
};

pub(super) struct RootEnds {
    input: PipeReader,
    reply: PipeWriter,
    output: File,
}

impl RootEnds {
    pub(super) fn create() -> Result<(Self, PipeReader, PipeWriter), JobError> {
        let (input, ack) = pipe().map_err(|_| JobError::Unavailable)?;
        let (reply, writer) = pipe().map_err(|_| JobError::Unavailable)?;
        let output = OpenOptions::new()
            .read(true)
            .write(true)
            .open("NUL")
            .map_err(|_| JobError::Unavailable)?;
        let ends = Self {
            input,
            reply: writer,
            output,
        };
        for handle in ends.handles() {
            api::inherit(handle, true)?;
        }
        Ok((ends, reply, ack))
    }

    pub(super) fn handles(&self) -> [BorrowedHandle<'_>; 3] {
        [
            self.input.as_handle(),
            self.reply.as_handle(),
            self.output.as_handle(),
        ]
    }
}

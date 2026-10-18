use super::JobError;
use std::{
    io::{PipeReader, Read},
    sync::mpsc::{self, Receiver},
    thread::{self, JoinHandle},
    time::Duration,
};

pub(super) fn read_handle(mut input: impl Read) -> Result<usize, JobError> {
    let mut packet = [0; 8];
    input
        .read_exact(&mut packet)
        .map_err(|_| JobError::Unavailable)?;
    let value = usize::try_from(u64::from_le_bytes(packet)).map_err(|_| JobError::Rejected)?;
    if value == 0 || value == usize::MAX {
        return Err(JobError::Rejected);
    }
    Ok(value)
}

pub(super) struct Reader {
    result: Receiver<Result<usize, JobError>>,
    worker: Option<JoinHandle<()>>,
}

impl Reader {
    pub(super) fn start(input: PipeReader) -> Self {
        let (sender, result) = mpsc::sync_channel(1);
        let worker = thread::spawn(move || {
            sender
                .send(read_handle(input))
                .expect("test receiver retained through reader join");
        });
        Self {
            result,
            worker: Some(worker),
        }
    }

    pub(super) fn receive(&mut self) -> Result<usize, JobError> {
        let result = self
            .result
            .recv_timeout(Duration::from_secs(10))
            .map_err(|_| JobError::Unavailable)?;
        self.worker
            .take()
            .ok_or(JobError::Rejected)?
            .join()
            .map_err(|_| JobError::Unavailable)?;
        result
    }
}

impl Drop for Reader {
    fn drop(&mut self) {
        if self.worker.is_some() {
            // Startup drops/waits the root first; its reply writer cannot survive in the leaf.
            // EOF or the fixed packet must arrive before a join. Missing evidence fails visibly.
            let outcome = self.receive();
            assert!(
                self.worker.is_none(),
                "synthetic reader teardown unavailable: {outcome:?}"
            );
        }
    }
}
